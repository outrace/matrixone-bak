// Copyright 2021 Matrix Origin
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

package hashbuild

import (
	"bytes"
	"context"
	"testing"

	"github.com/matrixorigin/matrixone/pkg/common/hashmap"
	"github.com/matrixorigin/matrixone/pkg/common/mpool"
	"github.com/matrixorigin/matrixone/pkg/container/batch"
	"github.com/matrixorigin/matrixone/pkg/container/index"
	"github.com/matrixorigin/matrixone/pkg/container/types"
	"github.com/matrixorigin/matrixone/pkg/container/vector"
	"github.com/matrixorigin/matrixone/pkg/pb/plan"
	"github.com/matrixorigin/matrixone/pkg/testutil"
	"github.com/matrixorigin/matrixone/pkg/vm/process"
	"github.com/stretchr/testify/require"
)

const (
	Rows          = 10     // default rows
	BenchmarkRows = 100000 // default rows for benchmark
)

// add unit tests for cases
type buildTestCase struct {
	arg    *Argument
	flgs   []bool // flgs[i] == true: nullable
	types  []types.Type
	proc   *process.Process
	cancel context.CancelFunc
}

var (
	tcs []buildTestCase
)

func init() {
	tcs = []buildTestCase{
		newTestCase([]bool{false}, []types.Type{{Oid: types.T_int8}},
			[]*plan.Expr{
				newExpr(0, types.Type{Oid: types.T_int8}),
			}),
		newTestCase([]bool{true}, []types.Type{{Oid: types.T_int8}},
			[]*plan.Expr{
				newExpr(0, types.Type{Oid: types.T_int8}),
			}),
	}
}

func TestString(t *testing.T) {
	buf := new(bytes.Buffer)
	for _, tc := range tcs {
		String(tc.arg, buf)
	}
}

func TestBuild(t *testing.T) {
	for _, tc := range tcs[:1] {
		err := Prepare(tc.proc, tc.arg)
		require.NoError(t, err)
		tc.proc.Reg.MergeReceivers[0].Ch <- newBatch(t, tc.flgs, tc.types, tc.proc, Rows)
		tc.proc.Reg.MergeReceivers[0].Ch <- &batch.Batch{}
		tc.proc.Reg.MergeReceivers[0].Ch <- nil
		for {
			ok, err := Call(0, tc.proc, tc.arg, false, false)
			require.NoError(t, err)
			require.Equal(t, true, ok)
			mp := tc.proc.Reg.InputBatch.Ht.(*hashmap.JoinMap)
			mp.Free()
			tc.proc.Reg.InputBatch.Clean(tc.proc.Mp())
			break
		}
		tc.arg.Free(tc.proc, false)
		require.Equal(t, int64(0), tc.proc.Mp().CurrNB())
	}
}

func TestLowCardinalityBuild(t *testing.T) {
	tc := newTestCase([]bool{false}, []types.Type{types.T_varchar.ToType()},
		[]*plan.Expr{
			newExpr(0, types.T_varchar.ToType()),
		},
	)
	err := Prepare(tc.proc, tc.arg)
	require.NoError(t, err)

	values := []string{"a", "b", "a", "c", "b", "c", "a", "a"}
	v := testutil.NewVector(len(values), types.T_varchar.ToType(), tc.proc.Mp(), false, values)
	constructIndex(t, v, tc.proc.Mp())

	tc.proc.Reg.MergeReceivers[0].Ch <- testutil.NewBatchWithVectors([]*vector.Vector{v}, nil)
	tc.proc.Reg.MergeReceivers[0].Ch <- nil

	ok, err := Call(0, tc.proc, tc.arg, false, false)
	require.NoError(t, err)
	require.Equal(t, true, ok)
	mp := tc.proc.Reg.InputBatch.Ht.(*hashmap.JoinMap)
	require.NotNil(t, mp.Index())

	sels := mp.Sels()
	require.Equal(t, []int64{0, 2, 6, 7}, sels[0])
	require.Equal(t, []int64{1, 4}, sels[1])
	require.Equal(t, []int64{3, 5}, sels[2])

	mp.Free()
	tc.proc.Reg.InputBatch.Clean(tc.proc.Mp())
}

func BenchmarkBuild(b *testing.B) {
	for i := 0; i < b.N; i++ {
		tcs = []buildTestCase{
			newTestCase([]bool{false}, []types.Type{{Oid: types.T_int8}},
				[]*plan.Expr{
					newExpr(0, types.Type{Oid: types.T_int8}),
				}),
		}
		t := new(testing.T)
		for _, tc := range tcs {
			err := Prepare(tc.proc, tc.arg)
			require.NoError(t, err)
			tc.proc.Reg.MergeReceivers[0].Ch <- newBatch(t, tc.flgs, tc.types, tc.proc, Rows)
			tc.proc.Reg.MergeReceivers[0].Ch <- &batch.Batch{}
			tc.proc.Reg.MergeReceivers[0].Ch <- nil
			for {
				ok, err := Call(0, tc.proc, tc.arg, false, false)
				require.NoError(t, err)
				require.Equal(t, true, ok)
				mp := tc.proc.Reg.InputBatch.Ht.(*hashmap.JoinMap)
				mp.Free()
				tc.proc.Reg.InputBatch.Clean(tc.proc.Mp())
				break
			}
		}
	}
}

func newExpr(pos int32, typ types.Type) *plan.Expr {
	return &plan.Expr{
		Typ: &plan.Type{
			Size:  typ.Size,
			Scale: typ.Scale,
			Width: typ.Width,
			Id:    int32(typ.Oid),
		},
		Expr: &plan.Expr_Col{
			Col: &plan.ColRef{
				ColPos: pos,
			},
		},
	}
}

func newTestCase(flgs []bool, ts []types.Type, cs []*plan.Expr) buildTestCase {
	proc := testutil.NewProcessWithMPool(mpool.MustNewZero())
	proc.Reg.MergeReceivers = make([]*process.WaitRegister, 1)
	ctx, cancel := context.WithCancel(context.Background())
	proc.Reg.MergeReceivers[0] = &process.WaitRegister{
		Ctx: ctx,
		Ch:  make(chan *batch.Batch, 10),
	}
	return buildTestCase{
		types:  ts,
		flgs:   flgs,
		proc:   proc,
		cancel: cancel,
		arg: &Argument{
			Typs:        ts,
			Conditions:  cs,
			NeedHashMap: true,
		},
	}
}

// create a new block based on the type information, flgs[i] == ture: has null
func newBatch(t *testing.T, flgs []bool, ts []types.Type, proc *process.Process, rows int64) *batch.Batch {
	return testutil.NewBatch(ts, false, int(rows), proc.Mp())
}

func constructIndex(t *testing.T, v *vector.Vector, m *mpool.MPool) {
	idx, err := index.New(v.Typ, m)
	require.NoError(t, err)

	err = idx.InsertBatch(v)
	require.NoError(t, err)

	v.SetIndex(idx)
}
