// Copyright 2022 Matrix Origin
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package plan

import (
	"github.com/matrixorigin/matrixone/pkg/catalog"
	"github.com/matrixorigin/matrixone/pkg/common/moerr"
	"github.com/matrixorigin/matrixone/pkg/container/types"
	"github.com/matrixorigin/matrixone/pkg/pb/plan"
	"github.com/matrixorigin/matrixone/pkg/sql/parsers/tree"
	"github.com/matrixorigin/matrixone/pkg/sql/util"
)

const derivedTableName = "_t"

type dmlSelectInfo struct {
	typ            string
	projectList    []*Expr
	tblInfo        *dmlTableInfo
	idx            int32
	rootId         int32
	derivedTableId int32

	onIdx    []int32   //remove these row
	onIdxVal [][]int64 //insert these value
	onIdxTbl []*ObjectRef

	onRestrict    []int32 // check these, not all null then throw error
	onRestrictTbl []*ObjectRef

	onSet     [][]int64
	onSetAttr [][]string
	onSetTbl  []*ObjectRef

	onCascade     [][]int64
	onCascadeAttr [][]string
	onCascadeTbl  []*ObjectRef
}

type dmlTableInfo struct {
	objRef          []*ObjectRef
	tableDefs       []*TableDef
	updateColOffset []map[string]int
	updateKeys      []map[string]tree.Expr
	nameToIdx       map[string]int
	idToName        map[uint64]string
	alias           map[string]int
}

func getAliasToName(ctx CompilerContext, expr tree.TableExpr, alias string, aliasMap map[string][2]string) {
	switch t := expr.(type) {
	case *tree.TableName:
		dbName := string(t.SchemaName)
		if dbName == "" {
			dbName = ctx.DefaultDatabase()
		}
		tblName := string(t.ObjectName)
		if alias != "" {
			aliasMap[alias] = [2]string{dbName, tblName}
		}
	case *tree.AliasedTableExpr:
		alias := string(t.As.Alias)
		getAliasToName(ctx, t.Expr, alias, aliasMap)
	case *tree.JoinTableExpr:
		getAliasToName(ctx, t.Left, alias, aliasMap)
		getAliasToName(ctx, t.Right, alias, aliasMap)
	}
}

func getUpdateTableInfo(ctx CompilerContext, stmt *tree.Update) (*dmlTableInfo, error) {
	tblInfo, err := getDmlTableInfo(ctx, stmt.Tables, nil)
	if err != nil {
		return nil, err
	}

	//check update field and set updateKeys
	usedTbl := make(map[string]map[string]tree.Expr)
	allColumns := make(map[string]map[string]struct{})
	for alias, idx := range tblInfo.alias {
		allColumns[alias] = make(map[string]struct{})
		for _, col := range tblInfo.tableDefs[idx].Cols {
			allColumns[alias][col.Name] = struct{}{}
		}
	}

	appendToTbl := func(table, column string, expr tree.Expr) {
		if _, exists := usedTbl[table]; !exists {
			usedTbl[table] = make(map[string]tree.Expr)
		}
		usedTbl[table][column] = expr
	}

	for _, updateExpr := range stmt.Exprs {
		if len(updateExpr.Names) > 1 {
			return nil, moerr.NewNYI(ctx.GetContext(), "unsupport expr")
		}
		parts := updateExpr.Names[0]
		expr := updateExpr.Expr
		if parts.NumParts > 1 {
			colName := parts.Parts[0]
			tblName := parts.Parts[1]
			if _, tblExists := tblInfo.alias[tblName]; tblExists {
				if _, colExists := allColumns[tblName][colName]; colExists {
					appendToTbl(tblName, colName, expr)
				} else {
					return nil, moerr.NewInternalError(ctx.GetContext(), "column '%v' not found in table %s", colName, tblName)
				}
			} else {
				return nil, moerr.NewNoSuchTable(ctx.GetContext(), "", tblName)
			}
		} else {
			colName := parts.Parts[0]
			tblName := ""
			for alias, colulmns := range allColumns {
				if _, colExists := colulmns[colName]; colExists {
					if tblName != "" {
						return nil, moerr.NewInternalError(ctx.GetContext(), "Column '%v' in field list is ambiguous", colName)
					}
					appendToTbl(alias, colName, expr)
				}
			}
		}
	}

	// remove unused table
	newTblInfo := &dmlTableInfo{
		nameToIdx: make(map[string]int),
		idToName:  make(map[uint64]string),
		alias:     make(map[string]int),
	}
	for alias, columns := range usedTbl {
		idx := tblInfo.alias[alias]
		newTblInfo.objRef = append(newTblInfo.objRef, tblInfo.objRef[idx])
		newTblInfo.tableDefs = append(newTblInfo.tableDefs, tblInfo.tableDefs[idx])
		newTblInfo.alias[alias] = len(newTblInfo.tableDefs) - 1
		newTblInfo.updateKeys = append(newTblInfo.updateKeys, columns)
	}
	for idx, ref := range newTblInfo.objRef {
		key := ref.SchemaName + "." + ref.ObjName
		newTblInfo.idToName[newTblInfo.tableDefs[idx].TblId] = key
		newTblInfo.nameToIdx[key] = idx
	}

	return newTblInfo, nil
}

