// Copyright 2018 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package cascades

import (
	plannercore "github.com/pingcap/tidb/planner/core"
	"github.com/pingcap/tidb/planner/memo"
<<<<<<< HEAD
=======
	"github.com/pingcap/tidb/planner/util"
	"github.com/pingcap/tidb/sessionctx"
	"github.com/pingcap/tidb/types"
	"github.com/pingcap/tidb/util/ranger"
	"github.com/pingcap/tidb/util/set"
>>>>>>> 7ebcc20... executor: support GROUP_CONCAT(ORDER BY) (#16591)
)

// Transformation defines the interface for the transformation rules.
type Transformation interface {
	GetPattern() *memo.Pattern
	Match(expr *memo.ExprIter) (matched bool)
	OnTransform(old *memo.ExprIter) (new *memo.GroupExpr, eraseOld bool, err error)
}

// GetTransformationRules gets the all the candidate transformation rules based
// on the logical plan node.
func GetTransformationRules(node plannercore.LogicalPlan) []Transformation {
	return transformationMap[memo.GetOperand(node)]
}

var transformationMap = map[memo.Operand][]Transformation{
	/**
	operandSelect: []Transformation{
		nil,
	},
	operandProject: []Transformation{
		nil,
	},
<<<<<<< HEAD
	*/
=======
}

// PostTransformationBatch does the transformation which is related to
// the constraints of the execution engine of TiDB.
// For example, TopN/Sort only support `order by` columns in TiDB layer,
// as for scalar functions, we need to inject a Projection for them
// below the TopN/Sort.
var PostTransformationBatch = TransformationRuleBatch{
	memo.OperandProjection: {
		NewRuleEliminateProjection(),
		NewRuleMergeAdjacentProjection(),
	},
	memo.OperandTopN: {
		NewRuleInjectProjectionBelowTopN(),
	},
}

type baseRule struct {
	pattern *memo.Pattern
}

// Match implements Transformation Interface.
func (r *baseRule) Match(expr *memo.ExprIter) bool {
	return true
}

// GetPattern implements Transformation Interface.
func (r *baseRule) GetPattern() *memo.Pattern {
	return r.pattern
}

// PushSelDownTableScan pushes the selection down to TableScan.
type PushSelDownTableScan struct {
	baseRule
}

// NewRulePushSelDownTableScan creates a new Transformation PushSelDownTableScan.
// The pattern of this rule is: `Selection -> TableScan`
func NewRulePushSelDownTableScan() Transformation {
	rule := &PushSelDownTableScan{}
	ts := memo.NewPattern(memo.OperandTableScan, memo.EngineTiKVOrTiFlash)
	p := memo.BuildPattern(memo.OperandSelection, memo.EngineTiKVOrTiFlash, ts)
	rule.pattern = p
	return rule
}

// OnTransform implements Transformation interface.
//
// It transforms `sel -> ts` to one of the following new exprs:
// 1. `newSel -> newTS`
// 2. `newTS`
//
// Filters of the old `sel` operator are removed if they are used to calculate
// the key ranges of the `ts` operator.
func (r *PushSelDownTableScan) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	sel := old.GetExpr().ExprNode.(*plannercore.LogicalSelection)
	ts := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalTableScan)
	if ts.Handle == nil {
		return nil, false, false, nil
	}
	accesses, remained := ranger.DetachCondsForColumn(ts.SCtx(), sel.Conditions, ts.Handle)
	if accesses == nil {
		return nil, false, false, nil
	}
	newTblScan := plannercore.LogicalTableScan{
		Source:      ts.Source,
		Handle:      ts.Handle,
		AccessConds: ts.AccessConds.Shallow(),
	}.Init(ts.SCtx(), ts.SelectBlockOffset())
	newTblScan.AccessConds = append(newTblScan.AccessConds, accesses...)
	tblScanExpr := memo.NewGroupExpr(newTblScan)
	if len(remained) == 0 {
		// `sel -> ts` is transformed to `newTS`.
		return []*memo.GroupExpr{tblScanExpr}, true, false, nil
	}
	schema := old.GetExpr().Group.Prop.Schema
	tblScanGroup := memo.NewGroupWithSchema(tblScanExpr, schema)
	newSel := plannercore.LogicalSelection{Conditions: remained}.Init(sel.SCtx(), sel.SelectBlockOffset())
	selExpr := memo.NewGroupExpr(newSel)
	selExpr.Children = append(selExpr.Children, tblScanGroup)
	// `sel -> ts` is transformed to `newSel ->newTS`.
	return []*memo.GroupExpr{selExpr}, true, false, nil
}

// PushSelDownIndexScan pushes a Selection down to IndexScan.
type PushSelDownIndexScan struct {
	baseRule
}

// NewRulePushSelDownIndexScan creates a new Transformation PushSelDownIndexScan.
// The pattern of this rule is `Selection -> IndexScan`.
func NewRulePushSelDownIndexScan() Transformation {
	rule := &PushSelDownIndexScan{}
	rule.pattern = memo.BuildPattern(
		memo.OperandSelection,
		memo.EngineTiKVOnly,
		memo.NewPattern(memo.OperandIndexScan, memo.EngineTiKVOnly),
	)
	return rule
}

// OnTransform implements Transformation interface.
// It will transform `Selection -> IndexScan` to:
//   `IndexScan(with a new access range)` or
//   `Selection -> IndexScan(with a new access range)`
//	 or just keep the two GroupExprs unchanged.
func (r *PushSelDownIndexScan) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	sel := old.GetExpr().ExprNode.(*plannercore.LogicalSelection)
	is := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalIndexScan)
	if len(is.IdxCols) == 0 {
		return nil, false, false, nil
	}
	conditions := sel.Conditions
	if is.AccessConds != nil {
		// If we have already pushed some conditions down here,
		// we merge old AccessConds with new conditions,
		// to make sure this rule can be applied more than once.
		conditions = make([]expression.Expression, len(sel.Conditions)+len(is.AccessConds))
		copy(conditions, sel.Conditions)
		copy(conditions[len(sel.Conditions):], is.AccessConds)
	}
	res, err := ranger.DetachCondAndBuildRangeForIndex(is.SCtx(), conditions, is.IdxCols, is.IdxColLens)
	if err != nil {
		return nil, false, false, err
	}
	if len(res.AccessConds) == len(is.AccessConds) {
		// There is no condition can be pushed down as range,
		// or the pushed down conditions are the same with before.
		sameConds := true
		for i := range res.AccessConds {
			if !res.AccessConds[i].Equal(is.SCtx(), is.AccessConds[i]) {
				sameConds = false
				break
			}
		}
		if sameConds {
			return nil, false, false, nil
		}
	}
	// TODO: `res` still has some unused fields: EqOrInCount, IsDNFCond.
	newIs := plannercore.LogicalIndexScan{
		Source:         is.Source,
		IsDoubleRead:   is.IsDoubleRead,
		EqCondCount:    res.EqCondCount,
		AccessConds:    res.AccessConds,
		Ranges:         res.Ranges,
		Index:          is.Index,
		Columns:        is.Columns,
		FullIdxCols:    is.FullIdxCols,
		FullIdxColLens: is.FullIdxColLens,
		IdxCols:        is.IdxCols,
		IdxColLens:     is.IdxColLens,
	}.Init(is.SCtx(), is.SelectBlockOffset())
	isExpr := memo.NewGroupExpr(newIs)

	if len(res.RemainedConds) == 0 {
		return []*memo.GroupExpr{isExpr}, true, false, nil
	}
	isGroup := memo.NewGroupWithSchema(isExpr, old.Children[0].GetExpr().Group.Prop.Schema)
	newSel := plannercore.LogicalSelection{Conditions: res.RemainedConds}.Init(sel.SCtx(), sel.SelectBlockOffset())
	selExpr := memo.NewGroupExpr(newSel)
	selExpr.SetChildren(isGroup)
	return []*memo.GroupExpr{selExpr}, true, false, nil
}

// PushSelDownTiKVSingleGather pushes the selection down to child of TiKVSingleGather.
type PushSelDownTiKVSingleGather struct {
	baseRule
}

// NewRulePushSelDownTiKVSingleGather creates a new Transformation PushSelDownTiKVSingleGather.
// The pattern of this rule is `Selection -> TiKVSingleGather -> Any`.
func NewRulePushSelDownTiKVSingleGather() Transformation {
	any := memo.NewPattern(memo.OperandAny, memo.EngineTiKVOrTiFlash)
	tg := memo.BuildPattern(memo.OperandTiKVSingleGather, memo.EngineTiDBOnly, any)
	p := memo.BuildPattern(memo.OperandSelection, memo.EngineTiDBOnly, tg)

	rule := &PushSelDownTiKVSingleGather{}
	rule.pattern = p
	return rule
}

// OnTransform implements Transformation interface.
//
// It transforms `oldSel -> oldTg -> any` to one of the following new exprs:
// 1. `newTg -> pushedSel -> any`
// 2. `remainedSel -> newTg -> pushedSel -> any`
func (r *PushSelDownTiKVSingleGather) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	sel := old.GetExpr().ExprNode.(*plannercore.LogicalSelection)
	sg := old.Children[0].GetExpr().ExprNode.(*plannercore.TiKVSingleGather)
	childGroup := old.Children[0].Children[0].Group
	var pushed, remained []expression.Expression
	sctx := sg.SCtx()
	pushed, remained = expression.PushDownExprs(sctx.GetSessionVars().StmtCtx, sel.Conditions, sctx.GetClient(), kv.TiKV)
	if len(pushed) == 0 {
		return nil, false, false, nil
	}
	pushedSel := plannercore.LogicalSelection{Conditions: pushed}.Init(sctx, sel.SelectBlockOffset())
	pushedSelExpr := memo.NewGroupExpr(pushedSel)
	pushedSelExpr.Children = append(pushedSelExpr.Children, childGroup)
	pushedSelGroup := memo.NewGroupWithSchema(pushedSelExpr, childGroup.Prop.Schema).SetEngineType(childGroup.EngineType)
	// The field content of TiKVSingleGather would not be modified currently, so we
	// just reference the same tg instead of making a copy of it.
	//
	// TODO: if we save pushed filters later in TiKVSingleGather, in order to do partition
	//       pruning or skyline pruning, we need to make a copy of the TiKVSingleGather here.
	tblGatherExpr := memo.NewGroupExpr(sg)
	tblGatherExpr.Children = append(tblGatherExpr.Children, pushedSelGroup)
	if len(remained) == 0 {
		// `oldSel -> oldTg -> any` is transformed to `newTg -> pushedSel -> any`.
		return []*memo.GroupExpr{tblGatherExpr}, true, false, nil
	}
	tblGatherGroup := memo.NewGroupWithSchema(tblGatherExpr, pushedSelGroup.Prop.Schema)
	remainedSel := plannercore.LogicalSelection{Conditions: remained}.Init(sel.SCtx(), sel.SelectBlockOffset())
	remainedSelExpr := memo.NewGroupExpr(remainedSel)
	remainedSelExpr.Children = append(remainedSelExpr.Children, tblGatherGroup)
	// `oldSel -> oldTg -> any` is transformed to `remainedSel -> newTg -> pushedSel -> any`.
	return []*memo.GroupExpr{remainedSelExpr}, true, false, nil
}

// EnumeratePaths converts DataSource to table scan and index scans.
type EnumeratePaths struct {
	baseRule
}

// NewRuleEnumeratePaths creates a new Transformation EnumeratePaths.
// The pattern of this rule is: `DataSource`.
func NewRuleEnumeratePaths() Transformation {
	rule := &EnumeratePaths{}
	rule.pattern = memo.NewPattern(memo.OperandDataSource, memo.EngineTiDBOnly)
	return rule
}

// OnTransform implements Transformation interface.
func (r *EnumeratePaths) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	ds := old.GetExpr().ExprNode.(*plannercore.DataSource)
	gathers := ds.Convert2Gathers()
	for _, gather := range gathers {
		expr := memo.Convert2GroupExpr(gather)
		expr.Children[0].SetEngineType(memo.EngineTiKV)
		newExprs = append(newExprs, expr)
	}
	return newExprs, true, false, nil
}

// PushAggDownGather splits Aggregation to two stages, final and partial1,
// and pushed the partial Aggregation down to the child of TiKVSingleGather.
type PushAggDownGather struct {
	baseRule
}

