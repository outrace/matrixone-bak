// Copyright 2021 - 2022 Matrix Origin
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package plan

import (
	"strings"

	"github.com/matrixorigin/matrixone/pkg/catalog"
	"github.com/matrixorigin/matrixone/pkg/common/moerr"
	"github.com/matrixorigin/matrixone/pkg/pb/plan"
	"github.com/matrixorigin/matrixone/pkg/sql/parsers/tree"
)

func buildDelete(stmt *tree.Delete, ctx CompilerContext) (*Plan, error) {
	aliasMap := make(map[string][2]string)
	for _, tbl := range stmt.TableRefs {
		getAliasToName(ctx, tbl, "", aliasMap)
	}
	tblInfo, err := getDmlTableInfo(ctx, stmt.Tables, aliasMap)
	if err != nil {
		return nil, err
	}
	builder := NewQueryBuilder(plan.Query_SELECT, ctx)
	bindCtx := NewBindContext(builder, nil)
	bindCtx.groupTag = builder.genNewTag()
	bindCtx.aggregateTag = builder.genNewTag()
	bindCtx.projectTag = builder.genNewTag()

	rewriteInfo := &deleteSelectInfo{
		rootId:  -1,
		tblInfo: tblInfo,
	}

	canTruncate := false
	parentIds := make([]*plan.DeleteTableCtxIdList, len(tblInfo.tableDefs))
	if checkIfStmtHaveRewriteConstraint(tblInfo) {
		// if delete table have constraint
		err = initDeleteStmt(builder, bindCtx, rewriteInfo, stmt)
		if err != nil {
			return nil, err
		}

		for _, tableDef := range tblInfo.tableDefs {
			err = rewriteDeleteSelectInfo(builder, bindCtx, rewriteInfo, tableDef, rewriteInfo.derivedTableId)
			if err != nil {
				return nil, err
			}
		}

		// append ProjectNode
		rewriteInfo.rootId = builder.appendNode(&plan.Node{
			NodeType:    plan.Node_PROJECT,
			ProjectList: rewriteInfo.projectList,
			Children:    []int32{rewriteInfo.rootId},
			BindingTags: []int32{bindCtx.projectTag},
		}, bindCtx)
		bindCtx.results = rewriteInfo.projectList
	} else {
		// if delete table have no constraint
		if stmt.Where == nil && stmt.Limit == nil {
			canTruncate = true
			for i, def := range tblInfo.tableDefs {
				var idList []int64
				for _, fk := range def.Fkeys {
					if _, existInDelTable := tblInfo.idToName[fk.ForeignTbl]; !existInDelTable {
						idList = append(idList, int64(fk.ForeignTbl))
					}
				}
				parentIds[i] = &plan.DeleteTableCtxIdList{
					List: idList,
				}
			}
		}
		selectStmt := deleteToSelect(builder.GetContext(), stmt, false)
		rewriteInfo.rootId, err = builder.buildSelect(selectStmt, bindCtx, false)
		if err != nil {
			return nil, err
		}
	}

	builder.qry.Steps = append(builder.qry.Steps, rewriteInfo.rootId)
	query, err := builder.createQuery()
	if err != nil {
		return nil, err
	}

	// append delete node
	deleteCtx := &plan.DeleteTableCtx{
		CanTruncate:   canTruncate,
		ParentIds:     parentIds,
		DelRef:        rewriteInfo.tblInfo.objRef,
		DelIdxRef:     rewriteInfo.onDeleteIdxTbl,
		DelIdxIdx:     rewriteInfo.onDeleteIdx,
		OnRestrictRef: rewriteInfo.onDeleteRestrictTbl,
		OnRestrictIdx: rewriteInfo.onDeleteRestrict,
		OnCascadeRef:  rewriteInfo.onDeleteCascadeTbl,
		OnCascadeIdx:  rewriteInfo.onDeleteCascade,
		OnSetRef:      rewriteInfo.onDeleteSetTbl,
		OnSetIdx:      make([]*plan.DeleteTableCtxIdList, len(rewriteInfo.onDeleteSet)),
	}
	for i, setList := range rewriteInfo.onDeleteSet {
		deleteCtx.OnSetIdx[i] = &plan.DeleteTableCtxIdList{
			List: setList,
		}
	}

	node := &Node{
		NodeType: plan.Node_DELETE,
		ObjRef:   nil,
		TableDef: nil,
		Children: []int32{query.Steps[len(query.Steps)-1]},
		NodeId:   int32(len(query.Nodes)),
		// DeleteTablesCtx: delCtxs,
		DeleteCtx: deleteCtx,
	}
	query.Nodes = append(query.Nodes, node)
	query.Steps[len(query.Steps)-1] = node.NodeId
	query.StmtType = plan.Query_DELETE

	return &Plan{
		Plan: &plan.Plan_Query{
			Query: query,
		},
	}, err
}

