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
	"github.com/matrixorigin/matrixone/pkg/sql/parsers/dialect"
	"github.com/matrixorigin/matrixone/pkg/sql/parsers/dialect/mysql"
	"github.com/matrixorigin/matrixone/pkg/sql/parsers/tree"
	"github.com/matrixorigin/matrixone/pkg/sql/util"
)

const derivedTableName = "_t"

type deleteSelectInfo struct {
	projectList           []*Expr
	tblInfo               *dmlTableInfo
	idx                   int
	rootId                int32
	derivedTableId        int32
	onDeleteIdx           []int
	onDeleteIdxTblName    [][2]string
	onDeleteRestrict      []int
	onDeleteRestrictTblId []uint64
	onDeleteSet           []int
	onDeleteSetTblId      []uint64
	onDeleteCascade       []int
	onDeleteCascadeTblId  []uint64
}

type dmlTableInfo struct {
	dbNames    []string
	tableNames []string
	tableDefs  []*TableDef
	nameMap    map[string]int
}

func getAliasToName(ctx CompilerContext, expr tree.TableExpr, alias string, aliasMap map[string][2]string) {
	switch t := expr.(type) {
	case *tree.TableName:
		dbName := string(t.SchemaName)
		if dbName == "" {
			dbName = ctx.DefaultDatabase()
		}
		tblName := string(t.ObjectName)
		aliasMap[alias] = [2]string{dbName, tblName}
	case *tree.AliasedTableExpr:
		alias := string(t.As.Alias)
		getAliasToName(ctx, t.Expr, alias, aliasMap)
	case *tree.JoinTableExpr:
		getAliasToName(ctx, t.Left, alias, aliasMap)
		getAliasToName(ctx, t.Right, alias, aliasMap)
	}
}

func getDmlTableInfo(ctx CompilerContext, tableExprs tree.TableExprs, aliasMap map[string][2]string) (*dmlTableInfo, error) {
	tblLen := len(tableExprs)
	tblInfo := &dmlTableInfo{
		dbNames:    make([]string, tblLen),
		tableNames: make([]string, tblLen),
		tableDefs:  make([]*plan.TableDef, tblLen),
		nameMap:    make(map[string]int),
	}

	for idx, tbl := range tableExprs {
		var tblName, dbName string

		if aliasTbl, ok := tbl.(*tree.AliasedTableExpr); ok {
			if baseTbl, ok := aliasTbl.Expr.(*tree.TableName); ok {
				tblName = string(baseTbl.ObjectName)
			} else {
				return nil, moerr.NewInternalError(ctx.GetContext(), "%v is not a normal table", tree.String(tbl, dialect.MYSQL))
			}
		} else if baseTbl, ok := tbl.(*tree.TableName); ok {
			tblName = string(baseTbl.ObjectName)
		}
		if aliasNames, exist := aliasMap[tblName]; exist {
			dbName = aliasNames[0]
			tblName = aliasNames[1]
		} else {
			dbName = ctx.DefaultDatabase()
		}

		_, tableDef := ctx.Resolve(dbName, tblName)
		if tableDef == nil {
			return nil, moerr.NewNoSuchTable(ctx.GetContext(), dbName, tblName)
		}
		if tableDef.TableType == catalog.SystemExternalRel {
			return nil, moerr.NewInvalidInput(ctx.GetContext(), "cannot update/delete from external table")
		} else if tableDef.TableType == catalog.SystemViewRel {
			return nil, moerr.NewInvalidInput(ctx.GetContext(), "cannot update/delete from view")
		}
		if util.TableIsClusterTable(tableDef.GetTableType()) && ctx.GetAccountId() != catalog.System_Account {
			return nil, moerr.NewInternalError(ctx.GetContext(), "only the sys account can delete the cluster table %s", tableDef.GetName())
		}

		tblInfo.dbNames[idx] = dbName
		tblInfo.tableNames[idx] = tblName
		tblInfo.tableDefs[idx] = tableDef
		tblInfo.nameMap[tblName] = idx
	}

	return tblInfo, nil
}