// NewRulePushAggDownGather creates a new Transformation PushAggDownGather.
// The pattern of this rule is: `Aggregation -> TiKVSingleGather`.
func NewRulePushAggDownGather() Transformation {
	rule := &PushAggDownGather{}
	rule.pattern = memo.BuildPattern(
		memo.OperandAggregation,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandTiKVSingleGather, memo.EngineTiDBOnly),
	)
	return rule
}

// Match implements Transformation interface.
func (r *PushAggDownGather) Match(expr *memo.ExprIter) bool {
	if expr.GetExpr().HasAppliedRule(r) {
		return false
	}
	agg := expr.GetExpr().ExprNode.(*plannercore.LogicalAggregation)
	for _, aggFunc := range agg.AggFuncs {
		if aggFunc.Mode != aggregation.CompleteMode {
			return false
		}
	}
	if agg.HasDistinct() {
		// TODO: remove this logic after the cost estimation of distinct pushdown is implemented.
		// If AllowDistinctAggPushDown is set to true, we should not consider RootTask.
		if !agg.SCtx().GetSessionVars().AllowDistinctAggPushDown {
			return false
		}
	}
	childEngine := expr.Children[0].GetExpr().Children[0].EngineType
	if childEngine != memo.EngineTiKV {
		// TODO: Remove this check when we have implemented TiFlashAggregation.
		return false
	}
	return plannercore.CheckAggCanPushCop(agg.SCtx(), agg.AggFuncs, agg.GroupByItems, kv.TiKV)
}

// OnTransform implements Transformation interface.
// It will transform `Agg->Gather` to `Agg(Final) -> Gather -> Agg(Partial1)`.
func (r *PushAggDownGather) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	agg := old.GetExpr().ExprNode.(*plannercore.LogicalAggregation)
	aggSchema := old.GetExpr().Group.Prop.Schema
	gather := old.Children[0].GetExpr().ExprNode.(*plannercore.TiKVSingleGather)
	childGroup := old.Children[0].GetExpr().Children[0]
	// The old Aggregation should stay unchanged for other transformation.
	// So we build a new LogicalAggregation for the partialAgg.
	aggFuncs := make([]*aggregation.AggFuncDesc, len(agg.AggFuncs))
	for i := range agg.AggFuncs {
		aggFuncs[i] = agg.AggFuncs[i].Clone()
	}
	gbyItems := make([]expression.Expression, len(agg.GroupByItems))
	copy(gbyItems, agg.GroupByItems)

	partialPref, finalPref, funcMap := plannercore.BuildFinalModeAggregation(agg.SCtx(),
		&plannercore.AggInfo{
			AggFuncs:     aggFuncs,
			GroupByItems: gbyItems,
			Schema:       aggSchema,
		}, true)
	// Remove unnecessary FirstRow.
	partialPref.AggFuncs =
		plannercore.RemoveUnnecessaryFirstRow(agg.SCtx(), finalPref.AggFuncs, finalPref.GroupByItems, partialPref.AggFuncs, partialPref.GroupByItems, partialPref.Schema, funcMap)

	partialAgg := plannercore.LogicalAggregation{
		AggFuncs:     partialPref.AggFuncs,
		GroupByItems: partialPref.GroupByItems,
	}.Init(agg.SCtx(), agg.SelectBlockOffset())
	partialAgg.CopyAggHints(agg)

	finalAgg := plannercore.LogicalAggregation{
		AggFuncs:     finalPref.AggFuncs,
		GroupByItems: finalPref.GroupByItems,
	}.Init(agg.SCtx(), agg.SelectBlockOffset())
	finalAgg.CopyAggHints(agg)

	partialAggExpr := memo.NewGroupExpr(partialAgg)
	partialAggExpr.SetChildren(childGroup)
	partialAggGroup := memo.NewGroupWithSchema(partialAggExpr, partialPref.Schema).SetEngineType(childGroup.EngineType)
	gatherExpr := memo.NewGroupExpr(gather)
	gatherExpr.SetChildren(partialAggGroup)
	gatherGroup := memo.NewGroupWithSchema(gatherExpr, partialPref.Schema)
	finalAggExpr := memo.NewGroupExpr(finalAgg)
	finalAggExpr.SetChildren(gatherGroup)
	finalAggExpr.AddAppliedRule(r)
	// We don't erase the old complete mode Aggregation because
	// this transformation would not always be better.
	return []*memo.GroupExpr{finalAggExpr}, false, false, nil
}

// PushSelDownSort pushes the Selection down to the child of Sort.
type PushSelDownSort struct {
	baseRule
}

// NewRulePushSelDownSort creates a new Transformation PushSelDownSort.
// The pattern of this rule is: `Selection -> Sort`.
func NewRulePushSelDownSort() Transformation {
	rule := &PushSelDownSort{}
	rule.pattern = memo.BuildPattern(
		memo.OperandSelection,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandSort, memo.EngineTiDBOnly),
	)
	return rule
}

// OnTransform implements Transformation interface.
// It will transform `sel->sort->x` to `sort->sel->x`.
func (r *PushSelDownSort) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	sel := old.GetExpr().ExprNode.(*plannercore.LogicalSelection)
	sort := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalSort)
	childGroup := old.Children[0].GetExpr().Children[0]

	newSelExpr := memo.NewGroupExpr(sel)
	newSelExpr.Children = append(newSelExpr.Children, childGroup)
	newSelGroup := memo.NewGroupWithSchema(newSelExpr, childGroup.Prop.Schema)

	newSortExpr := memo.NewGroupExpr(sort)
	newSortExpr.Children = append(newSortExpr.Children, newSelGroup)
	return []*memo.GroupExpr{newSortExpr}, true, false, nil
}

// PushSelDownProjection pushes the Selection down to the child of Projection.
type PushSelDownProjection struct {
	baseRule
}

// NewRulePushSelDownProjection creates a new Transformation PushSelDownProjection.
// The pattern of this rule is: `Selection -> Projection`.
func NewRulePushSelDownProjection() Transformation {
	rule := &PushSelDownProjection{}
	rule.pattern = memo.BuildPattern(
		memo.OperandSelection,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandProjection, memo.EngineTiDBOnly),
	)
	return rule
}

// OnTransform implements Transformation interface.
// It will transform `selection -> projection -> x` to
// 1. `projection -> selection -> x` or
// 2. `selection -> projection -> selection -> x` or
// 3. just keep unchanged.
func (r *PushSelDownProjection) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	sel := old.GetExpr().ExprNode.(*plannercore.LogicalSelection)
	proj := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalProjection)
	projSchema := old.Children[0].Prop.Schema
	childGroup := old.Children[0].GetExpr().Children[0]
	for _, expr := range proj.Exprs {
		if expression.HasAssignSetVarFunc(expr) {
			return nil, false, false, nil
		}
	}
	canBePushed := make([]expression.Expression, 0, len(sel.Conditions))
	canNotBePushed := make([]expression.Expression, 0, len(sel.Conditions))
	for _, cond := range sel.Conditions {
		if !expression.HasGetSetVarFunc(cond) {
			canBePushed = append(canBePushed, expression.ColumnSubstitute(cond, projSchema, proj.Exprs))
		} else {
			canNotBePushed = append(canNotBePushed, cond)
		}
	}
	if len(canBePushed) == 0 {
		return nil, false, false, nil
	}
	newBottomSel := plannercore.LogicalSelection{Conditions: canBePushed}.Init(sel.SCtx(), sel.SelectBlockOffset())
	newBottomSelExpr := memo.NewGroupExpr(newBottomSel)
	newBottomSelExpr.SetChildren(childGroup)
	newBottomSelGroup := memo.NewGroupWithSchema(newBottomSelExpr, childGroup.Prop.Schema)
	newProjExpr := memo.NewGroupExpr(proj)
	newProjExpr.SetChildren(newBottomSelGroup)
	if len(canNotBePushed) == 0 {
		return []*memo.GroupExpr{newProjExpr}, true, false, nil
	}
	newProjGroup := memo.NewGroupWithSchema(newProjExpr, projSchema)
	newTopSel := plannercore.LogicalSelection{Conditions: canNotBePushed}.Init(sel.SCtx(), sel.SelectBlockOffset())
	newTopSelExpr := memo.NewGroupExpr(newTopSel)
	newTopSelExpr.SetChildren(newProjGroup)
	return []*memo.GroupExpr{newTopSelExpr}, true, false, nil
}

// PushSelDownAggregation pushes Selection down to the child of Aggregation.
type PushSelDownAggregation struct {
	baseRule
}

// NewRulePushSelDownAggregation creates a new Transformation PushSelDownAggregation.
// The pattern of this rule is `Selection -> Aggregation`.
func NewRulePushSelDownAggregation() Transformation {
	rule := &PushSelDownAggregation{}
	rule.pattern = memo.BuildPattern(
		memo.OperandSelection,
		memo.EngineAll,
		memo.NewPattern(memo.OperandAggregation, memo.EngineAll),
	)
	return rule
}

// OnTransform implements Transformation interface.
// It will transform `sel->agg->x` to `agg->sel->x` or `sel->agg->sel->x`
// or just keep the selection unchanged.
func (r *PushSelDownAggregation) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	sel := old.GetExpr().ExprNode.(*plannercore.LogicalSelection)
	agg := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalAggregation)
	aggSchema := old.Children[0].Prop.Schema
	var pushedExprs []expression.Expression
	var remainedExprs []expression.Expression
	exprsOriginal := make([]expression.Expression, 0, len(agg.AggFuncs))
	for _, aggFunc := range agg.AggFuncs {
		exprsOriginal = append(exprsOriginal, aggFunc.Args[0])
	}
	groupByColumns := expression.NewSchema(agg.GetGroupByCols()...)
	for _, cond := range sel.Conditions {
		switch cond.(type) {
		case *expression.Constant:
			// Consider SQL list "select sum(b) from t group by a having 1=0". "1=0" is a constant predicate which should be
			// retained and pushed down at the same time. Because we will get a wrong query result that contains one column
			// with value 0 rather than an empty query result.
			pushedExprs = append(pushedExprs, cond)
			remainedExprs = append(remainedExprs, cond)
		case *expression.ScalarFunction:
			extractedCols := expression.ExtractColumns(cond)
			canPush := true
			for _, col := range extractedCols {
				if !groupByColumns.Contains(col) {
					canPush = false
					break
				}
			}
			if canPush {
				pushedExprs = append(pushedExprs, cond)
			} else {
				remainedExprs = append(remainedExprs, cond)
			}
		default:
			remainedExprs = append(remainedExprs, cond)
		}
	}
	// If no condition can be pushed, keep the selection unchanged.
	if len(pushedExprs) == 0 {
		return nil, false, false, nil
	}
	sctx := sel.SCtx()
	childGroup := old.Children[0].GetExpr().Children[0]
	pushedSel := plannercore.LogicalSelection{Conditions: pushedExprs}.Init(sctx, sel.SelectBlockOffset())
	pushedGroupExpr := memo.NewGroupExpr(pushedSel)
	pushedGroupExpr.SetChildren(childGroup)
	pushedGroup := memo.NewGroupWithSchema(pushedGroupExpr, childGroup.Prop.Schema)

	aggGroupExpr := memo.NewGroupExpr(agg)
	aggGroupExpr.SetChildren(pushedGroup)

	if len(remainedExprs) == 0 {
		return []*memo.GroupExpr{aggGroupExpr}, true, false, nil
	}

	aggGroup := memo.NewGroupWithSchema(aggGroupExpr, aggSchema)
	remainedSel := plannercore.LogicalSelection{Conditions: remainedExprs}.Init(sctx, sel.SelectBlockOffset())
	remainedGroupExpr := memo.NewGroupExpr(remainedSel)
	remainedGroupExpr.SetChildren(aggGroup)
	return []*memo.GroupExpr{remainedGroupExpr}, true, false, nil
}

// PushSelDownWindow pushes Selection down to the child of Window.
type PushSelDownWindow struct {
	baseRule
}

// NewRulePushSelDownWindow creates a new Transformation PushSelDownWindow.
// The pattern of this rule is `Selection -> Window`.
func NewRulePushSelDownWindow() Transformation {
	rule := &PushSelDownWindow{}
	rule.pattern = memo.BuildPattern(
		memo.OperandSelection,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandWindow, memo.EngineAll),
	)
	return rule
}