// build rowid column abstract syntax tree expression of the table to be deleted
func buildRowIdAstExpr(ctx CompilerContext, tbinfo *tableInfo, schemaName string, tableName string) (tree.SelectExpr, error) {
	hideKey := ctx.GetHideKeyDef(schemaName, tableName)
	if hideKey == nil {
		return tree.SelectExpr{}, moerr.NewInvalidState(ctx.GetContext(), "cannot find hide key")
	}
	tblAliasName := tableName
	if tbinfo != nil {
		tblAliasName = tbinfo.baseName2AliasMap[tableName]
	}
	expr := tree.SetUnresolvedName(tblAliasName, hideKey.Name)
	return tree.SelectExpr{Expr: expr}, nil
}

// build Index table ast expr
func buildIndexTableExpr(indexTableName string) tree.TableExpr {
	prefix := tree.ObjectNamePrefix{
		CatalogName:     "",
		SchemaName:      "",
		ExplicitCatalog: false,
		ExplicitSchema:  false,
	}

	tableExpr := tree.NewTableName(tree.Identifier(indexTableName), prefix)

	aliasClause := tree.AliasClause{
		Alias: "",
	}
	return tree.NewAliasedTableExpr(tableExpr, aliasClause)
}

// construct equivalent connection conditions between original table and index table
func buildJoinOnCond(tbinfo *tableInfo, originTableName string, indexTableName string, indexField *plan.Field) *tree.OnJoinCond {
	originTableAlias := tbinfo.baseName2AliasMap[originTableName]
	// If it is a single column index
	if len(indexField.Parts) == 1 {
		uniqueColName := indexField.Parts[0]
		leftExpr := tree.SetUnresolvedName(originTableAlias, uniqueColName)
		rightExpr := tree.SetUnresolvedName(indexTableName, strings.ToLower(catalog.IndexTableIndexColName))

		onCondExpr := tree.NewComparisonExprWithSubop(tree.EQUAL, tree.EQUAL, leftExpr, rightExpr)
		return tree.NewOnJoinCond(onCondExpr)
	} else { // If it is a composite index
		funcName := tree.SetUnresolvedName(strings.ToLower("serial"))
		// build function parameters
		exprs := make(tree.Exprs, len(indexField.Parts))
		for i, part := range indexField.Parts {
			exprs[i] = tree.SetUnresolvedName(originTableAlias, part)
		}

		// build composite index serialize function expression
		leftExpr := &tree.FuncExpr{
			Func:  tree.FuncName2ResolvableFunctionReference(funcName),
			Exprs: exprs,
		}

		rightExpr := tree.SetUnresolvedName(indexTableName, strings.ToLower(catalog.IndexTableIndexColName))
		onCondExpr := tree.NewComparisonExprWithSubop(tree.EQUAL, tree.EQUAL, leftExpr, rightExpr)
		return tree.NewOnJoinCond(onCondExpr)
	}
}
