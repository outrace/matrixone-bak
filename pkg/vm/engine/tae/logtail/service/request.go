// Copyright 2021 Matrix Origin
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package service

import (
	"sync"

	"github.com/matrixorigin/matrixone/pkg/common/morpc"
	"github.com/matrixorigin/matrixone/pkg/pb/logtail"
)

// LogtailRequest wraps logtail.LogtailRequest.
type LogtailRequest struct {
	logtail.LogtailRequest
}

var _ morpc.Message = (*LogtailRequest)(nil)

func (r *LogtailRequest) SetID(id uint64) {
	r.RequestId = id
}

func (r *LogtailRequest) GetID() uint64 {
	return r.RequestId
}

func (r *LogtailRequest) DebugString() string {
	return r.LogtailRequest.String()
}

func (r *LogtailRequest) Size() int {
	return r.ProtoSize()
}

// RequestPool acquires or releases LogtailRequest.
type RequestPool interface {
	// Acquire fetches item from pool.
	Acquire() morpc.Message

	// Release puts item back to pool.
	Release(*LogtailRequest)
}

type requestPool struct {
	pool *sync.Pool
}

func NewRequestPool() RequestPool {
	return &requestPool{
		pool: &sync.Pool{
			New: func() any {
				return &LogtailRequest{}
			},
		},
	}
}

func (p *requestPool) Acquire() morpc.Message {
	return p.pool.Get().(*LogtailRequest)
}

func (p *requestPool) Release(req *LogtailRequest) {
	req.Reset()
	p.pool.Put(req)
}