// OnTransform implements Transformation interface.
// This rule will transform `sel -> window -> x` to
// 1. `window -> sel -> x` or
// 2. `sel -> window -> sel -> x` or
// 3. just keep unchanged.
func (r *PushSelDownWindow) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	sel := old.GetExpr().ExprNode.(*plannercore.LogicalSelection)
	window := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalWindow)
	windowSchema := old.Children[0].Prop.Schema
	childGroup := old.Children[0].GetExpr().Children[0]
	canBePushed := make([]expression.Expression, 0, len(sel.Conditions))
	canNotBePushed := make([]expression.Expression, 0, len(sel.Conditions))

	// get partition Columns' Schema
	partitionColsSchema := expression.NewSchema(window.GetPartitionByCols()...)

	for _, cond := range sel.Conditions {
		if expression.ExprFromSchema(cond, partitionColsSchema) {
			canBePushed = append(canBePushed, cond)
		} else {
			canNotBePushed = append(canNotBePushed, cond)
		}
	}
	// Nothing can be pushed!
	if len(canBePushed) == 0 {
		return nil, false, false, nil
	}

	// construct return GroupExpr
	newBottomSel := plannercore.LogicalSelection{Conditions: canBePushed}.Init(sel.SCtx(), sel.SelectBlockOffset())
	newBottomSelExpr := memo.NewGroupExpr(newBottomSel)
	newBottomSelExpr.SetChildren(childGroup)
	newBottomSelGroup := memo.NewGroupWithSchema(newBottomSelExpr, childGroup.Prop.Schema)
	newWindowExpr := memo.NewGroupExpr(window)
	newWindowExpr.SetChildren(newBottomSelGroup)
	if len(canNotBePushed) == 0 {
		return []*memo.GroupExpr{newWindowExpr}, true, false, nil
	}

	newWindowGroup := memo.NewGroupWithSchema(newWindowExpr, windowSchema)
	newTopSel := plannercore.LogicalSelection{Conditions: canNotBePushed}.Init(sel.SCtx(), sel.SelectBlockOffset())
	newTopSelExpr := memo.NewGroupExpr(newTopSel)
	newTopSelExpr.SetChildren(newWindowGroup)
	return []*memo.GroupExpr{newTopSelExpr}, true, false, nil
}

// TransformLimitToTopN transforms Limit+Sort to TopN.
type TransformLimitToTopN struct {
	baseRule
}

// NewRuleTransformLimitToTopN creates a new Transformation TransformLimitToTopN.
// The pattern of this rule is `Limit -> Sort`.
func NewRuleTransformLimitToTopN() Transformation {
	rule := &TransformLimitToTopN{}
	rule.pattern = memo.BuildPattern(
		memo.OperandLimit,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandSort, memo.EngineTiDBOnly),
	)
	return rule
}

// OnTransform implements Transformation interface.
// This rule will transform `Limit -> Sort -> x` to `TopN -> x`.
func (r *TransformLimitToTopN) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	limit := old.GetExpr().ExprNode.(*plannercore.LogicalLimit)
	sort := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalSort)
	childGroup := old.Children[0].GetExpr().Children[0]
	topN := plannercore.LogicalTopN{
		ByItems: sort.ByItems,
		Offset:  limit.Offset,
		Count:   limit.Count,
	}.Init(limit.SCtx(), limit.SelectBlockOffset())
	topNExpr := memo.NewGroupExpr(topN)
	topNExpr.SetChildren(childGroup)
	return []*memo.GroupExpr{topNExpr}, true, false, nil
}

// PushLimitDownProjection pushes Limit to Projection.
type PushLimitDownProjection struct {
	baseRule
}

// NewRulePushLimitDownProjection creates a new Transformation.
// The pattern of this rule is `Limit->Projection->X` to `Projection->Limit->X`.
func NewRulePushLimitDownProjection() Transformation {
	rule := &PushLimitDownProjection{}
	rule.pattern = memo.BuildPattern(
		memo.OperandLimit,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandProjection, memo.EngineTiDBOnly),
	)
	return rule
}

// Match implements Transformation interface.
func (r *PushLimitDownProjection) Match(expr *memo.ExprIter) bool {
	proj := expr.Children[0].GetExpr().ExprNode.(*plannercore.LogicalProjection)
	for _, expr := range proj.Exprs {
		if expression.HasAssignSetVarFunc(expr) {
			return false
		}
	}
	return true
}

// OnTransform implements Transformation interface.
// This rule tries to pushes the Limit through Projection.
func (r *PushLimitDownProjection) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	limit := old.GetExpr().ExprNode.(*plannercore.LogicalLimit)
	proj := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalProjection)
	childGroup := old.Children[0].GetExpr().Children[0]

	projExpr := memo.NewGroupExpr(proj)
	limitExpr := memo.NewGroupExpr(limit)
	limitExpr.SetChildren(childGroup)
	limitGroup := memo.NewGroupWithSchema(limitExpr, childGroup.Prop.Schema)
	projExpr.SetChildren(limitGroup)
	return []*memo.GroupExpr{projExpr}, true, false, nil
}

// PushLimitDownUnionAll pushes limit to union all.
type PushLimitDownUnionAll struct {
	baseRule
}

// NewRulePushLimitDownUnionAll creates a new Transformation PushLimitDownUnionAll.
// The pattern of this rule is `Limit->UnionAll->X`.
func NewRulePushLimitDownUnionAll() Transformation {
	rule := &PushLimitDownUnionAll{}
	rule.pattern = memo.BuildPattern(
		memo.OperandLimit,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandUnionAll, memo.EngineTiDBOnly),
	)
	return rule
}

// Match implements Transformation interface.
// Use appliedRuleSet in GroupExpr to avoid re-apply rules.
func (r *PushLimitDownUnionAll) Match(expr *memo.ExprIter) bool {
	return !expr.GetExpr().HasAppliedRule(r)
}

// OnTransform implements Transformation interface.
// It will transform `Limit->UnionAll->X` to `Limit->UnionAll->Limit->X`.
func (r *PushLimitDownUnionAll) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	limit := old.GetExpr().ExprNode.(*plannercore.LogicalLimit)
	unionAll := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalUnionAll)
	unionAllSchema := old.Children[0].Group.Prop.Schema

	newLimit := plannercore.LogicalLimit{
		Count: limit.Count + limit.Offset,
	}.Init(limit.SCtx(), limit.SelectBlockOffset())

	newUnionAllExpr := memo.NewGroupExpr(unionAll)
	for _, childGroup := range old.Children[0].GetExpr().Children {
		newLimitExpr := memo.NewGroupExpr(newLimit)
		newLimitExpr.Children = append(newLimitExpr.Children, childGroup)
		newLimitGroup := memo.NewGroupWithSchema(newLimitExpr, childGroup.Prop.Schema)

		newUnionAllExpr.Children = append(newUnionAllExpr.Children, newLimitGroup)
	}

	newLimitExpr := memo.NewGroupExpr(limit)
	newUnionAllGroup := memo.NewGroupWithSchema(newUnionAllExpr, unionAllSchema)
	newLimitExpr.SetChildren(newUnionAllGroup)
	newLimitExpr.AddAppliedRule(r)
	return []*memo.GroupExpr{newLimitExpr}, true, false, nil
}

// PushSelDownJoin pushes Selection through Join.
type PushSelDownJoin struct {
	baseRule
}

// NewRulePushSelDownJoin creates a new Transformation PushSelDownJoin.
// The pattern of this rule is `Selection -> Join`.
func NewRulePushSelDownJoin() Transformation {
	rule := &PushSelDownJoin{}
	rule.pattern = memo.BuildPattern(
		memo.OperandSelection,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandJoin, memo.EngineTiDBOnly),
	)
	return rule
}

// Match implements Transformation interface.
func (r *PushSelDownJoin) Match(expr *memo.ExprIter) bool {
	return !expr.GetExpr().HasAppliedRule(r)
}

// buildChildSelectionGroup builds a new childGroup if the pushed down condition is not empty.
func buildChildSelectionGroup(
	oldSel *plannercore.LogicalSelection,
	conditions []expression.Expression,
	childGroup *memo.Group) *memo.Group {
	if len(conditions) == 0 {
		return childGroup
	}
	newSel := plannercore.LogicalSelection{Conditions: conditions}.Init(oldSel.SCtx(), oldSel.SelectBlockOffset())
	groupExpr := memo.NewGroupExpr(newSel)
	groupExpr.SetChildren(childGroup)
	newChild := memo.NewGroupWithSchema(groupExpr, childGroup.Prop.Schema)
	return newChild
}

// OnTransform implements Transformation interface.
// This rule tries to pushes the Selection through Join. Besides, this rule fulfills the `XXXConditions` field of Join.
func (r *PushSelDownJoin) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	sel := old.GetExpr().ExprNode.(*plannercore.LogicalSelection)
	joinExpr := old.Children[0].GetExpr()
	// TODO: we need to create a new LogicalJoin here.
	join := joinExpr.ExprNode.(*plannercore.LogicalJoin)
	sctx := sel.SCtx()
	leftGroup := old.Children[0].GetExpr().Children[0]
	rightGroup := old.Children[0].GetExpr().Children[1]
	var equalCond []*expression.ScalarFunction
	var leftPushCond, rightPushCond, otherCond, leftCond, rightCond, remainCond []expression.Expression
	switch join.JoinType {
	case plannercore.SemiJoin, plannercore.InnerJoin:
		tempCond := make([]expression.Expression, 0,
			len(join.LeftConditions)+len(join.RightConditions)+len(join.EqualConditions)+len(join.OtherConditions)+len(sel.Conditions))
		tempCond = append(tempCond, join.LeftConditions...)
		tempCond = append(tempCond, join.RightConditions...)
		tempCond = append(tempCond, expression.ScalarFuncs2Exprs(join.EqualConditions)...)
		tempCond = append(tempCond, join.OtherConditions...)
		tempCond = append(tempCond, sel.Conditions...)
		tempCond = expression.ExtractFiltersFromDNFs(sctx, tempCond)
		tempCond = expression.PropagateConstant(sctx, tempCond)
		// Return table dual when filter is constant false or null.
		dual := plannercore.Conds2TableDual(join, tempCond)
		if dual != nil {
			return []*memo.GroupExpr{memo.NewGroupExpr(dual)}, false, true, nil
		}
		equalCond, leftPushCond, rightPushCond, otherCond = join.ExtractOnCondition(tempCond, leftGroup.Prop.Schema, rightGroup.Prop.Schema, true, true)
		join.LeftConditions = nil
		join.RightConditions = nil
		join.EqualConditions = equalCond
		join.OtherConditions = otherCond
		leftCond = leftPushCond
		rightCond = rightPushCond
	case plannercore.LeftOuterJoin, plannercore.LeftOuterSemiJoin, plannercore.AntiLeftOuterSemiJoin,
		plannercore.RightOuterJoin:
		lenJoinConds := len(join.EqualConditions) + len(join.LeftConditions) + len(join.RightConditions) + len(join.OtherConditions)
		joinConds := make([]expression.Expression, 0, lenJoinConds)
		for _, equalCond := range join.EqualConditions {
			joinConds = append(joinConds, equalCond)
		}
		joinConds = append(joinConds, join.LeftConditions...)
		joinConds = append(joinConds, join.RightConditions...)
		joinConds = append(joinConds, join.OtherConditions...)
		join.EqualConditions = nil
		join.LeftConditions = nil
		join.RightConditions = nil
		join.OtherConditions = nil
		remainCond = make([]expression.Expression, len(sel.Conditions))
		copy(remainCond, sel.Conditions)
		nullSensitive := join.JoinType == plannercore.AntiLeftOuterSemiJoin || join.JoinType == plannercore.LeftOuterSemiJoin
		if join.JoinType == plannercore.RightOuterJoin {
			joinConds, remainCond = expression.PropConstOverOuterJoin(join.SCtx(), joinConds, remainCond, rightGroup.Prop.Schema, leftGroup.Prop.Schema, nullSensitive)
		} else {
			joinConds, remainCond = expression.PropConstOverOuterJoin(join.SCtx(), joinConds, remainCond, leftGroup.Prop.Schema, rightGroup.Prop.Schema, nullSensitive)
		}
		eq, left, right, other := join.ExtractOnCondition(joinConds, leftGroup.Prop.Schema, rightGroup.Prop.Schema, false, false)
		join.AppendJoinConds(eq, left, right, other)
		// Return table dual when filter is constant false or null.
		dual := plannercore.Conds2TableDual(join, remainCond)
		if dual != nil {
			return []*memo.GroupExpr{memo.NewGroupExpr(dual)}, false, true, nil
		}
		if join.JoinType == plannercore.RightOuterJoin {
			remainCond = expression.ExtractFiltersFromDNFs(join.SCtx(), remainCond)
			// Only derive right where condition, because left where condition cannot be pushed down
			equalCond, leftPushCond, rightPushCond, otherCond = join.ExtractOnCondition(remainCond, leftGroup.Prop.Schema, rightGroup.Prop.Schema, false, true)
			rightCond = rightPushCond
			// Handle join conditions, only derive left join condition, because right join condition cannot be pushed down
			derivedLeftJoinCond, _ := plannercore.DeriveOtherConditions(join, true, false)
			leftCond = append(join.LeftConditions, derivedLeftJoinCond...)
			join.LeftConditions = nil
			remainCond = append(expression.ScalarFuncs2Exprs(equalCond), otherCond...)
			remainCond = append(remainCond, leftPushCond...)
		} else {
			remainCond = expression.ExtractFiltersFromDNFs(join.SCtx(), remainCond)
			// Only derive left where condition, because right where condition cannot be pushed down
			equalCond, leftPushCond, rightPushCond, otherCond = join.ExtractOnCondition(remainCond, leftGroup.Prop.Schema, rightGroup.Prop.Schema, true, false)
			leftCond = leftPushCond
			// Handle join conditions, only derive left join condition, because right join condition cannot be pushed down
			_, derivedRightJoinCond := plannercore.DeriveOtherConditions(join, false, true)
			rightCond = append(join.RightConditions, derivedRightJoinCond...)
			join.RightConditions = nil
			remainCond = append(expression.ScalarFuncs2Exprs(equalCond), otherCond...)
			remainCond = append(remainCond, rightPushCond...)
		}
	default:
		// TODO: Enhance this rule to deal with Semi/SmiAnti Joins.
	}
	leftCond = expression.RemoveDupExprs(sctx, leftCond)
	rightCond = expression.RemoveDupExprs(sctx, rightCond)
	// TODO: Update EqualConditions like what we have done in the method join.updateEQCond() before.
	leftGroup = buildChildSelectionGroup(sel, leftCond, leftGroup)
	rightGroup = buildChildSelectionGroup(sel, rightCond, rightGroup)
	newJoinExpr := memo.NewGroupExpr(join)
	newJoinExpr.SetChildren(leftGroup, rightGroup)
	if len(remainCond) > 0 {
		newSel := plannercore.LogicalSelection{Conditions: remainCond}.Init(sctx, sel.SelectBlockOffset())
		newSel.Conditions = remainCond
		newSelExpr := memo.NewGroupExpr(newSel)
		newSelExpr.SetChildren(memo.NewGroupWithSchema(newJoinExpr, old.Children[0].Prop.Schema))
		newSelExpr.AddAppliedRule(r)
		return []*memo.GroupExpr{newSelExpr}, true, false, nil
	}
	return []*memo.GroupExpr{newJoinExpr}, true, false, nil
}

