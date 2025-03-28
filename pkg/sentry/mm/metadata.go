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

package mm

import (
	"github.com/talismancer/gvisor-ligolo/pkg/context"
	"github.com/talismancer/gvisor-ligolo/pkg/hostarch"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/arch"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/vfs"
)

// Dumpability describes if and how core dumps should be created.
type Dumpability int

const (
	// NotDumpable indicates that core dumps should never be created.
	NotDumpable Dumpability = iota

	// UserDumpable indicates that core dumps should be created, owned by
	// the current user.
	UserDumpable

	// RootDumpable indicates that core dumps should be created, owned by
	// root.
	RootDumpable
)

// Dumpability returns the dumpability.
func (mm *MemoryManager) Dumpability() Dumpability {
	return Dumpability(mm.dumpability.Load())
}

// SetDumpability sets the dumpability.
func (mm *MemoryManager) SetDumpability(d Dumpability) {
	mm.dumpability.Store(int32(d))
}

// ArgvStart returns the start of the application argument vector.
//
// There is no guarantee that this value is sensible w.r.t. ArgvEnd.
func (mm *MemoryManager) ArgvStart() hostarch.Addr {
	mm.metadataMu.Lock()
	defer mm.metadataMu.Unlock()
	return mm.argv.Start
}

// SetArgvStart sets the start of the application argument vector.
func (mm *MemoryManager) SetArgvStart(a hostarch.Addr) {
	mm.metadataMu.Lock()
	defer mm.metadataMu.Unlock()
	mm.argv.Start = a
}

// ArgvEnd returns the end of the application argument vector.
//
// There is no guarantee that this value is sensible w.r.t. ArgvStart.
func (mm *MemoryManager) ArgvEnd() hostarch.Addr {
	mm.metadataMu.Lock()
	defer mm.metadataMu.Unlock()
	return mm.argv.End
}

// SetArgvEnd sets the end of the application argument vector.
func (mm *MemoryManager) SetArgvEnd(a hostarch.Addr) {
	mm.metadataMu.Lock()
	defer mm.metadataMu.Unlock()
	mm.argv.End = a
}

// EnvvStart returns the start of the application environment vector.
//
// There is no guarantee that this value is sensible w.r.t. EnvvEnd.
func (mm *MemoryManager) EnvvStart() hostarch.Addr {
	mm.metadataMu.Lock()
	defer mm.metadataMu.Unlock()
	return mm.envv.Start
}

// SetEnvvStart sets the start of the application environment vector.
func (mm *MemoryManager) SetEnvvStart(a hostarch.Addr) {
	mm.metadataMu.Lock()
	defer mm.metadataMu.Unlock()
	mm.envv.Start = a
}

// EnvvEnd returns the end of the application environment vector.
//
// There is no guarantee that this value is sensible w.r.t. EnvvStart.
func (mm *MemoryManager) EnvvEnd() hostarch.Addr {
	mm.metadataMu.Lock()
	defer mm.metadataMu.Unlock()
	return mm.envv.End
}

// SetEnvvEnd sets the end of the application environment vector.
func (mm *MemoryManager) SetEnvvEnd(a hostarch.Addr) {
	mm.metadataMu.Lock()
	defer mm.metadataMu.Unlock()
	mm.envv.End = a
}

// Auxv returns the current map of auxiliary vectors.
func (mm *MemoryManager) Auxv() arch.Auxv {
	mm.metadataMu.Lock()
	defer mm.metadataMu.Unlock()
	return append(arch.Auxv(nil), mm.auxv...)
}

// SetAuxv sets the entire map of auxiliary vectors.
func (mm *MemoryManager) SetAuxv(auxv arch.Auxv) {
	mm.metadataMu.Lock()
	defer mm.metadataMu.Unlock()
	mm.auxv = append(arch.Auxv(nil), auxv...)
}

// Executable returns the executable, if available.
//
// An additional reference will be taken in the case of a non-nil executable,
// which must be released by the caller.
func (mm *MemoryManager) Executable() *vfs.FileDescription {
	mm.metadataMu.Lock()
	defer mm.metadataMu.Unlock()

	if mm.executable == nil {
		return nil
	}

	mm.executable.IncRef()
	return mm.executable
}

// SetExecutable sets the executable.
//
// This takes a reference on d.
func (mm *MemoryManager) SetExecutable(ctx context.Context, fd *vfs.FileDescription) {
	mm.metadataMu.Lock()

	// Grab a new reference.
	fd.IncRef()

	// Set the executable.
	orig := mm.executable
	mm.executable = fd

	mm.metadataMu.Unlock()

	// Release the old reference.
	//
	// Do this without holding the lock, since it may wind up doing some
	// I/O to sync the dirent, etc.
	if orig != nil {
		orig.DecRef(ctx)
	}
}

// VDSOSigReturn returns the address of vdso_sigreturn.
func (mm *MemoryManager) VDSOSigReturn() uint64 {
	mm.metadataMu.Lock()
	defer mm.metadataMu.Unlock()
	return mm.vdsoSigReturnAddr
}

// SetVDSOSigReturn sets the address of vdso_sigreturn.
func (mm *MemoryManager) SetVDSOSigReturn(addr uint64) {
	mm.metadataMu.Lock()
	defer mm.metadataMu.Unlock()
	mm.vdsoSigReturnAddr = addr
}
