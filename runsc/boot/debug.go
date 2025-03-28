// Copyright 2018 The gVisor Authors.
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

package boot

import (
	"github.com/talismancer/gvisor-ligolo/pkg/log"
)

type debug struct {
}

// Stacks collects all sandbox stacks and copies them to 'stacks'.
func (*debug) Stacks(_ *struct{}, stacks *string) error {
	buf := log.Stacks(true)
	*stacks = string(buf)
	return nil
}