// PushSelDownUnionAll pushes selection through union all.
type PushSelDownUnionAll struct {
	baseRule
}

// NewRulePushSelDownUnionAll creates a new Transformation PushSelDownUnionAll.
// The pattern of this rule is `Selection -> UnionAll`.
func NewRulePushSelDownUnionAll() Transformation {
	rule := &PushSelDownUnionAll{}
	rule.pattern = memo.BuildPattern(
		memo.OperandSelection,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandUnionAll, memo.EngineTiDBOnly),
	)
	return rule
}

// OnTransform implements Transformation interface.
// It will transform `Selection->UnionAll->x` to `UnionAll->Selection->x`.
func (r *PushSelDownUnionAll) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	sel := old.GetExpr().ExprNode.(*plannercore.LogicalSelection)
	unionAll := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalUnionAll)
	childGroups := old.Children[0].GetExpr().Children

	newUnionAllExpr := memo.NewGroupExpr(unionAll)
	for _, group := range childGroups {
		newSelExpr := memo.NewGroupExpr(sel)
		newSelExpr.Children = append(newSelExpr.Children, group)
		newSelGroup := memo.NewGroupWithSchema(newSelExpr, group.Prop.Schema)

		newUnionAllExpr.Children = append(newUnionAllExpr.Children, newSelGroup)
	}
	return []*memo.GroupExpr{newUnionAllExpr}, true, false, nil
}

// EliminateProjection eliminates the projection.
type EliminateProjection struct {
	baseRule
}

// NewRuleEliminateProjection creates a new Transformation EliminateProjection.
// The pattern of this rule is `Projection -> Any`.
func NewRuleEliminateProjection() Transformation {
	rule := &EliminateProjection{}
	rule.pattern = memo.BuildPattern(
		memo.OperandProjection,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandAny, memo.EngineTiDBOnly),
	)
	return rule
}

// OnTransform implements Transformation interface.
// This rule tries to eliminate the projection whose output columns are the same with its child.
func (r *EliminateProjection) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	child := old.Children[0]
	if child.Group.Prop.Schema.Len() != old.GetExpr().Group.Prop.Schema.Len() {
		return nil, false, false, nil
	}

	oldCols := old.GetExpr().Group.Prop.Schema.Columns
	for i, col := range child.Group.Prop.Schema.Columns {
		if !col.Equal(nil, oldCols[i]) {
			return nil, false, false, nil
		}
	}

	// Promote the children group's expression.
	finalGroupExprs := make([]*memo.GroupExpr, 0, child.Group.Equivalents.Len())
	for elem := child.Group.Equivalents.Front(); elem != nil; elem = elem.Next() {
		childExpr := elem.Value.(*memo.GroupExpr)
		copyChildExpr := memo.NewGroupExpr(childExpr.ExprNode)
		copyChildExpr.SetChildren(childExpr.Children...)
		finalGroupExprs = append(finalGroupExprs, copyChildExpr)
	}
	return finalGroupExprs, true, false, nil
}

// MergeAdjacentProjection merge the adjacent projection.
type MergeAdjacentProjection struct {
	baseRule
}

// NewRuleMergeAdjacentProjection creates a new Transformation MergeAdjacentProjection.
// The pattern of this rule is `Projection -> Projection`.
func NewRuleMergeAdjacentProjection() Transformation {
	rule := &MergeAdjacentProjection{}
	rule.pattern = memo.BuildPattern(
		memo.OperandProjection,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandProjection, memo.EngineTiDBOnly),
	)
	return rule
}

// OnTransform implements Transformation interface.
// It will transform `proj->proj->x` to `proj->x`
// or just keep the adjacent projections unchanged.
func (r *MergeAdjacentProjection) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	proj := old.GetExpr().ExprNode.(*plannercore.LogicalProjection)
	childGroup := old.Children[0].Group
	child := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalProjection)
	if plannercore.ExprsHasSideEffects(child.Exprs) {
		return nil, false, false, nil
	}

	replace := make(map[string]*expression.Column)
	for i, col := range childGroup.Prop.Schema.Columns {
		if colOrigin, ok := child.Exprs[i].(*expression.Column); ok {
			replace[string(col.HashCode(nil))] = colOrigin
		}
	}

	newProj := plannercore.LogicalProjection{Exprs: make([]expression.Expression, len(proj.Exprs))}.Init(proj.SCtx(), proj.SelectBlockOffset())
	newProj.SetSchema(old.GetExpr().Group.Prop.Schema)
	for i, expr := range proj.Exprs {
		newExpr := expr.Clone()
		plannercore.ResolveExprAndReplace(newExpr, replace)
		newProj.Exprs[i] = plannercore.ReplaceColumnOfExpr(newExpr, child, childGroup.Prop.Schema)
	}

	newProjExpr := memo.NewGroupExpr(newProj)
	newProjExpr.SetChildren(old.Children[0].GetExpr().Children[0])
	return []*memo.GroupExpr{newProjExpr}, true, false, nil
}

// PushTopNDownOuterJoin pushes topN to outer join.
type PushTopNDownOuterJoin struct {
	baseRule
}

// NewRulePushTopNDownOuterJoin creates a new Transformation PushTopNDownOuterJoin.
// The pattern of this rule is: `TopN -> Join`.
func NewRulePushTopNDownOuterJoin() Transformation {
	rule := &PushTopNDownOuterJoin{}
	rule.pattern = memo.BuildPattern(
		memo.OperandTopN,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandJoin, memo.EngineTiDBOnly),
	)
	return rule
}

// Match implements Transformation interface.
// Use appliedRuleSet in GroupExpr to avoid re-apply rules.
func (r *PushTopNDownOuterJoin) Match(expr *memo.ExprIter) bool {
	if expr.GetExpr().HasAppliedRule(r) {
		return false
	}
	join := expr.Children[0].GetExpr().ExprNode.(*plannercore.LogicalJoin)
	switch join.JoinType {
	case plannercore.LeftOuterJoin, plannercore.LeftOuterSemiJoin, plannercore.AntiLeftOuterSemiJoin, plannercore.RightOuterJoin:
		return true
	default:
		return false
	}
}

func pushTopNDownOuterJoinToChild(topN *plannercore.LogicalTopN, outerGroup *memo.Group) *memo.Group {
	for _, by := range topN.ByItems {
		cols := expression.ExtractColumns(by.Expr)
		for _, col := range cols {
			if !outerGroup.Prop.Schema.Contains(col) {
				return outerGroup
			}
		}
	}

	newTopN := plannercore.LogicalTopN{
		Count:   topN.Count + topN.Offset,
		ByItems: make([]*util.ByItems, len(topN.ByItems)),
	}.Init(topN.SCtx(), topN.SelectBlockOffset())

	for i := range topN.ByItems {
		newTopN.ByItems[i] = topN.ByItems[i].Clone()
	}
	newTopNGroup := memo.NewGroupExpr(newTopN)
	newTopNGroup.SetChildren(outerGroup)
	newChild := memo.NewGroupWithSchema(newTopNGroup, outerGroup.Prop.Schema)
	return newChild
}

// OnTransform implements Transformation interface.
// This rule will transform `TopN->OuterJoin->(OuterChild, InnerChild)` to `TopN->OuterJoin->(TopN->OuterChild, InnerChild)`
func (r *PushTopNDownOuterJoin) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	topN := old.GetExpr().ExprNode.(*plannercore.LogicalTopN)
	joinExpr := old.Children[0].GetExpr()
	join := joinExpr.ExprNode.(*plannercore.LogicalJoin)
	joinSchema := old.Children[0].Group.Prop.Schema
	leftGroup := joinExpr.Children[0]
	rightGroup := joinExpr.Children[1]

	switch join.JoinType {
	case plannercore.LeftOuterJoin, plannercore.LeftOuterSemiJoin, plannercore.AntiLeftOuterSemiJoin:
		leftGroup = pushTopNDownOuterJoinToChild(topN, leftGroup)
	case plannercore.RightOuterJoin:
		rightGroup = pushTopNDownOuterJoinToChild(topN, rightGroup)
	default:
		return nil, false, false, nil
	}

	newJoinExpr := memo.NewGroupExpr(join)
	newJoinExpr.SetChildren(leftGroup, rightGroup)
	newTopNExpr := memo.NewGroupExpr(topN)
	newTopNExpr.SetChildren(memo.NewGroupWithSchema(newJoinExpr, joinSchema))
	newTopNExpr.AddAppliedRule(r)
	return []*memo.GroupExpr{newTopNExpr}, true, false, nil
}

// PushTopNDownProjection pushes TopN to Projection.
type PushTopNDownProjection struct {
	baseRule
}

// NewRulePushTopNDownProjection creates a new Transformation PushTopNDownProjection.
// The pattern of this rule is `TopN->Projection->X` to `Projection->TopN->X`.
func NewRulePushTopNDownProjection() Transformation {
	rule := &PushTopNDownProjection{}
	rule.pattern = memo.BuildPattern(
		memo.OperandTopN,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandProjection, memo.EngineTiDBOnly),
	)
	return rule
}

// Match implements Transformation interface.
func (r *PushTopNDownProjection) Match(expr *memo.ExprIter) bool {
	proj := expr.Children[0].GetExpr().ExprNode.(*plannercore.LogicalProjection)
	for _, expr := range proj.Exprs {
		if expression.HasAssignSetVarFunc(expr) {
			return false
		}
	}
	return true
}