// delete from a1, a2 using t1 as a1 inner join t2 as a2 where a1.id = a2.id
// select a1.row_id, a2.row_id from t1 as a1 inner join t2 as a2 where a1.id = a2.id
// select _t.* from (select a1.row_id, a2.row_id from t1 as a1 inner join t2 as a2 where a1.id = a2.id) _t
func deleteToSelect(node *tree.Delete, dialectType dialect.DialectType, haveConstraint bool) string {
	ctx := tree.NewFmtCtx(dialectType)
	if node.With != nil {
		node.With.Format(ctx)
		ctx.WriteByte(' ')
	}
	ctx.WriteString("select ")
	prefix := ""
	for _, tbl := range node.Tables {
		ctx.WriteString(prefix)
		tbl.Format(ctx)
		if haveConstraint {
			ctx.WriteString(".*")
		} else {
			ctx.WriteByte('.')
			ctx.WriteString(catalog.Row_ID)
		}
		prefix = ", "
	}

	// if node.PartitionNames != nil {
	// 	ctx.WriteString(" partition(")
	// 	node.PartitionNames.Format(ctx)
	// 	ctx.WriteByte(')')
	// }

	if node.TableRefs != nil {
		ctx.WriteString(" from ")
		node.TableRefs.Format(ctx)
	} else {
		ctx.WriteString(" from ")
		prefix := ""
		for _, tbl := range node.Tables {
			ctx.WriteString(prefix)
			tbl.Format(ctx)
			prefix = ", "
		}
	}

	if node.Where != nil {
		ctx.WriteByte(' ')
		node.Where.Format(ctx)
	}
	if len(node.OrderBy) > 0 {
		ctx.WriteByte(' ')
		node.OrderBy.Format(ctx)
	}
	if node.Limit != nil {
		ctx.WriteByte(' ')
		node.Limit.Format(ctx)
	}
	return ctx.String()
}

// columns in index_table: idx_col = 0; pri_col = 1; rowid =2
const INDEX_TABLE_IDX_POS = 0
const INDEX_TABLE_ROWID_POS = 2

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

func initDeleteStmt(builder *QueryBuilder, bindCtx *BindContext, info *deleteSelectInfo, stmt *tree.Delete) error {
	sql := deleteToSelect(stmt, dialect.MYSQL, true)
	stmts, err := mysql.Parse(builder.GetContext(), sql)
	if err != nil {
		return err
	}
	subCtx := NewBindContext(builder, bindCtx)
	info.rootId, err = builder.buildSelect(stmts[0].(*tree.Select), subCtx, false)
	if err != nil {
		return err
	}

	err = builder.addBinding(info.rootId, tree.AliasClause{
		Alias: derivedTableName,
	}, bindCtx)
	if err != nil {
		return err
	}

	info.idx = len(info.tblInfo.tableNames)
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
			break
		}
	}
	return nil
}

