// Copyright 2023 The gVisor Authors.
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

package shm

import (
	"github.com/talismancer/gvisor-ligolo/pkg/context"
)

type contextID int

const (
	// CtxDeviceID is a Context.Value key for kernel.Kernel.sysVShmDevID, which
	// this package cannot refer to due to dependency cycles.
	CtxDeviceID contextID = iota
)

func deviceIDFromContext(ctx context.Context) (uint32, bool) {
	v := ctx.Value(CtxDeviceID)
	if v == nil {
		return 0, false
	}
	return v.(uint32), true
}