// OnTransform implements Transformation interface.
// This rule tries to pushes the TopN through Projection.
func (r *PushTopNDownProjection) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	topN := old.GetExpr().ExprNode.(*plannercore.LogicalTopN)
	proj := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalProjection)
	childGroup := old.Children[0].GetExpr().Children[0]

	newTopN := plannercore.LogicalTopN{
		Offset: topN.Offset,
		Count:  topN.Count,
	}.Init(topN.SCtx(), topN.SelectBlockOffset())

	newTopN.ByItems = make([]*util.ByItems, 0, len(topN.ByItems))
	for _, by := range topN.ByItems {
		newTopN.ByItems = append(newTopN.ByItems, &util.ByItems{
			Expr: expression.ColumnSubstitute(by.Expr, old.Children[0].Group.Prop.Schema, proj.Exprs),
			Desc: by.Desc,
		})
	}

	// remove meaningless constant sort items.
	for i := len(newTopN.ByItems) - 1; i >= 0; i-- {
		switch newTopN.ByItems[i].Expr.(type) {
		case *expression.Constant, *expression.CorrelatedColumn:
			topN.ByItems = append(newTopN.ByItems[:i], newTopN.ByItems[i+1:]...)
		}
	}
	projExpr := memo.NewGroupExpr(proj)
	topNExpr := memo.NewGroupExpr(newTopN)
	topNExpr.SetChildren(childGroup)
	topNGroup := memo.NewGroupWithSchema(topNExpr, childGroup.Prop.Schema)
	projExpr.SetChildren(topNGroup)
	return []*memo.GroupExpr{projExpr}, true, false, nil
}

// PushTopNDownUnionAll pushes topN to union all.
type PushTopNDownUnionAll struct {
	baseRule
}

// NewRulePushTopNDownUnionAll creates a new Transformation PushTopNDownUnionAll.
// The pattern of this rule is `TopN->UnionAll->X`.
func NewRulePushTopNDownUnionAll() Transformation {
	rule := &PushTopNDownUnionAll{}
	rule.pattern = memo.BuildPattern(
		memo.OperandTopN,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandUnionAll, memo.EngineTiDBOnly),
	)
	return rule
}

// Match implements Transformation interface.
// Use appliedRuleSet in GroupExpr to avoid re-apply rules.
func (r *PushTopNDownUnionAll) Match(expr *memo.ExprIter) bool {
	return !expr.GetExpr().HasAppliedRule(r)
}

// OnTransform implements Transformation interface.
// It will transform `TopN->UnionAll->X` to `TopN->UnionAll->TopN->X`.
func (r *PushTopNDownUnionAll) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	topN := old.GetExpr().ExprNode.(*plannercore.LogicalTopN)
	unionAll := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalUnionAll)

	newTopN := plannercore.LogicalTopN{
		Count:   topN.Count + topN.Offset,
		ByItems: topN.ByItems,
	}.Init(topN.SCtx(), topN.SelectBlockOffset())

	newUnionAllExpr := memo.NewGroupExpr(unionAll)
	for _, childGroup := range old.Children[0].GetExpr().Children {
		newTopNExpr := memo.NewGroupExpr(newTopN)
		newTopNExpr.Children = append(newTopNExpr.Children, childGroup)
		newTopNGroup := memo.NewGroupWithSchema(newTopNExpr, childGroup.Prop.Schema)

		newUnionAllExpr.Children = append(newUnionAllExpr.Children, newTopNGroup)
	}

	newTopNExpr := memo.NewGroupExpr(topN)
	newUnionAllGroup := memo.NewGroupWithSchema(newUnionAllExpr, unionAll.Schema())
	newTopNExpr.SetChildren(newUnionAllGroup)
	newTopNExpr.AddAppliedRule(r)
	return []*memo.GroupExpr{newTopNExpr}, true, false, nil
}

// PushTopNDownTiKVSingleGather pushes the top-n down to child of TiKVSingleGather.
type PushTopNDownTiKVSingleGather struct {
	baseRule
}

// NewRulePushTopNDownTiKVSingleGather creates a new Transformation PushTopNDownTiKVSingleGather.
// The pattern of this rule is `TopN -> TiKVSingleGather`.
func NewRulePushTopNDownTiKVSingleGather() Transformation {
	rule := &PushTopNDownTiKVSingleGather{}
	rule.pattern = memo.BuildPattern(
		memo.OperandTopN,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandTiKVSingleGather, memo.EngineTiDBOnly),
	)
	return rule
}

// Match implements Transformation interface.
// Use appliedRuleSet in GroupExpr to avoid re-apply rules.
func (r *PushTopNDownTiKVSingleGather) Match(expr *memo.ExprIter) bool {
	return !expr.GetExpr().HasAppliedRule(r)
}

// OnTransform implements Transformation interface.
// It transforms `TopN -> TiKVSingleGather` to `TopN(Final) -> TiKVSingleGather -> TopN(Partial)`.
func (r *PushTopNDownTiKVSingleGather) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	topN := old.GetExpr().ExprNode.(*plannercore.LogicalTopN)
	topNSchema := old.Children[0].Group.Prop.Schema
	gather := old.Children[0].GetExpr().ExprNode.(*plannercore.TiKVSingleGather)
	childGroup := old.Children[0].GetExpr().Children[0]

	particalTopN := plannercore.LogicalTopN{
		ByItems: topN.ByItems,
		Count:   topN.Count + topN.Offset,
	}.Init(topN.SCtx(), topN.SelectBlockOffset())
	partialTopNExpr := memo.NewGroupExpr(particalTopN)
	partialTopNExpr.SetChildren(childGroup)
	partialTopNGroup := memo.NewGroupWithSchema(partialTopNExpr, topNSchema).SetEngineType(childGroup.EngineType)

	gatherExpr := memo.NewGroupExpr(gather)
	gatherExpr.SetChildren(partialTopNGroup)
	gatherGroup := memo.NewGroupWithSchema(gatherExpr, topNSchema)

	finalTopNExpr := memo.NewGroupExpr(topN)
	finalTopNExpr.SetChildren(gatherGroup)
	finalTopNExpr.AddAppliedRule(r)
	return []*memo.GroupExpr{finalTopNExpr}, true, false, nil
}

// MergeAdjacentTopN merge adjacent TopN.
type MergeAdjacentTopN struct {
	baseRule
}

// NewRuleMergeAdjacentTopN creates a new Transformation MergeAdjacentTopN.
// The pattern of this rule is `TopN->TopN->X`.
func NewRuleMergeAdjacentTopN() Transformation {
	rule := &MergeAdjacentTopN{}
	rule.pattern = memo.BuildPattern(
		memo.OperandTopN,
		memo.EngineAll,
		memo.NewPattern(memo.OperandTopN, memo.EngineAll),
	)
	return rule
}

// Match implements Transformation interface.
func (r *MergeAdjacentTopN) Match(expr *memo.ExprIter) bool {
	topN := expr.GetExpr().ExprNode.(*plannercore.LogicalTopN)
	child := expr.Children[0].GetExpr().ExprNode.(*plannercore.LogicalTopN)

	// We can use this rule when the sort columns of parent TopN is a prefix of child TopN.
	if len(child.ByItems) < len(topN.ByItems) {
		return false
	}
	for i := 0; i < len(topN.ByItems); i++ {
		if !topN.ByItems[i].Equal(topN.SCtx(), child.ByItems[i]) {
			return false
		}
	}
	return true
}

// OnTransform implements Transformation interface.
// This rule tries to merge adjacent TopN.
func (r *MergeAdjacentTopN) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	topN := old.GetExpr().ExprNode.(*plannercore.LogicalTopN)
	child := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalTopN)
	childGroups := old.Children[0].GetExpr().Children

	if child.Count <= topN.Offset {
		tableDual := plannercore.LogicalTableDual{RowCount: 0}.Init(child.SCtx(), child.SelectBlockOffset())
		tableDual.SetSchema(old.GetExpr().Schema())
		tableDualExpr := memo.NewGroupExpr(tableDual)
		return []*memo.GroupExpr{tableDualExpr}, true, true, nil
	}

	offset := child.Offset + topN.Offset
	count := uint64(math.Min(float64(child.Count-topN.Offset), float64(topN.Count)))
	newTopN := plannercore.LogicalTopN{
		Count:   count,
		Offset:  offset,
		ByItems: child.ByItems,
	}.Init(child.SCtx(), child.SelectBlockOffset())
	newTopNExpr := memo.NewGroupExpr(newTopN)
	newTopNExpr.SetChildren(childGroups...)
	return []*memo.GroupExpr{newTopNExpr}, true, false, nil
}

// MergeAggregationProjection merges the Projection below an Aggregation as a new Aggregation.
// The Projection may be regenerated in the ImplementationPhase. But this rule allows the
// Aggregation to match other rules, such as MergeAdjacentAggregation.
type MergeAggregationProjection struct {
	baseRule
}

// NewRuleMergeAggregationProjection creates a new Transformation MergeAggregationProjection.
// The pattern of this rule is: `Aggregation -> Projection`.
func NewRuleMergeAggregationProjection() Transformation {
	rule := &MergeAggregationProjection{}
	rule.pattern = memo.BuildPattern(
		memo.OperandAggregation,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandProjection, memo.EngineTiDBOnly),
	)
	return rule
}

// Match implements Transformation interface.
func (r *MergeAggregationProjection) Match(old *memo.ExprIter) bool {
	proj := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalProjection)
	if plannercore.ExprsHasSideEffects(proj.Exprs) {
		return false
	}
	return true
}

// OnTransform implements Transformation interface.
// It will transform `Aggregation->Projection->X` to `Aggregation->X`.
func (r *MergeAggregationProjection) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	oldAgg := old.GetExpr().ExprNode.(*plannercore.LogicalAggregation)
	proj := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalProjection)
	projSchema := old.Children[0].GetExpr().Schema()

	groupByItems := make([]expression.Expression, len(oldAgg.GroupByItems))
	for i, item := range oldAgg.GroupByItems {
		groupByItems[i] = expression.ColumnSubstitute(item, projSchema, proj.Exprs)
	}

	aggFuncs := make([]*aggregation.AggFuncDesc, len(oldAgg.AggFuncs))
	for i, aggFunc := range oldAgg.AggFuncs {
		aggFuncs[i] = aggFunc.Clone()
		newArgs := make([]expression.Expression, len(aggFunc.Args))
		for j, arg := range aggFunc.Args {
			newArgs[j] = expression.ColumnSubstitute(arg, projSchema, proj.Exprs)
		}
		aggFuncs[i].Args = newArgs
	}

	newAgg := plannercore.LogicalAggregation{
		GroupByItems: groupByItems,
		AggFuncs:     aggFuncs,
	}.Init(oldAgg.SCtx(), oldAgg.SelectBlockOffset())

	newAggExpr := memo.NewGroupExpr(newAgg)
	newAggExpr.SetChildren(old.Children[0].GetExpr().Children...)
	return []*memo.GroupExpr{newAggExpr}, false, false, nil
}

// EliminateSingleMaxMin tries to convert a single max/min to Limit+Sort operators.
type EliminateSingleMaxMin struct {
	baseRule
}

// NewRuleEliminateSingleMaxMin creates a new Transformation EliminateSingleMaxMin.
// The pattern of this rule is `max/min->X`.
func NewRuleEliminateSingleMaxMin() Transformation {
	rule := &EliminateSingleMaxMin{}
	rule.pattern = memo.BuildPattern(
		memo.OperandAggregation,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandAny, memo.EngineTiDBOnly),
	)
	return rule
}

// Match implements Transformation interface.
func (r *EliminateSingleMaxMin) Match(expr *memo.ExprIter) bool {
	// Use appliedRuleSet in GroupExpr to avoid re-apply rules.
	if expr.GetExpr().HasAppliedRule(r) {
		return false
	}

	agg := expr.GetExpr().ExprNode.(*plannercore.LogicalAggregation)
	// EliminateSingleMaxMin only works on the complete mode.
	if !agg.IsCompleteModeAgg() {
		return false
	}
	if len(agg.GroupByItems) != 0 {
		return false
	}

	// If there is only one aggFunc, we don't need to guarantee that the child of it is a data
	// source, or whether the sort can be eliminated. This transformation won't be worse than previous.
	// Make sure that the aggFunc are Max or Min.
	// TODO: If there have only one Max or Min aggFunc and the other aggFuncs are FirstRow() can also use this rule. Waiting for the not null prop is maintained.
	if len(agg.AggFuncs) != 1 {
		return false
	}
	if agg.AggFuncs[0].Name != ast.AggFuncMax && agg.AggFuncs[0].Name != ast.AggFuncMin {
		return false
	}
	return true
}