func rewriteDeleteSelectInfo(builder *QueryBuilder, bindCtx *BindContext, info *deleteSelectInfo, tableDef *TableDef, leftId int32) error {
	posMap := make(map[string]int32)
	typMap := make(map[string]*plan.Type)
	id2name := make(map[uint64]string)
	beginPos := 0
	//use origin query as left, we need add prefix pos
	if leftId == info.derivedTableId {
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
				// append table_scan node
				rightCtx := NewBindContext(builder, bindCtx)
				astTblName := tree.NewTableName(tree.Identifier(tblName), tree.ObjectNamePrefix{})
				// here we get columns: idx_col = 0; pri_col = 1; rowid =2
				rightId, err := builder.buildTable(astTblName, rightCtx)
				if err != nil {
					return err
				}
				rightTag := builder.qry.Nodes[rightId].BindingTags[0]
				leftTag := builder.qry.Nodes[leftId].BindingTags[0]
				rightTableDef := builder.qry.Nodes[rightId].TableDef

				// append projection
				info.projectList = append(info.projectList, &plan.Expr{
					Typ: rightTableDef.Cols[INDEX_TABLE_ROWID_POS].Typ,
					Expr: &plan.Expr_Col{
						Col: &plan.ColRef{
							RelPos: rightTag,
							ColPos: INDEX_TABLE_ROWID_POS,
						},
					},
				})

				rightExpr := &plan.Expr{
					Typ: rightTableDef.Cols[INDEX_TABLE_IDX_POS].Typ,
					Expr: &plan.Expr_Col{
						Col: &plan.ColRef{
							RelPos: rightTag,
							ColPos: INDEX_TABLE_IDX_POS,
						},
					},
				}

				// append join node
				var joinConds []*Expr
				var leftExpr *Expr
				partsLength := len(idxDef.UIdx.Fields[idx].Parts)
				if partsLength == 1 {
					orginIndexColumnName := idxDef.UIdx.Fields[idx].Parts[0]
					typ := typMap[orginIndexColumnName]
					leftExpr = &Expr{
						Typ: typ,
						Expr: &plan.Expr_Col{
							Col: &plan.ColRef{
								RelPos: leftTag,
								ColPos: int32(posMap[orginIndexColumnName]),
							},
						},
					}
				} else {
					args := make([]*Expr, partsLength)
					for i, column := range idxDef.UIdx.Fields[idx].Parts {
						typ := typMap[column]
						args[i] = &plan.Expr{
							Typ: typ,
							Expr: &plan.Expr_Col{
								Col: &plan.ColRef{
									RelPos: leftTag,
									ColPos: int32(posMap[column]),
								},
							},
						}
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

				leftCtx := builder.ctxByNode[leftId]
				err = bindCtx.mergeContexts(leftCtx, rightCtx)
				if err != nil {
					return err
				}
				newRootId := builder.appendNode(&plan.Node{
					NodeType: plan.Node_JOIN,
					Children: []int32{leftId, rightId},
					JoinType: plan.Node_LEFT,
				}, bindCtx)
				node := builder.qry.Nodes[newRootId]
				bindCtx.binder = NewTableBinder(builder, bindCtx)
				node.OnList = joinConds
				info.rootId = newRootId

				info.onDeleteIdxTblName = append(info.onDeleteIdxTblName, [2]string{builder.compCtx.DefaultDatabase(), tblName})
				info.onDeleteIdx = append(info.onDeleteIdx, info.idx)
				info.idx = info.idx + 1
			}
		}
	}

	// rewrite foreign key
	for _, tableId := range tableDef.RefChildTbls {
		_, childTableDef := builder.compCtx.ResolveById(tableId) //opt: actionRef是否也记录到RefChildTbls里？

		childPosMap := make(map[string]int32)
		childTypMap := make(map[string]*plan.Type)
		childId2name := make(map[uint64]string)
		for idx, col := range tableDef.Cols {
			childPosMap[col.Name] = int32(idx)
			childTypMap[col.Name] = col.Typ
			childId2name[col.ColId] = col.Name
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
				leftTag := builder.qry.Nodes[leftId].BindingTags[0]
				needRecursionCall := false

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
										RelPos: leftTag,
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
				switch fk.OnDelete {
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
					info.onDeleteRestrict = append(info.onDeleteIdx, info.idx)
					info.idx = info.idx + 1
					info.onDeleteRestrictTblId = append(info.onDeleteRestrictTblId, childTableDef.TblId)

				case plan.ForeignKeyDef_CASCADE:
					info.projectList = append(info.projectList, &plan.Expr{
						Typ: childTypMap[catalog.Row_ID],
						Expr: &plan.Expr_Col{
							Col: &plan.ColRef{
								RelPos: rightTag,
								ColPos: childPosMap[catalog.Row_ID],
							},
						},
					})
					info.onDeleteCascade = append(info.onDeleteIdx, info.idx)
					info.idx = info.idx + 1
					info.onDeleteCascadeTblId = append(info.onDeleteCascadeTblId, childTableDef.TblId)

					needRecursionCall = true

				case plan.ForeignKeyDef_SET_NULL:
					fkIdMap := make(map[uint64]struct{})
					for _, colId := range fk.Cols {
						fkIdMap[colId] = struct{}{}
					}
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
						info.onDeleteSet = append(info.onDeleteIdx, info.idx)
						info.idx = info.idx + 1
					}
					info.onDeleteSetTblId = append(info.onDeleteSetTblId, childTableDef.TblId)
					needRecursionCall = true
				}

				// append join node
				leftCtx := builder.ctxByNode[leftId]
				err = bindCtx.mergeContexts(leftCtx, rightCtx)
				if err != nil {
					return err
				}
				newRootId := builder.appendNode(&plan.Node{
					NodeType: plan.Node_JOIN,
					Children: []int32{leftId, rightId},
					JoinType: plan.Node_LEFT,
				}, bindCtx)
				node := builder.qry.Nodes[newRootId]
				bindCtx.binder = NewTableBinder(builder, bindCtx)
				node.OnList = joinConds
				info.rootId = newRootId

				if needRecursionCall {
					err := rewriteDeleteSelectInfo(builder, bindCtx, info, childTableDef, info.rootId)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	// projectBegin = projectBegin + len(tableDef.Cols)

	return nil
}

// func rewriteDeleteSelect(ctx CompilerContext, tableDef *TableDef, info *rewriteSelectInfo) error {
// 	// rewrite index
// 	for _, def := range tableDef.Defs {
// 		if idxDef, ok := def.Def.(*plan.TableDef_DefType_UIdx); ok {
// 			for idx, tblName := range idxDef.UIdx.TableNames {
// 				if len(idxDef.UIdx.Fields[idx].Parts) == 1 {
// 					orginIndexColumnName := idxDef.UIdx.Fields[idx].Parts[0]
// 					info.projects += ", `" + tblName + "`." + catalog.Row_ID
// 					leftJoinStr := fmt.Sprintf(" left join `%s` on `%s`.%s = `%s`.%s", tblName, tblName, catalog.IndexTableIndexColName, derivedTableName, orginIndexColumnName)
// 					info.leftJoins += leftJoinStr
// 				} else {
// 					orginIndexColumnNames := ""
// 					prefix := ""
// 					for _, column := range idxDef.UIdx.Fields[idx].Parts {
// 						orginIndexColumnNames += fmt.Sprintf(" `%s`.%s%s", derivedTableName, column, prefix)
// 						prefix = ","
// 					}
// 					info.projects += ", `" + tblName + "`." + catalog.Row_ID
// 					leftJoinStr := fmt.Sprintf(" left join `%s` on `%s`.%s = serial(%s)", tblName, tblName, catalog.IndexTableIndexColName, orginIndexColumnNames)
// 					info.leftJoins += leftJoinStr
// 				}

// 				info.onDeleteIdxTblName = append(info.onDeleteIdxTblName, [2]string{ctx.DefaultDatabase(), tblName})
// 				info.onDeleteIdx = append(info.onDeleteIdx, info.idx)
// 				info.idx = info.idx + 1
// 			}
// 		}
// 	}

// 	// rewrite refChild
// 	id2name := make(map[uint64]string)
// 	for _, col := range tableDef.Cols {
// 		id2name[col.ColId] = col.Name
// 	}

// 	for _, tableId := range tableDef.RefChildTbls {
// 		_, childTableDef := ctx.ResolveById(tableId) //opt: actionRef是否也记录到RefChildTbls里？
// 		for _, fk := range childTableDef.Fkeys {
// 			if fk.ForeignTbl == tableDef.TblId {
// 				leftJoinStr := ""
// 				prefix := fmt.Sprintf(" left join %s on", childTableDef.Name)
// 				for i, colId := range fk.Cols {
// 					childColumnName := ""
// 					originColumnName := ""
// 					for _, col := range childTableDef.Cols {
// 						if col.ColId == colId {
// 							childColumnName = col.Name
// 							originColumnName = id2name[fk.ForeignCols[i]]
// 							break
// 						}
// 					}
// 					leftJoinStr = fmt.Sprintf("%s %s.%s = %s.%s", prefix, childTableDef.Name, childColumnName, derivedTableName, originColumnName)
// 					prefix = ", and"
// 				}

// 				switch fk.OnDelete {
// 				case plan.ForeignKeyDef_CASCADE:
// 					info.projects += ", " + childTableDef.Name + "." + catalog.Row_ID
// 					info.leftJoins += leftJoinStr
// 					info.onDeleteCascade = append(info.onDeleteIdx, info.idx)
// 					info.idx = info.idx + 1
// 					info.onDeleteCascadeTblId = append(info.onDeleteCascadeTblId, childTableDef.TblId)

// 					err := rewriteDeleteSelect(ctx, childTableDef, info)
// 					if err != nil {
// 						return err
// 					}

// 				case plan.ForeignKeyDef_NO_ACTION, plan.ForeignKeyDef_RESTRICT:
// 					info.projects += ", " + childTableDef.Name + "." + catalog.Row_ID
// 					info.leftJoins += leftJoinStr
// 					info.onDeleteRestrict = append(info.onDeleteIdx, info.idx)
// 					info.idx = info.idx + 1
// 					info.onDeleteRestrictTblId = append(info.onDeleteRestrictTblId, childTableDef.TblId)

// 				case plan.ForeignKeyDef_SET_NULL, plan.ForeignKeyDef_SET_DEFAULT:
// 					idMap := make(map[uint64]struct{})
// 					for _, colId := range fk.Cols {
// 						idMap[colId] = struct{}{}
// 					}
// 					info.projects += ", " + childTableDef.Name + "." + catalog.Row_ID
// 					info.onDeleteSet = append(info.onDeleteIdx, info.idx)
// 					info.idx = info.idx + 1
// 					for _, col := range childTableDef.Cols {
// 						if col.Name == catalog.Row_ID {
// 							continue
// 						}
// 						if _, ok := idMap[col.ColId]; ok {
// 							if fk.OnDelete == plan.ForeignKeyDef_SET_NULL {
// 								info.projects += ", null"
// 							} else {
// 								info.projects += ", " + col.Default.OriginString
// 							}
// 						} else {
// 							info.projects += ", " + childTableDef.Name + "." + col.Name
// 						}
// 						info.onDeleteSet = append(info.onDeleteIdx, info.idx)
// 						info.idx = info.idx + 1
// 					}
// 					info.leftJoins += leftJoinStr
// 					info.onDeleteSetTblId = append(info.onDeleteSetTblId, childTableDef.TblId)

// 					err := rewriteDeleteSelect(ctx, childTableDef, info)
// 					if err != nil {
// 						return err
// 					}

// 				}
// 			}
// 		}
// 	}
// 	return nil
// }