func setTableExprToDmlTableInfo(ctx CompilerContext, tbl tree.TableExpr, tblInfo *dmlTableInfo, aliasMap map[string][2]string) error {
	var tblName, dbName, alias string

	if aliasTbl, ok := tbl.(*tree.AliasedTableExpr); ok {
		alias = string(aliasTbl.As.Alias)
		tbl = aliasTbl.Expr
	}

	if jionTbl, ok := tbl.(*tree.JoinTableExpr); ok {
		err := setTableExprToDmlTableInfo(ctx, jionTbl.Left, tblInfo, aliasMap)
		if err != nil {
			return err
		}
		if jionTbl.Right != nil {
			return setTableExprToDmlTableInfo(ctx, jionTbl.Right, tblInfo, aliasMap)
		}
		return nil
	}

	if baseTbl, ok := tbl.(*tree.TableName); ok {
		tblName = string(baseTbl.ObjectName)
		dbName = string(baseTbl.SchemaName)
	}

	if aliasNames, exist := aliasMap[tblName]; exist {
		dbName = aliasNames[0]
		tblName = aliasNames[1]
	}

	if tblName == "" {
		return nil
	}

	if dbName == "" {
		dbName = ctx.DefaultDatabase()
	}

	_, tableDef := ctx.Resolve(dbName, tblName)
	if tableDef == nil {
		return moerr.NewNoSuchTable(ctx.GetContext(), dbName, tblName)
	}
	if tableDef.TableType == catalog.SystemExternalRel {
		return moerr.NewInvalidInput(ctx.GetContext(), "cannot insert/update/delete from external table")
	} else if tableDef.TableType == catalog.SystemViewRel {
		return moerr.NewInvalidInput(ctx.GetContext(), "cannot insert/update/delete from view")
	}
	if util.TableIsClusterTable(tableDef.GetTableType()) && ctx.GetAccountId() != catalog.System_Account {
		return moerr.NewInternalError(ctx.GetContext(), "only the sys account can insert/update/delete the cluster table %s", tableDef.GetName())
	}

	if tableDef.CompositePkey != nil {
		tableDef.Cols = append(tableDef.Cols, &ColDef{
			Name: tableDef.CompositePkey.Name,
			Alg:  plan.CompressType_Lz4,
			Typ: &Type{
				Id:    int32(types.T_varchar),
				Size:  types.VarlenaSize,
				Width: types.MaxVarcharLen,
			},
			Default: &plan.Default{
				NullAbility:  false,
				Expr:         nil,
				OriginString: "",
			},
		})
	}

	if tableDef.ClusterBy != nil {
		tableDef.Cols = append(tableDef.Cols, &ColDef{
			Name: tableDef.ClusterBy.Name,
			Alg:  plan.CompressType_Lz4,
			Typ: &Type{
				Id:    int32(types.T_varchar),
				Size:  types.VarlenaSize,
				Width: types.MaxVarcharLen,
			},
			Default: &plan.Default{
				NullAbility:  false,
				Expr:         nil,
				OriginString: "",
			},
		})
	}

	nowIdx := len(tblInfo.tableDefs)
	tblInfo.objRef = append(tblInfo.objRef, &ObjectRef{
		Obj:        int64(tableDef.TblId),
		SchemaName: dbName,
		ObjName:    tblName,
	})
	tblInfo.tableDefs = append(tblInfo.tableDefs, tableDef)
	key := dbName + "." + tblName
	tblInfo.nameToIdx[key] = nowIdx
	tblInfo.idToName[tableDef.TblId] = key
	if alias == "" {
		alias = tblName
	}
	tblInfo.alias[alias] = nowIdx

	return nil
}

func getDmlTableInfo(ctx CompilerContext, tableExprs tree.TableExprs, aliasMap map[string][2]string) (*dmlTableInfo, error) {
	tblInfo := &dmlTableInfo{
		nameToIdx: make(map[string]int),
		idToName:  make(map[uint64]string),
		alias:     make(map[string]int),
	}

	for _, tbl := range tableExprs {
		err := setTableExprToDmlTableInfo(ctx, tbl, tblInfo, aliasMap)
		if err != nil {
			return nil, err
		}
	}

	return tblInfo, nil
}