// OnTransform implements Transformation interface.
// It will transform `max/min->X` to `max/min->top1->sel->X`.
func (r *EliminateSingleMaxMin) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	agg := old.GetExpr().ExprNode.(*plannercore.LogicalAggregation)
	childGroup := old.GetExpr().Children[0]
	ctx := agg.SCtx()
	f := agg.AggFuncs[0]

	// If there's no column in f.GetArgs()[0], we still need limit and read data from real table because the result should be NULL if the input is empty.
	if len(expression.ExtractColumns(f.Args[0])) > 0 {
		// If it can be NULL, we need to filter NULL out first.
		if !mysql.HasNotNullFlag(f.Args[0].GetType().Flag) {
			sel := plannercore.LogicalSelection{}.Init(ctx, agg.SelectBlockOffset())
			isNullFunc := expression.NewFunctionInternal(ctx, ast.IsNull, types.NewFieldType(mysql.TypeTiny), f.Args[0])
			notNullFunc := expression.NewFunctionInternal(ctx, ast.UnaryNot, types.NewFieldType(mysql.TypeTiny), isNullFunc)
			sel.Conditions = []expression.Expression{notNullFunc}
			selExpr := memo.NewGroupExpr(sel)
			selExpr.SetChildren(childGroup)
			selGroup := memo.NewGroupWithSchema(selExpr, childGroup.Prop.Schema)
			childGroup = selGroup
		}

		// Add top(1) operators.
		// For max function, the sort order should be desc.
		desc := f.Name == ast.AggFuncMax
		var byItems []*util.ByItems
		byItems = append(byItems, &util.ByItems{
			Expr: f.Args[0],
			Desc: desc,
		})
		top1 := plannercore.LogicalTopN{
			ByItems: byItems,
			Count:   1,
		}.Init(ctx, agg.SelectBlockOffset())
		top1Expr := memo.NewGroupExpr(top1)
		top1Expr.SetChildren(childGroup)
		top1Group := memo.NewGroupWithSchema(top1Expr, childGroup.Prop.Schema)
		childGroup = top1Group
	} else {
		li := plannercore.LogicalLimit{Count: 1}.Init(ctx, agg.SelectBlockOffset())
		liExpr := memo.NewGroupExpr(li)
		liExpr.SetChildren(childGroup)
		liGroup := memo.NewGroupWithSchema(liExpr, childGroup.Prop.Schema)
		childGroup = liGroup
	}

	newAgg := agg
	newAggExpr := memo.NewGroupExpr(newAgg)
	// If no data in the child, we need to return NULL instead of empty. This cannot be done by sort and limit themselves.
	// Since now there would be at most one row returned, the remained agg operator is not expensive anymore.
	newAggExpr.SetChildren(childGroup)
	newAggExpr.AddAppliedRule(r)
	return []*memo.GroupExpr{newAggExpr}, false, false, nil
}

// MergeAdjacentSelection merge adjacent selection.
type MergeAdjacentSelection struct {
	baseRule
}

// NewRuleMergeAdjacentSelection creates a new Transformation MergeAdjacentSelection.
// The pattern of this rule is `Selection->Selection->X`.
func NewRuleMergeAdjacentSelection() Transformation {
	rule := &MergeAdjacentSelection{}
	rule.pattern = memo.BuildPattern(
		memo.OperandSelection,
		memo.EngineAll,
		memo.NewPattern(memo.OperandSelection, memo.EngineAll),
	)
	return rule
}

// OnTransform implements Transformation interface.
// This rule tries to merge adjacent selection, with no simplification.
func (r *MergeAdjacentSelection) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	sel := old.GetExpr().ExprNode.(*plannercore.LogicalSelection)
	child := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalSelection)
	childGroups := old.Children[0].GetExpr().Children

	conditions := make([]expression.Expression, 0, len(sel.Conditions)+len(child.Conditions))
	conditions = append(conditions, sel.Conditions...)
	conditions = append(conditions, child.Conditions...)
	newSel := plannercore.LogicalSelection{Conditions: conditions}.Init(sel.SCtx(), sel.SelectBlockOffset())
	newSelExpr := memo.NewGroupExpr(newSel)
	newSelExpr.SetChildren(childGroups...)
	return []*memo.GroupExpr{newSelExpr}, true, false, nil
}

// MergeAdjacentLimit merge the adjacent limit.
type MergeAdjacentLimit struct {
	baseRule
}

// NewRuleMergeAdjacentLimit creates a new Transformation MergeAdjacentLimit.
// The pattern of this rule is `Limit->Limit->X`.
func NewRuleMergeAdjacentLimit() Transformation {
	rule := &MergeAdjacentLimit{}
	rule.pattern = memo.BuildPattern(
		memo.OperandLimit,
		memo.EngineAll,
		memo.NewPattern(memo.OperandLimit, memo.EngineAll),
	)
	return rule
}

// OnTransform implements Transformation interface.
// This rule tries to merge adjacent limit.
func (r *MergeAdjacentLimit) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	limit := old.GetExpr().ExprNode.(*plannercore.LogicalLimit)
	child := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalLimit)
	childGroups := old.Children[0].GetExpr().Children

	if child.Count <= limit.Offset {
		tableDual := plannercore.LogicalTableDual{RowCount: 0}.Init(child.SCtx(), child.SelectBlockOffset())
		tableDual.SetSchema(old.GetExpr().Schema())
		tableDualExpr := memo.NewGroupExpr(tableDual)
		return []*memo.GroupExpr{tableDualExpr}, true, true, nil
	}

	offset := child.Offset + limit.Offset
	count := uint64(math.Min(float64(child.Count-limit.Offset), float64(limit.Count)))
	newLimit := plannercore.LogicalLimit{
		Offset: offset,
		Count:  count,
	}.Init(limit.SCtx(), limit.SelectBlockOffset())
	newLimitExpr := memo.NewGroupExpr(newLimit)
	newLimitExpr.SetChildren(childGroups...)
	return []*memo.GroupExpr{newLimitExpr}, true, false, nil
}

// TransformLimitToTableDual convert limit to TableDual.
type TransformLimitToTableDual struct {
	baseRule
}

// NewRuleTransformLimitToTableDual creates a new Transformation TransformLimitToTableDual.
// The pattern of this rule is `Limit->X`.
func NewRuleTransformLimitToTableDual() Transformation {
	rule := &TransformLimitToTableDual{}
	rule.pattern = memo.BuildPattern(
		memo.OperandLimit,
		memo.EngineAll,
	)
	return rule
}

// Match implements Transformation interface.
func (r *TransformLimitToTableDual) Match(expr *memo.ExprIter) bool {
	limit := expr.GetExpr().ExprNode.(*plannercore.LogicalLimit)
	return 0 == limit.Count
}

// OnTransform implements Transformation interface.
// This rule tries to convert limit to tableDual.
func (r *TransformLimitToTableDual) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	limit := old.GetExpr().ExprNode.(*plannercore.LogicalLimit)
	tableDual := plannercore.LogicalTableDual{RowCount: 0}.Init(limit.SCtx(), limit.SelectBlockOffset())
	tableDual.SetSchema(old.GetExpr().Schema())
	tableDualExpr := memo.NewGroupExpr(tableDual)
	return []*memo.GroupExpr{tableDualExpr}, true, true, nil
}

// PushLimitDownOuterJoin pushes Limit through Join.
type PushLimitDownOuterJoin struct {
	baseRule
}

// NewRulePushLimitDownOuterJoin creates a new Transformation PushLimitDownOuterJoin.
// The pattern of this rule is `Limit -> Join`.
func NewRulePushLimitDownOuterJoin() Transformation {
	rule := &PushLimitDownOuterJoin{}
	rule.pattern = memo.BuildPattern(
		memo.OperandLimit,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandJoin, memo.EngineTiDBOnly),
	)
	return rule
}

// Match implements Transformation interface.
func (r *PushLimitDownOuterJoin) Match(expr *memo.ExprIter) bool {
	if expr.GetExpr().HasAppliedRule(r) {
		return false
	}
	join := expr.Children[0].GetExpr().ExprNode.(*plannercore.LogicalJoin)
	return join.JoinType.IsOuterJoin()
}

// OnTransform implements Transformation interface.
// This rule tries to pushes the Limit through outer Join.
func (r *PushLimitDownOuterJoin) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	limit := old.GetExpr().ExprNode.(*plannercore.LogicalLimit)
	join := old.Children[0].GetExpr().ExprNode.(*plannercore.LogicalJoin)
	joinSchema := old.Children[0].Group.Prop.Schema
	leftGroup := old.Children[0].GetExpr().Children[0]
	rightGroup := old.Children[0].GetExpr().Children[1]

	switch join.JoinType {
	case plannercore.LeftOuterJoin, plannercore.LeftOuterSemiJoin, plannercore.AntiLeftOuterSemiJoin:
		leftGroup = r.pushLimitDownOuterJoinToChild(limit, leftGroup)
	case plannercore.RightOuterJoin:
		rightGroup = r.pushLimitDownOuterJoinToChild(limit, rightGroup)
	default:
		return nil, false, false, nil
	}

	newJoinExpr := memo.NewGroupExpr(join)
	newJoinExpr.SetChildren(leftGroup, rightGroup)
	newLimitExpr := memo.NewGroupExpr(limit)
	newLimitExpr.SetChildren(memo.NewGroupWithSchema(newJoinExpr, joinSchema))
	newLimitExpr.AddAppliedRule(r)
	return []*memo.GroupExpr{newLimitExpr}, true, false, nil
}

func (r *PushLimitDownOuterJoin) pushLimitDownOuterJoinToChild(limit *plannercore.LogicalLimit, outerGroup *memo.Group) *memo.Group {
	newLimit := plannercore.LogicalLimit{
		Count: limit.Count + limit.Offset,
	}.Init(limit.SCtx(), limit.SelectBlockOffset())
	newLimitGroup := memo.NewGroupExpr(newLimit)
	newLimitGroup.SetChildren(outerGroup)
	return memo.NewGroupWithSchema(newLimitGroup, outerGroup.Prop.Schema)
}

// PushLimitDownTiKVSingleGather pushes the limit down to child of TiKVSingleGather.
type PushLimitDownTiKVSingleGather struct {
	baseRule
}

// NewRulePushLimitDownTiKVSingleGather creates a new Transformation PushLimitDownTiKVSingleGather.
// The pattern of this rule is `Limit -> TiKVSingleGather`.
func NewRulePushLimitDownTiKVSingleGather() Transformation {
	rule := &PushLimitDownTiKVSingleGather{}
	rule.pattern = memo.BuildPattern(
		memo.OperandLimit,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandTiKVSingleGather, memo.EngineTiDBOnly),
	)
	return rule
}

// Match implements Transformation interface.
// Use appliedRuleSet in GroupExpr to avoid re-apply rules.
func (r *PushLimitDownTiKVSingleGather) Match(expr *memo.ExprIter) bool {
	return !expr.GetExpr().HasAppliedRule(r)
}

// OnTransform implements Transformation interface.
// It transforms `Limit -> TiKVSingleGather` to `Limit(Final) -> TiKVSingleGather -> Limit(Partial)`.
func (r *PushLimitDownTiKVSingleGather) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	limit := old.GetExpr().ExprNode.(*plannercore.LogicalLimit)
	limitSchema := old.Children[0].Group.Prop.Schema
	gather := old.Children[0].GetExpr().ExprNode.(*plannercore.TiKVSingleGather)
	childGroup := old.Children[0].GetExpr().Children[0]

	particalLimit := plannercore.LogicalLimit{
		Count: limit.Count + limit.Offset,
	}.Init(limit.SCtx(), limit.SelectBlockOffset())
	partialLimitExpr := memo.NewGroupExpr(particalLimit)
	partialLimitExpr.SetChildren(childGroup)
	partialLimitGroup := memo.NewGroupWithSchema(partialLimitExpr, limitSchema).SetEngineType(childGroup.EngineType)

	gatherExpr := memo.NewGroupExpr(gather)
	gatherExpr.SetChildren(partialLimitGroup)
	gatherGroup := memo.NewGroupWithSchema(gatherExpr, limitSchema)

	finalLimitExpr := memo.NewGroupExpr(limit)
	finalLimitExpr.SetChildren(gatherGroup)
	finalLimitExpr.AddAppliedRule(r)
	return []*memo.GroupExpr{finalLimitExpr}, true, false, nil
}

