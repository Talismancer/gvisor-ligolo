// Copyright 2020 The gVisor Authors.
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

//go:build go1.1
// +build go1.1

// Binary runsc implements the OCI runtime interface.
package main

import (
	"github.com/talismancer/gvisor-ligolo/runsc/cli"
	"github.com/talismancer/gvisor-ligolo/runsc/version"
)

// version.Version is set dynamically, but needs to be
// linked in the binary, so reference it here.
var _ = version.Version()

func main() {
	cli.Main()
}