func updateToSelect(builder *QueryBuilder, bindCtx *BindContext, stmt *tree.Update, tableInfo *dmlTableInfo, haveConstraint bool) (int32, error) {
	fromTables := &tree.From{
		Tables: stmt.Tables,
	}
	selectList := make([]tree.SelectExpr, len(tableInfo.tableDefs))
	columnsSize := 0
	for alias, i := range tableInfo.alias {
		e, _ := tree.NewUnresolvedNameWithStar(builder.GetContext(), alias)
		columnsSize += len(tableInfo.tableDefs[i].Cols)
		selectList[i] = tree.SelectExpr{
			Expr: e,
		}
	}

	//------------------------------------------wuxiliang add--------------------------------------------
	counter := 0
	updateColsOffset := make([]map[string]int, len(tableInfo.updateKeys))
	for idx, tbUpdateMap := range tableInfo.updateKeys {
		tableDef := tableInfo.tableDefs[idx]
		updateColsOffset[idx] = make(map[string]int)

		for colName, updateCol := range tbUpdateMap {
			valuePos := columnsSize + counter
			// Add update expression after select list
			selectList = append(selectList, tree.SelectExpr{
				Expr: updateCol,
			})

			found := false
			for _, coldef := range tableDef.Cols {
				if colName == coldef.Name {
					found = true
					updateColsOffset[idx][colName] = valuePos
					break
				}
			}
			if !found {
				return -1, moerr.NewInvalidInput(builder.GetContext(), "Column '%s' does not exist in the table", colName)
			}
			counter++
		}
	}
	tableInfo.updateColOffset = updateColsOffset
	//--------------------------------------wuxiliang add------------------------------------------------

	selectAst := &tree.Select{
		Select: &tree.SelectClause{
			Distinct: false,
			Exprs:    selectList,
			From:     fromTables,
			Where:    stmt.Where,
		},
		OrderBy: stmt.OrderBy,
		Limit:   stmt.Limit,
		With:    stmt.With,
	}
	//ftCtx := tree.NewFmtCtx(dialect.MYSQL)
	//selectAst.Format(ftCtx)
	//sql := ftCtx.String()
	//fmt.Print(sql)
	return builder.buildSelect(selectAst, bindCtx, false)
}