type outerJoinEliminator struct {
}

func (*outerJoinEliminator) prepareForEliminateOuterJoin(joinExpr *memo.GroupExpr) (ok bool, innerChildIdx int, outerGroup *memo.Group, innerGroup *memo.Group, outerUniqueIDs set.Int64Set) {
	join := joinExpr.ExprNode.(*plannercore.LogicalJoin)

	switch join.JoinType {
	case plannercore.LeftOuterJoin:
		innerChildIdx = 1
	case plannercore.RightOuterJoin:
		innerChildIdx = 0
	default:
		ok = false
		return
	}
	outerGroup = joinExpr.Children[1^innerChildIdx]
	innerGroup = joinExpr.Children[innerChildIdx]

	outerUniqueIDs = set.NewInt64Set()
	for _, outerCol := range outerGroup.Prop.Schema.Columns {
		outerUniqueIDs.Insert(outerCol.UniqueID)
	}

	ok = true
	return
}

// check whether one of unique keys sets is contained by inner join keys.
func (*outerJoinEliminator) isInnerJoinKeysContainUniqueKey(innerGroup *memo.Group, joinKeys *expression.Schema) (bool, error) {
	// builds UniqueKey info of innerGroup.
	innerGroup.BuildKeyInfo()
	for _, keyInfo := range innerGroup.Prop.Schema.Keys {
		joinKeysContainKeyInfo := true
		for _, col := range keyInfo {
			if !joinKeys.Contains(col) {
				joinKeysContainKeyInfo = false
				break
			}
		}
		if joinKeysContainKeyInfo {
			return true, nil
		}
	}
	return false, nil
}

// EliminateOuterJoinBelowAggregation eliminate the outer join which below aggregation.
type EliminateOuterJoinBelowAggregation struct {
	baseRule
	outerJoinEliminator
}

// NewRuleEliminateOuterJoinBelowAggregation creates a new Transformation EliminateOuterJoinBelowAggregation.
// The pattern of this rule is `Aggregation->Join->X`.
func NewRuleEliminateOuterJoinBelowAggregation() Transformation {
	rule := &EliminateOuterJoinBelowAggregation{}
	rule.pattern = memo.BuildPattern(
		memo.OperandAggregation,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandJoin, memo.EngineTiDBOnly),
	)
	return rule
}

// Match implements Transformation interface.
func (r *EliminateOuterJoinBelowAggregation) Match(expr *memo.ExprIter) bool {
	joinType := expr.Children[0].GetExpr().ExprNode.(*plannercore.LogicalJoin).JoinType
	return joinType == plannercore.LeftOuterJoin || joinType == plannercore.RightOuterJoin
}

// OnTransform implements Transformation interface.
// This rule tries to eliminate outer join which below aggregation.
func (r *EliminateOuterJoinBelowAggregation) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	agg := old.GetExpr().ExprNode.(*plannercore.LogicalAggregation)
	joinExpr := old.Children[0].GetExpr()
	join := joinExpr.ExprNode.(*plannercore.LogicalJoin)

	ok, innerChildIdx, outerGroup, innerGroup, outerUniqueIDs := r.prepareForEliminateOuterJoin(joinExpr)
	if !ok {
		return nil, false, false, nil
	}

	// only when agg only use the columns from outer table can eliminate outer join.
	if !plannercore.IsColsAllFromOuterTable(agg.GetUsedCols(), outerUniqueIDs) {
		return nil, false, false, nil
	}
	// outer join elimination with duplicate agnostic aggregate functions.
	_, aggCols := plannercore.GetDupAgnosticAggCols(agg, nil)
	if len(aggCols) > 0 {
		newAggExpr := memo.NewGroupExpr(agg)
		newAggExpr.SetChildren(outerGroup)
		return []*memo.GroupExpr{newAggExpr}, true, false, nil
	}
	// outer join elimination without duplicate agnostic aggregate functions.
	innerJoinKeys := join.ExtractJoinKeys(innerChildIdx)
	contain, err := r.isInnerJoinKeysContainUniqueKey(innerGroup, innerJoinKeys)
	if err != nil {
		return nil, false, false, err
	}
	if contain {
		newAggExpr := memo.NewGroupExpr(agg)
		newAggExpr.SetChildren(outerGroup)
		return []*memo.GroupExpr{newAggExpr}, true, false, nil
	}

	return nil, false, false, nil
}

// EliminateOuterJoinBelowProjection eliminate the outer join which below projection.
type EliminateOuterJoinBelowProjection struct {
	baseRule
	outerJoinEliminator
}

// NewRuleEliminateOuterJoinBelowProjection creates a new Transformation EliminateOuterJoinBelowProjection.
// The pattern of this rule is `Projection->Join->X`.
func NewRuleEliminateOuterJoinBelowProjection() Transformation {
	rule := &EliminateOuterJoinBelowProjection{}
	rule.pattern = memo.BuildPattern(
		memo.OperandProjection,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandJoin, memo.EngineTiDBOnly),
	)
	return rule
}

// Match implements Transformation interface.
func (r *EliminateOuterJoinBelowProjection) Match(expr *memo.ExprIter) bool {
	joinType := expr.Children[0].GetExpr().ExprNode.(*plannercore.LogicalJoin).JoinType
	return joinType == plannercore.LeftOuterJoin || joinType == plannercore.RightOuterJoin
}

// OnTransform implements Transformation interface.
// This rule tries to eliminate outer join which below projection.
func (r *EliminateOuterJoinBelowProjection) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	proj := old.GetExpr().ExprNode.(*plannercore.LogicalProjection)
	joinExpr := old.Children[0].GetExpr()
	join := joinExpr.ExprNode.(*plannercore.LogicalJoin)

	ok, innerChildIdx, outerGroup, innerGroup, outerUniqueIDs := r.prepareForEliminateOuterJoin(joinExpr)
	if !ok {
		return nil, false, false, nil
	}

	// only when proj only use the columns from outer table can eliminate outer join.
	if !plannercore.IsColsAllFromOuterTable(proj.GetUsedCols(), outerUniqueIDs) {
		return nil, false, false, nil
	}

	innerJoinKeys := join.ExtractJoinKeys(innerChildIdx)
	contain, err := r.isInnerJoinKeysContainUniqueKey(innerGroup, innerJoinKeys)
	if err != nil {
		return nil, false, false, err
	}
	if contain {
		newProjExpr := memo.NewGroupExpr(proj)
		newProjExpr.SetChildren(outerGroup)
		return []*memo.GroupExpr{newProjExpr}, true, false, nil
	}

	return nil, false, false, nil
}

// TransformAggregateCaseToSelection convert Agg(case when) to Agg->Selection.
type TransformAggregateCaseToSelection struct {
	baseRule
}

// NewRuleTransformAggregateCaseToSelection creates a new Transformation TransformAggregateCaseToSelection.
// The pattern of this rule is `Agg->X`.
func NewRuleTransformAggregateCaseToSelection() Transformation {
	rule := &TransformAggregateCaseToSelection{}
	rule.pattern = memo.BuildPattern(
		memo.OperandAggregation,
		memo.EngineTiDBOnly,
	)
	return rule
}

// Match implements Transformation interface.
func (r *TransformAggregateCaseToSelection) Match(expr *memo.ExprIter) bool {
	agg := expr.GetExpr().ExprNode.(*plannercore.LogicalAggregation)
	return agg.IsCompleteModeAgg() && len(agg.GroupByItems) == 0 && len(agg.AggFuncs) == 1 && len(agg.AggFuncs[0].Args) == 1 && r.isTwoOrThreeArgCase(agg.AggFuncs[0].Args[0])
}

// OnTransform implements Transformation interface.
// This rule tries to convert Agg(case when) to Agg->Selection.
func (r *TransformAggregateCaseToSelection) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	agg := old.GetExpr().ExprNode.(*plannercore.LogicalAggregation)

	ok, newConditions, newAggFuncs := r.transform(agg)
	if !ok {
		return nil, false, false, nil
	}

	newSel := plannercore.LogicalSelection{Conditions: newConditions}.Init(agg.SCtx(), agg.SelectBlockOffset())
	newSelExpr := memo.NewGroupExpr(newSel)
	newSelExpr.SetChildren(old.GetExpr().Children...)
	newSelGroup := memo.NewGroupWithSchema(newSelExpr, old.GetExpr().Children[0].Prop.Schema)

	newAgg := plannercore.LogicalAggregation{
		AggFuncs:     newAggFuncs,
		GroupByItems: agg.GroupByItems,
	}.Init(agg.SCtx(), agg.SelectBlockOffset())
	newAgg.CopyAggHints(agg)
	newAggExpr := memo.NewGroupExpr(newAgg)
	newAggExpr.SetChildren(newSelGroup)
	return []*memo.GroupExpr{newAggExpr}, true, false, nil
}

func (r *TransformAggregateCaseToSelection) transform(agg *plannercore.LogicalAggregation) (ok bool, newConditions []expression.Expression, newAggFuncs []*aggregation.AggFuncDesc) {
	aggFuncDesc := agg.AggFuncs[0]
	aggFuncName := aggFuncDesc.Name
	ctx := agg.SCtx()

	caseFunc := aggFuncDesc.Args[0].(*expression.ScalarFunction)
	conditionFromCase := caseFunc.GetArgs()[0]
	caseArgs := caseFunc.GetArgs()
	caseArgsNum := len(caseArgs)

	// `case when a>0 then null else a end` should be converted to `case when !(a>0) then a else null end`.
	var nullFlip = caseArgsNum == 3 && caseArgs[1].Equal(ctx, expression.NewNull()) && !caseArgs[2].Equal(ctx, expression.NewNull())
	// `case when a>0 then 0 else a end` should be converted to `case when !(a>0) then a else 0 end`.
	var zeroFlip = !nullFlip && caseArgsNum == 3 && caseArgs[1].Equal(ctx, expression.NewZero())

	var outputIdx int
	if nullFlip || zeroFlip {
		outputIdx = 2
		newConditions = []expression.Expression{expression.NewFunctionInternal(ctx, ast.UnaryNot, types.NewFieldType(mysql.TypeTiny), conditionFromCase)}
	} else {
		outputIdx = 1
		newConditions = expression.SplitCNFItems(conditionFromCase)
	}

	if aggFuncDesc.HasDistinct {
		// Just one style supported:
		//   COUNT(DISTINCT CASE WHEN x = 'foo' THEN y END)
		// =>
		//   newAggFuncDesc: COUNT(DISTINCT y), newCondition: x = 'foo'

		if aggFuncName == ast.AggFuncCount && r.isOnlyOneNotNull(ctx, caseArgs, caseArgsNum, outputIdx) {
			newAggFuncDesc := aggFuncDesc.Clone()
			newAggFuncDesc.Args = []expression.Expression{caseArgs[outputIdx]}
			return true, newConditions, []*aggregation.AggFuncDesc{newAggFuncDesc}
		}
		return false, nil, nil
	}

	// Two styles supported:
	//
	// A1: AGG(CASE WHEN x = 'foo' THEN cnt END)
	//   => newAggFuncDesc: AGG(cnt), newCondition: x = 'foo'
	// A2: SUM(CASE WHEN x = 'foo' THEN cnt ELSE 0 END)
	//   => newAggFuncDesc: SUM(cnt), newCondition: x = 'foo'

	switch {
	case r.allowsSelection(aggFuncName) && (caseArgsNum == 2 || caseArgs[3-outputIdx].Equal(ctx, expression.NewNull())), // Case A1
		aggFuncName == ast.AggFuncSum && caseArgsNum == 3 && caseArgs[3-outputIdx].Equal(ctx, expression.NewZero()): // Case A2
		newAggFuncDesc := aggFuncDesc.Clone()
		newAggFuncDesc.Args = []expression.Expression{caseArgs[outputIdx]}
		return true, newConditions, []*aggregation.AggFuncDesc{newAggFuncDesc}
	default:
		return false, nil, nil
	}
}