func initInsertStmt(builder *QueryBuilder, bindCtx *BindContext, stmt *tree.Insert, info *dmlSelectInfo) error {
	var err error
	syntaxHasColumnNames := false
	if stmt.Columns != nil {
		syntaxHasColumnNames = true
	}
	tableDef := info.tblInfo.tableDefs[0]

	var astSlt *tree.Select
	switch slt := stmt.Rows.Select.(type) {
	case *tree.ValuesClause:

		isAllDefault := false
		if slt.Rows[0] == nil {
			isAllDefault = true
		}
		//example1:insert into a(a) values ();
		//but it does not work at the case:
		//insert into a(a) values (0),();
		if isAllDefault && syntaxHasColumnNames {
			return moerr.NewInvalidInput(builder.GetContext(), "insert values does not match the number of columns")
		}

		slt.RowWord = true
		astSlt = &tree.Select{
			Select: &tree.SelectClause{
				Exprs: []tree.SelectExpr{
					{
						Expr: tree.UnqualifiedStar{},
					},
				},
				From: &tree.From{
					Tables: []tree.TableExpr{
						&tree.JoinTableExpr{
							JoinType: tree.JOIN_TYPE_CROSS,
							Left: &tree.AliasedTableExpr{
								As: tree.AliasClause{},
								Expr: &tree.ParenTableExpr{
									Expr: &tree.Select{Select: slt},
								},
							},
						},
					},
				},
			},
		}
	case *tree.SelectClause:
		astSlt = stmt.Rows
	case *tree.ParenSelect:
		astSlt = slt.Select
	default:
		return moerr.NewInvalidInput(builder.GetContext(), "insert has unknown select statement")
	}

	compositePkey := ""
	if tableDef.CompositePkey != nil {
		compositePkey = tableDef.CompositePkey.Name
	}
	clusterByKey := ""
	if tableDef.ClusterBy != nil {
		clusterByKey = tableDef.ClusterBy.Name
	}

	subCtx := NewBindContext(builder, bindCtx)
	info.rootId, err = builder.buildSelect(astSlt, subCtx, false)
	if err != nil {
		return err
	}
	err = builder.addBinding(info.rootId, tree.AliasClause{
		Alias: derivedTableName,
	}, bindCtx)
	if err != nil {
		return err
	}

	lastNode := builder.qry.Nodes[info.rootId]
	tag := builder.qry.Nodes[info.rootId].BindingTags[0]
	info.derivedTableId = info.rootId
	oldProject := append([]*Expr{}, lastNode.ProjectList...)

	colToIdx := make(map[string]int)
	for i, col := range tableDef.Cols {
		colToIdx[col.Name] = i
	}

	updateColumnToExpr := make(map[string]*Expr)
	for i, column := range stmt.Columns {
		columnStr := string(column)
		colIdx, exists := colToIdx[columnStr]
		if !exists {
			return moerr.NewInvalidInput(builder.GetContext(), "insert value into unknown column '%s'", columnStr)
		}
		projExpr := &plan.Expr{
			Typ: oldProject[i].Typ,
			Expr: &plan.Expr_Col{
				Col: &plan.ColRef{
					RelPos: tag,
					ColPos: int32(i),
				},
			},
		}
		if !isSameColumnType(projExpr.Typ, tableDef.Cols[colIdx].Typ) {
			projExpr, err = appendCastBeforeExpr(builder.GetContext(), projExpr, tableDef.Cols[colIdx].Typ)
			if err != nil {
				return err
			}
		}
		updateColumnToExpr[columnStr] = projExpr
	}

	info.projectList = make([]*Expr, 0, len(tableDef.Cols)-1)

	for _, col := range tableDef.Cols {
		if col.Name == catalog.Row_ID {
			continue
		} else if col.Name == compositePkey {
			// append composite primary key
			colNames := util.SplitCompositePrimaryKeyColumnName(compositePkey)
			args := make([]*Expr, len(colNames))
			for _, colName := range colNames {
				if oldExpr, exists := updateColumnToExpr[col.Name]; exists {
					args = append(args, oldExpr)
				} else {
					col = tableDef.Cols[colToIdx[colName]]
					defExpr, err := getDefaultExpr(builder.GetContext(), col)
					if err != nil {
						return err
					}
					args = append(args, defExpr)
				}
			}
			serFunExpr, err := bindFuncExprImplByPlanExpr(builder.GetContext(), "serial", args)
			if err != nil {
				return err
			}
			info.projectList = append(info.projectList, serFunExpr)

		} else if col.Name == clusterByKey {
			// append composite cluster key
			colNames := util.SplitCompositeClusterByColumnName(clusterByKey)
			args := make([]*Expr, len(colNames))
			for _, colName := range colNames {
				if oldExpr, exists := updateColumnToExpr[col.Name]; exists {
					args = append(args, oldExpr)
				} else {
					col = tableDef.Cols[colToIdx[colName]]
					defExpr, err := getDefaultExpr(builder.GetContext(), col)
					if err != nil {
						return err
					}
					args = append(args, defExpr)
				}
			}
			serFunExpr, err := bindFuncExprImplByPlanExpr(builder.GetContext(), "serial", args)
			if err != nil {
				return err
			}
			info.projectList = append(info.projectList, serFunExpr)

		} else {
			if oldExpr, exists := updateColumnToExpr[col.Name]; exists {
				info.projectList = append(info.projectList, oldExpr)
			} else {
				defExpr, err := getDefaultExpr(builder.GetContext(), col)
				if err != nil {
					return err
				}
				info.projectList = append(info.projectList, defExpr)
			}
		}
	}

	if checkIfStmtHaveRewriteConstraint(info.tblInfo) {
		err = rewriteDmlSelectInfo(builder, bindCtx, info, tableDef, info.derivedTableId)
		if err != nil {
			return err
		}
	}

	// append ProjectNode
	info.rootId = builder.appendNode(&plan.Node{
		NodeType:    plan.Node_PROJECT,
		ProjectList: info.projectList,
		Children:    []int32{info.rootId},
		BindingTags: []int32{bindCtx.projectTag},
	}, bindCtx)
	bindCtx.results = info.projectList

	return nil
}

func deleteToSelect(builder *QueryBuilder, bindCtx *BindContext, node *tree.Delete, haveConstraint bool) (int32, error) {
	var selectList []tree.SelectExpr
	fromTables := &tree.From{}

	getResolveExpr := func(tblName string) tree.SelectExpr {
		var ret *tree.UnresolvedName
		if haveConstraint {
			ret, _ = tree.NewUnresolvedNameWithStar(builder.GetContext(), tblName)
		} else {
			ret, _ = tree.NewUnresolvedName(builder.GetContext(), tblName, catalog.Row_ID)
		}
		return tree.SelectExpr{
			Expr: ret,
		}
	}

	for _, tbl := range node.Tables {
		if aliasTbl, ok := tbl.(*tree.AliasedTableExpr); ok {
			alias := string(aliasTbl.As.Alias)
			if alias != "" {
				selectList = append(selectList, getResolveExpr(alias))
			} else {
				astTbl := aliasTbl.Expr.(*tree.TableName)
				selectList = append(selectList, getResolveExpr(string(astTbl.ObjectName)))
			}
		} else if astTbl, ok := tbl.(*tree.TableName); ok {
			selectList = append(selectList, getResolveExpr(string(astTbl.ObjectName)))
		}
	}

	if node.TableRefs != nil {
		fromTables.Tables = node.TableRefs
	} else {
		fromTables.Tables = node.Tables
	}

	astSelect := &tree.Select{
		Select: &tree.SelectClause{
			Distinct: false,
			Exprs:    selectList,
			From:     fromTables,
			Where:    node.Where,
		},
		OrderBy: node.OrderBy,
		Limit:   node.Limit,
		With:    node.With,
	}
	// ftCtx := tree.NewFmtCtx(dialectType)
	// astSelect.Format(ftCtx)
	// sql := ftCtx.String()
	// fmt.Print(sql)

	return builder.buildSelect(astSelect, bindCtx, false)
}