func (r *TransformAggregateCaseToSelection) allowsSelection(aggFuncName string) bool {
	return aggFuncName != ast.AggFuncFirstRow
}

func (r *TransformAggregateCaseToSelection) isOnlyOneNotNull(ctx sessionctx.Context, args []expression.Expression, argsNum int, outputIdx int) bool {
	return !args[outputIdx].Equal(ctx, expression.NewNull()) && (argsNum == 2 || args[3-outputIdx].Equal(ctx, expression.NewNull()))
}

// TransformAggregateCaseToSelection only support `case when cond then var end` and `case when cond then var1 else var2 end`.
func (r *TransformAggregateCaseToSelection) isTwoOrThreeArgCase(expr expression.Expression) bool {
	scalarFunc, ok := expr.(*expression.ScalarFunction)
	if !ok {
		return false
	}
	return scalarFunc.FuncName.L == ast.Case && (len(scalarFunc.GetArgs()) == 2 || len(scalarFunc.GetArgs()) == 3)
}

// TransformAggToProj convert Agg to Proj.
type TransformAggToProj struct {
	baseRule
}

// NewRuleTransformAggToProj creates a new Transformation TransformAggToProj.
// The pattern of this rule is `Agg`.
func NewRuleTransformAggToProj() Transformation {
	rule := &TransformAggToProj{}
	rule.pattern = memo.BuildPattern(
		memo.OperandAggregation,
		memo.EngineTiDBOnly,
	)
	return rule
}

// Match implements Transformation interface.
func (r *TransformAggToProj) Match(expr *memo.ExprIter) bool {
	agg := expr.GetExpr().ExprNode.(*plannercore.LogicalAggregation)

	if !agg.IsCompleteModeAgg() {
		return false
	}

	for _, af := range agg.AggFuncs {
		// TODO(issue #9968): same as rule_aggregation_elimination.go -> tryToEliminateAggregation.
		// waiting for (issue #14616): `nullable` information.
		if af.Name == ast.AggFuncGroupConcat {
			return false
		}
	}

	childGroup := expr.GetExpr().Children[0]
	childGroup.BuildKeyInfo()
	schemaByGroupby := expression.NewSchema(agg.GetGroupByCols()...)
	for _, key := range childGroup.Prop.Schema.Keys {
		if schemaByGroupby.ColumnsIndices(key) != nil {
			return true
		}
	}

	return false
}

// OnTransform implements Transformation interface.
// This rule tries to convert agg to proj.
func (r *TransformAggToProj) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	agg := old.GetExpr().ExprNode.(*plannercore.LogicalAggregation)
	if ok, proj := plannercore.ConvertAggToProj(agg, old.GetExpr().Schema()); ok {
		newProjExpr := memo.NewGroupExpr(proj)
		newProjExpr.SetChildren(old.GetExpr().Children...)
		return []*memo.GroupExpr{newProjExpr}, true, false, nil
	}

	return nil, false, false, nil
}

// InjectProjectionBelowTopN injects two Projections below and upon TopN if TopN's ByItems
// contain ScalarFunctions.
type InjectProjectionBelowTopN struct {
	baseRule
}

// NewRuleInjectProjectionBelowTopN creates a new Transformation InjectProjectionBelowTopN.
// It will extract the ScalarFunctions of `ByItems` into a Projection and injects it below TopN.
// When a Projection is injected as the child of TopN, we need to add another Projection upon
// TopN to prune the extra Columns.
// The reason why we need this rule is that, TopNExecutor in TiDB does not support ScalarFunction
// as `ByItem`. So we have to use a Projection to calculate the ScalarFunctions in advance.
// The pattern of this rule is: a single TopN
func NewRuleInjectProjectionBelowTopN() Transformation {
	rule := &InjectProjectionBelowTopN{}
	rule.pattern = memo.BuildPattern(
		memo.OperandTopN,
		memo.EngineTiDBOnly,
	)
	return rule
}

// Match implements Transformation interface.
func (r *InjectProjectionBelowTopN) Match(expr *memo.ExprIter) bool {
	topN := expr.GetExpr().ExprNode.(*plannercore.LogicalTopN)
	for _, item := range topN.ByItems {
		if _, ok := item.Expr.(*expression.ScalarFunction); ok {
			return true
		}
	}
	return false
}

// OnTransform implements Transformation interface.
// It will convert `TopN -> X` to `Projection -> TopN -> Projection -> X`.
func (r *InjectProjectionBelowTopN) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	topN := old.GetExpr().ExprNode.(*plannercore.LogicalTopN)
	oldTopNSchema := old.GetExpr().Schema()

	// Construct top Projection.
	topProjExprs := make([]expression.Expression, oldTopNSchema.Len())
	for i := range oldTopNSchema.Columns {
		topProjExprs[i] = oldTopNSchema.Columns[i]
	}
	topProj := plannercore.LogicalProjection{
		Exprs: topProjExprs,
	}.Init(topN.SCtx(), topN.SelectBlockOffset())
	topProj.SetSchema(oldTopNSchema)

	// Construct bottom Projection.
	bottomProjExprs := make([]expression.Expression, 0, oldTopNSchema.Len()+len(topN.ByItems))
	bottomProjSchema := make([]*expression.Column, 0, oldTopNSchema.Len()+len(topN.ByItems))
	for _, col := range oldTopNSchema.Columns {
		bottomProjExprs = append(bottomProjExprs, col)
		bottomProjSchema = append(bottomProjSchema, col)
	}
	newByItems := make([]*util.ByItems, 0, len(topN.ByItems))
	for _, item := range topN.ByItems {
		itemExpr := item.Expr
		if _, isScalarFunc := itemExpr.(*expression.ScalarFunction); !isScalarFunc {
			newByItems = append(newByItems, item)
			continue
		}
		bottomProjExprs = append(bottomProjExprs, itemExpr)
		newCol := &expression.Column{
			UniqueID: topN.SCtx().GetSessionVars().AllocPlanColumnID(),
			RetType:  itemExpr.GetType(),
		}
		bottomProjSchema = append(bottomProjSchema, newCol)
		newByItems = append(newByItems, &util.ByItems{Expr: newCol, Desc: item.Desc})
	}
	bottomProj := plannercore.LogicalProjection{
		Exprs: bottomProjExprs,
	}.Init(topN.SCtx(), topN.SelectBlockOffset())
	newSchema := expression.NewSchema(bottomProjSchema...)
	bottomProj.SetSchema(newSchema)

	newTopN := plannercore.LogicalTopN{
		ByItems: newByItems,
		Offset:  topN.Offset,
		Count:   topN.Count,
	}.Init(topN.SCtx(), topN.SelectBlockOffset())

	// Construct GroupExpr, Group (TopProj -> TopN -> BottomProj -> Child)
	bottomProjGroupExpr := memo.NewGroupExpr(bottomProj)
	bottomProjGroupExpr.SetChildren(old.GetExpr().Children[0])
	bottomProjGroup := memo.NewGroupWithSchema(bottomProjGroupExpr, newSchema)

	topNGroupExpr := memo.NewGroupExpr(newTopN)
	topNGroupExpr.SetChildren(bottomProjGroup)
	topNGroup := memo.NewGroupWithSchema(topNGroupExpr, newSchema)

	topProjGroupExpr := memo.NewGroupExpr(topProj)
	topProjGroupExpr.SetChildren(topNGroup)
	return []*memo.GroupExpr{topProjGroupExpr}, true, false, nil
}

// TransformApplyToJoin transforms a LogicalApply to LogicalJoin if it's
// inner children has no correlated columns from it's outer schema.
type TransformApplyToJoin struct {
	baseRule
}

// NewRuleTransformApplyToJoin creates a new Transformation TransformApplyToJoin.
// The pattern of this rule is: `Apply -> (X, Y)`.
func NewRuleTransformApplyToJoin() Transformation {
	rule := &TransformApplyToJoin{}
	rule.pattern = memo.NewPattern(memo.OperandApply, memo.EngineTiDBOnly)
	return rule
}

// OnTransform implements Transformation interface.
func (r *TransformApplyToJoin) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	apply := old.GetExpr().ExprNode.(*plannercore.LogicalApply)
	groupExpr := old.GetExpr()
	// It's safe to use the old apply instead of creating a new LogicalApply here,
	// Because apply.CorCols will only be used and updated by this rule during Transformation.
	apply.CorCols = r.extractCorColumnsBySchema(groupExpr.Children[1], groupExpr.Children[0].Prop.Schema)
	if len(apply.CorCols) != 0 {
		return nil, false, false, nil
	}

	join := apply.LogicalJoin.Shallow()
	joinGroupExpr := memo.NewGroupExpr(join)
	joinGroupExpr.SetChildren(groupExpr.Children...)
	return []*memo.GroupExpr{joinGroupExpr}, true, false, nil
}

func (r *TransformApplyToJoin) extractCorColumnsBySchema(innerGroup *memo.Group, outerSchema *expression.Schema) []*expression.CorrelatedColumn {
	corCols := r.extractCorColumnsFromGroup(innerGroup)
	return plannercore.ExtractCorColumnsBySchema(corCols, outerSchema)
}

func (r *TransformApplyToJoin) extractCorColumnsFromGroup(g *memo.Group) []*expression.CorrelatedColumn {
	corCols := make([]*expression.CorrelatedColumn, 0)
	for elem := g.Equivalents.Front(); elem != nil; elem = elem.Next() {
		expr := elem.Value.(*memo.GroupExpr)
		corCols = append(corCols, expr.ExprNode.ExtractCorrelatedCols()...)
		for _, child := range expr.Children {
			corCols = append(corCols, r.extractCorColumnsFromGroup(child)...)
		}
	}
	// We may have duplicate CorrelatedColumns here, but it won't influence
	// the logic of the transformation. Apply.CorCols will be deduplicated in
	// `ResolveIndices`.
	return corCols
}

// PullSelectionUpApply pulls up the inner-side Selection into Apply as
// its join condition.
type PullSelectionUpApply struct {
	baseRule
}

// NewRulePullSelectionUpApply creates a new Transformation PullSelectionUpApply.
// The pattern of this rule is: `Apply -> (Any<outer>, Selection<inner>)`.
func NewRulePullSelectionUpApply() Transformation {
	rule := &PullSelectionUpApply{}
	rule.pattern = memo.BuildPattern(
		memo.OperandApply,
		memo.EngineTiDBOnly,
		memo.NewPattern(memo.OperandAny, memo.EngineTiDBOnly),       // outer child
		memo.NewPattern(memo.OperandSelection, memo.EngineTiDBOnly), // inner child
	)
	return rule
}

// OnTransform implements Transformation interface.
// This rule tries to pull up the inner side Selection, and add these conditions
// to Join condition inside the Apply.
func (r *PullSelectionUpApply) OnTransform(old *memo.ExprIter) (newExprs []*memo.GroupExpr, eraseOld bool, eraseAll bool, err error) {
	apply := old.GetExpr().ExprNode.(*plannercore.LogicalApply)
	outerChildGroup := old.Children[0].Group
	innerChildGroup := old.Children[1].Group
	sel := old.Children[1].GetExpr().ExprNode.(*plannercore.LogicalSelection)
	newConds := make([]expression.Expression, 0, len(sel.Conditions))
	for _, cond := range sel.Conditions {
		newConds = append(newConds, cond.Clone().Decorrelate(outerChildGroup.Prop.Schema))
	}
	newApply := plannercore.LogicalApply{
		LogicalJoin: *(apply.LogicalJoin.Shallow()),
		CorCols:     apply.CorCols,
	}.Init(apply.SCtx(), apply.SelectBlockOffset())
	// Update Join conditions.
	eq, left, right, other := newApply.LogicalJoin.ExtractOnCondition(newConds, outerChildGroup.Prop.Schema, innerChildGroup.Prop.Schema, false, false)
	newApply.LogicalJoin.AppendJoinConds(eq, left, right, other)

	newApplyGroupExpr := memo.NewGroupExpr(newApply)
	newApplyGroupExpr.SetChildren(outerChildGroup, old.Children[1].GetExpr().Children[0])
	return []*memo.GroupExpr{newApplyGroupExpr}, false, false, nil
>>>>>>> 7ebcc20... executor: support GROUP_CONCAT(ORDER BY) (#16591)
}