func checkIfStmtHaveRewriteConstraint(tblInfo *dmlTableInfo) bool {
	for _, tableDef := range tblInfo.tableDefs {
		for _, def := range tableDef.Defs {
			if _, ok := def.Def.(*plan.TableDef_DefType_UIdx); ok {
				return true
			}
		}
		if len(tableDef.RefChildTbls) > 0 {
			return true
		}
	}
	return false
}

func initDeleteStmt(builder *QueryBuilder, bindCtx *BindContext, info *dmlSelectInfo, stmt *tree.Delete) error {
	var err error
	subCtx := NewBindContext(builder, bindCtx)
	info.rootId, err = deleteToSelect(builder, subCtx, stmt, true)
	if err != nil {
		return err
	}

	err = builder.addBinding(info.rootId, tree.AliasClause{
		Alias: derivedTableName,
	}, bindCtx)
	if err != nil {
		return err
	}

	info.idx = int32(len(info.tblInfo.objRef))
	tag := builder.qry.Nodes[info.rootId].BindingTags[0]
	info.derivedTableId = info.rootId
	for idx, expr := range builder.qry.Nodes[info.rootId].ProjectList {
		if expr.Typ.Id == int32(types.T_Rowid) {
			info.projectList = append(info.projectList, &plan.Expr{
				Typ: expr.Typ,
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: tag,
						ColPos: int32(idx),
					},
				},
			})
		}
	}
	return nil
}

func initUpdateStmt(builder *QueryBuilder, bindCtx *BindContext, info *dmlSelectInfo, stmt *tree.Update) error {
	var err error
	subCtx := NewBindContext(builder, bindCtx)
	info.rootId, err = updateToSelect(builder, subCtx, stmt, info.tblInfo, true)
	if err != nil {
		return err
	}

	err = builder.addBinding(info.rootId, tree.AliasClause{
		Alias: derivedTableName,
	}, bindCtx)
	if err != nil {
		return err
	}

	info.idx = int32(len(info.tblInfo.objRef))
	tag := builder.qry.Nodes[info.rootId].BindingTags[0]
	info.derivedTableId = info.rootId

	for i, tableDef := range info.tblInfo.tableDefs {

		// compositePkey := ""
		// if tableDef.CompositePkey != nil {
		// 	compositePkey = tableDef.CompositePkey.Name
		// }
		// clusterByKey := ""
		// if tableDef.ClusterBy != nil {
		// 	clusterByKey = tableDef.ClusterBy.Name
		// }

		updateOffsetMap := info.tblInfo.updateColOffset[i]
		idx := 0
		for _, coldef := range tableDef.Cols {
			if pos, ok := updateOffsetMap[coldef.Name]; ok {
				info.projectList = append(info.projectList, &plan.Expr{
					Typ: coldef.Typ,
					Expr: &plan.Expr_Col{
						Col: &plan.ColRef{
							RelPos: tag,
							ColPos: int32(pos),
						},
					},
				})
			} else {
				// todo
				// if coldef.Name == compositePkey {

				// } else if coldef.Name == clusterByKey {

				// }
				info.projectList = append(info.projectList, &plan.Expr{
					Typ: coldef.Typ,
					Expr: &plan.Expr_Col{
						Col: &plan.ColRef{
							RelPos: tag,
							ColPos: int32(idx),
						},
					},
				})
			}
			idx++
		}
	}
	return nil
}

func rewriteDmlSelectInfo(builder *QueryBuilder, bindCtx *BindContext, info *dmlSelectInfo, tableDef *TableDef, baseNodeId int32) error {
	posMap := make(map[string]int32)
	typMap := make(map[string]*plan.Type)
	id2name := make(map[uint64]string)
	beginPos := 0

	//use origin query as left, we need add prefix pos
	if baseNodeId == info.derivedTableId {
		for _, d := range info.tblInfo.tableDefs {
			if d.Name == tableDef.Name {
				break
			}
			beginPos = beginPos + len(d.Cols)
		}
	}

	for idx, col := range tableDef.Cols {
		posMap[col.Name] = int32(beginPos + idx)
		typMap[col.Name] = col.Typ
		id2name[col.ColId] = col.Name
	}

	// rewrite index
	for _, def := range tableDef.Defs {
		if idxDef, ok := def.Def.(*plan.TableDef_DefType_UIdx); ok {
			for idx, tblName := range idxDef.UIdx.TableNames {
				idxRef := &plan.ObjectRef{
					SchemaName: builder.compCtx.DefaultDatabase(),
					ObjName:    tblName,
				}

				// append table_scan node
				rightCtx := NewBindContext(builder, bindCtx)
				astTblName := tree.NewTableName(tree.Identifier(tblName), tree.ObjectNamePrefix{})
				rightId, err := builder.buildTable(astTblName, rightCtx)
				if err != nil {
					return err
				}
				rightTag := builder.qry.Nodes[rightId].BindingTags[0]
				baseTag := builder.qry.Nodes[baseNodeId].BindingTags[0]
				rightTableDef := builder.qry.Nodes[rightId].TableDef
				rightRowIdPos := int32(len(rightTableDef.Cols)) - 1
				rightIdxPos := int32(0)

				// append projection
				info.projectList = append(info.projectList, &plan.Expr{
					Typ: rightTableDef.Cols[rightRowIdPos].Typ,
					Expr: &plan.Expr_Col{
						Col: &plan.ColRef{
							RelPos: rightTag,
							ColPos: rightRowIdPos,
						},
					},
				})

				rightExpr := &plan.Expr{
					Typ: rightTableDef.Cols[rightIdxPos].Typ,
					Expr: &plan.Expr_Col{
						Col: &plan.ColRef{
							RelPos: rightTag,
							ColPos: rightIdxPos,
						},
					},
				}

				// append join node
				var joinConds []*Expr
				var leftExpr *Expr
				var originIdxList []int64 //we need insert new value.  todo we have bug when cascade to next level
				partsLength := len(idxDef.UIdx.Fields[idx].Parts)
				if partsLength == 1 {
					orginIndexColumnName := idxDef.UIdx.Fields[idx].Parts[0]
					originIdx := int32(posMap[orginIndexColumnName])
					typ := typMap[orginIndexColumnName]
					leftExpr = &Expr{
						Typ: typ,
						Expr: &plan.Expr_Col{
							Col: &plan.ColRef{
								RelPos: baseTag,
								ColPos: originIdx,
							},
						},
					}
					originIdxList = append(originIdxList, int64(originIdx))
				} else {
					args := make([]*Expr, partsLength)
					for i, column := range idxDef.UIdx.Fields[idx].Parts {
						originIdx := int32(posMap[column])
						typ := typMap[column]
						args[i] = &plan.Expr{
							Typ: typ,
							Expr: &plan.Expr_Col{
								Col: &plan.ColRef{
									RelPos: baseTag,
									ColPos: int32(posMap[column]),
								},
							},
						}
						originIdxList = append(originIdxList, int64(originIdx))
					}
					leftExpr, err = bindFuncExprImplByPlanExpr(builder.GetContext(), "serial", args)
					if err != nil {
						return err
					}
				}

				condExpr, err := bindFuncExprImplByPlanExpr(builder.GetContext(), "=", []*Expr{leftExpr, rightExpr})
				if err != nil {
					return err
				}
				joinConds = []*Expr{condExpr}

				leftCtx := builder.ctxByNode[info.rootId]
				joinCtx := NewBindContext(builder, bindCtx)
				err = joinCtx.mergeContexts(leftCtx, rightCtx)
				if err != nil {
					return err
				}
				newRootId := builder.appendNode(&plan.Node{
					NodeType: plan.Node_JOIN,
					Children: []int32{info.rootId, rightId},
					JoinType: plan.Node_LEFT,
					OnList:   joinConds,
				}, joinCtx)
				bindCtx.binder = NewTableBinder(builder, bindCtx)
				info.rootId = newRootId

				info.onIdxTbl = append(info.onIdxTbl, idxRef)
				info.onIdx = append(info.onIdx, info.idx)
				info.onIdxVal = append(info.onIdxVal, originIdxList)
				info.idx = info.idx + 1
			}
		}
	}

	// rewrite foreign key
	if info.typ != "insert" {
		for _, tableId := range tableDef.RefChildTbls {
			if _, existInDelTable := info.tblInfo.idToName[tableId]; existInDelTable {
				// delete parent_tbl, child_tbl from parent_tbl join child_tbl xxxxxx
				// we will skip child_tbl here.
				continue
			}

			_, childTableDef := builder.compCtx.ResolveById(tableId) //opt: actionRef是否也记录到RefChildTbls里？

			childPosMap := make(map[string]int32)
			childTypMap := make(map[string]*plan.Type)
			childId2name := make(map[uint64]string)
			var childAttrs []string
			for idx, col := range childTableDef.Cols {
				childPosMap[col.Name] = int32(idx)
				childTypMap[col.Name] = col.Typ
				childId2name[col.ColId] = col.Name
				if col.Name != catalog.Row_ID {
					childAttrs = append(childAttrs, col.Name)
				}
			}

			objRef := &plan.ObjectRef{
				Obj:        int64(childTableDef.TblId),
				SchemaName: builder.compCtx.DefaultDatabase(),
				ObjName:    childTableDef.Name,
			}

			for _, fk := range childTableDef.Fkeys {
				if fk.ForeignTbl == tableDef.TblId {
					// append table scan node
					rightCtx := NewBindContext(builder, bindCtx)
					astTblName := tree.NewTableName(tree.Identifier(childTableDef.Name), tree.ObjectNamePrefix{})
					rightId, err := builder.buildTable(astTblName, rightCtx)
					if err != nil {
						return err
					}
					rightTag := builder.qry.Nodes[rightId].BindingTags[0]
					baseNodeTag := builder.qry.Nodes[baseNodeId].BindingTags[0]
					// needRecursionCall := false

					// build join conds
					joinConds := make([]*Expr, len(fk.Cols))
					for i, colId := range fk.Cols {
						for _, col := range childTableDef.Cols {
							if col.ColId == colId {
								childColumnName := col.Name
								originColumnName := id2name[fk.ForeignCols[i]]

								leftExpr := &Expr{
									Typ: typMap[originColumnName],
									Expr: &plan.Expr_Col{
										Col: &plan.ColRef{
											RelPos: baseNodeTag,
											ColPos: posMap[originColumnName],
										},
									},
								}
								rightExpr := &plan.Expr{
									Typ: childTypMap[childColumnName],
									Expr: &plan.Expr_Col{
										Col: &plan.ColRef{
											RelPos: rightTag,
											ColPos: childPosMap[childColumnName],
										},
									},
								}
								condExpr, err := bindFuncExprImplByPlanExpr(builder.GetContext(), "=", []*Expr{leftExpr, rightExpr})
								if err != nil {
									return err
								}
								joinConds[i] = condExpr
								break
							}
						}
					}

					// append project
					var refAction plan.ForeignKeyDef_RefAction
					if info.typ == "update" {
						refAction = fk.OnUpdate
					} else {
						refAction = fk.OnDelete
					}

					switch refAction {
					case plan.ForeignKeyDef_NO_ACTION, plan.ForeignKeyDef_RESTRICT, plan.ForeignKeyDef_SET_DEFAULT:
						info.projectList = append(info.projectList, &plan.Expr{
							Typ: childTypMap[catalog.Row_ID],
							Expr: &plan.Expr_Col{
								Col: &plan.ColRef{
									RelPos: rightTag,
									ColPos: childPosMap[catalog.Row_ID],
								},
							},
						})
						info.onRestrict = append(info.onRestrict, info.idx)
						info.idx = info.idx + 1
						info.onRestrictTbl = append(info.onRestrictTbl, objRef)

					case plan.ForeignKeyDef_CASCADE:
						// for update ,we need to reset column's value of child table, just like set null
						if info.typ == "update" {
							fkIdMap := make(map[uint64]struct{})
							for _, colId := range fk.Cols {
								fkIdMap[colId] = struct{}{}
							}

							var setIdxs []int64
							for j, col := range childTableDef.Cols {
								if _, ok := fkIdMap[col.ColId]; ok {
									originName := id2name[col.ColId]
									info.projectList = append(info.projectList, &plan.Expr{
										Typ: col.Typ,
										Expr: &plan.Expr_Col{
											Col: &plan.ColRef{
												RelPos: baseNodeTag,
												ColPos: posMap[originName],
											},
										},
									})
								} else {
									info.projectList = append(info.projectList, &plan.Expr{
										Typ: col.Typ,
										Expr: &plan.Expr_Col{
											Col: &plan.ColRef{
												RelPos: rightTag,
												ColPos: int32(j),
											},
										},
									})
								}
								setIdxs = append(setIdxs, int64(info.idx))
								info.idx = info.idx + 1
							}
							info.onCascade = append(info.onCascade, setIdxs)
							info.onCascadeTbl = append(info.onCascadeTbl, objRef)
							info.onCascadeAttr = append(info.onCascadeAttr, childAttrs)
						} else {
							// for delete, we only get row_id and delete the rows
							info.projectList = append(info.projectList, &plan.Expr{
								Typ: childTypMap[catalog.Row_ID],
								Expr: &plan.Expr_Col{
									Col: &plan.ColRef{
										RelPos: rightTag,
										ColPos: childPosMap[catalog.Row_ID],
									},
								},
							})
							info.onCascade = append(info.onCascade, []int64{int64(info.idx)})
							info.idx = info.idx + 1
							info.onCascadeTbl = append(info.onCascadeTbl, objRef)
						}

						// needRecursionCall = true

					case plan.ForeignKeyDef_SET_NULL:
						fkIdMap := make(map[uint64]struct{})
						for _, colId := range fk.Cols {
							fkIdMap[colId] = struct{}{}
						}
						var setIdxs []int64
						for j, col := range childTableDef.Cols {
							if _, ok := fkIdMap[col.ColId]; ok {
								info.projectList = append(info.projectList, makePlan2NullConstExprWithType())
							} else {
								info.projectList = append(info.projectList, &plan.Expr{
									Typ: col.Typ,
									Expr: &plan.Expr_Col{
										Col: &plan.ColRef{
											RelPos: rightTag,
											ColPos: int32(j),
										},
									},
								})
							}
							setIdxs = append(setIdxs, int64(info.idx))
							info.idx = info.idx + 1
						}
						info.onSet = append(info.onSet, setIdxs)
						info.onSetTbl = append(info.onSetTbl, objRef)
						info.onSetAttr = append(info.onSetAttr, childAttrs)
						// needRecursionCall = true
					}

					// append join node
					leftCtx := builder.ctxByNode[info.rootId]
					joinCtx := NewBindContext(builder, bindCtx)
					err = joinCtx.mergeContexts(leftCtx, rightCtx)
					if err != nil {
						return err
					}
					newRootId := builder.appendNode(&plan.Node{
						NodeType: plan.Node_JOIN,
						Children: []int32{info.rootId, rightId},
						JoinType: plan.Node_LEFT,
						OnList:   joinConds,
					}, joinCtx)
					bindCtx.binder = NewTableBinder(builder, bindCtx)
					info.rootId = newRootId

					// if needRecursionCall {

					// err := rewriteDeleteSelectInfo(builder, bindCtx, info, childTableDef, info.rootId)
					// if err != nil {
					// 	return err
					// }
					// }
				}
			}
		}
	}

	// rewrite for feign key
	if info.typ == "insert" {
		for _, fk := range info.tblInfo.tableDefs[0].Fkeys {
			_, parentTableDef := builder.compCtx.ResolveById(fk.ForeignTbl)

			parentPosMap := make(map[string]int32)
			parentTypMap := make(map[string]*plan.Type)
			parentId2name := make(map[uint64]string)
			for idx, col := range parentTableDef.Cols {
				parentPosMap[col.Name] = int32(idx)
				parentTypMap[col.Name] = col.Typ
				parentId2name[col.ColId] = col.Name
			}

			objRef := &plan.ObjectRef{
				Obj:        int64(parentTableDef.TblId),
				SchemaName: builder.compCtx.DefaultDatabase(),
				ObjName:    parentTableDef.Name,
			}

			// append table scan node
			rightCtx := NewBindContext(builder, bindCtx)
			astTblName := tree.NewTableName(tree.Identifier(parentTableDef.Name), tree.ObjectNamePrefix{})
			rightId, err := builder.buildTable(astTblName, rightCtx)
			if err != nil {
				return err
			}
			rightTag := builder.qry.Nodes[rightId].BindingTags[0]
			baseNodeTag := builder.qry.Nodes[baseNodeId].BindingTags[0]
			// needRecursionCall := false

			// build join conds
			joinConds := make([]*Expr, len(fk.Cols))
			for i, colId := range fk.Cols {
				for _, col := range parentTableDef.Cols {
					if col.ColId == colId {
						childColumnName := col.Name
						originColumnName := id2name[fk.ForeignCols[i]]

						leftExpr := &Expr{
							Typ: typMap[originColumnName],
							Expr: &plan.Expr_Col{
								Col: &plan.ColRef{
									RelPos: baseNodeTag,
									ColPos: posMap[originColumnName],
								},
							},
						}
						rightExpr := &plan.Expr{
							Typ: parentTypMap[childColumnName],
							Expr: &plan.Expr_Col{
								Col: &plan.ColRef{
									RelPos: rightTag,
									ColPos: parentPosMap[childColumnName],
								},
							},
						}
						condExpr, err := bindFuncExprImplByPlanExpr(builder.GetContext(), "=", []*Expr{leftExpr, rightExpr})
						if err != nil {
							return err
						}
						joinConds[i] = condExpr
						break
					}
				}
			}

			// append project
			info.projectList = append(info.projectList, &plan.Expr{
				Typ: parentTypMap[catalog.Row_ID],
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: rightTag,
						ColPos: parentPosMap[catalog.Row_ID],
					},
				},
			})
			info.onRestrict = append(info.onRestrict, info.idx)
			info.idx = info.idx + 1
			info.onRestrictTbl = append(info.onRestrictTbl, objRef)

			// append join node
			leftCtx := builder.ctxByNode[info.rootId]
			joinCtx := NewBindContext(builder, bindCtx)
			err = joinCtx.mergeContexts(leftCtx, rightCtx)
			if err != nil {
				return err
			}
			newRootId := builder.appendNode(&plan.Node{
				NodeType: plan.Node_JOIN,
				Children: []int32{info.rootId, rightId},
				JoinType: plan.Node_LEFT,
				OnList:   joinConds,
			}, joinCtx)
			bindCtx.binder = NewTableBinder(builder, bindCtx)
			info.rootId = newRootId
		}

		// todo check for OnDuplicateUpdate

		// todo check for replace
	}
	return nil
}
