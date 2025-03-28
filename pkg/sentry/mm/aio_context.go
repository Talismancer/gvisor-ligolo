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
	"github.com/talismancer/gvisor-ligolo/pkg/abi/linux"
	"github.com/talismancer/gvisor-ligolo/pkg/context"
	"github.com/talismancer/gvisor-ligolo/pkg/errors/linuxerr"
	"github.com/talismancer/gvisor-ligolo/pkg/hostarch"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/memmap"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/pgalloc"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/usage"
	"github.com/talismancer/gvisor-ligolo/pkg/usermem"
)

// aioManager creates and manages asynchronous I/O contexts.
//
// +stateify savable
type aioManager struct {
	// mu protects below.
	mu aioManagerMutex `state:"nosave"`

	// aioContexts is the set of asynchronous I/O contexts.
	contexts map[uint64]*AIOContext
}

func (mm *MemoryManager) destroyAIOManager(ctx context.Context) {
	mm.aioManager.mu.Lock()
	defer mm.aioManager.mu.Unlock()

	for id := range mm.aioManager.contexts {
		mm.destroyAIOContextLocked(ctx, id)
	}
}

// newAIOContext creates a new context for asynchronous I/O.
//
// Returns false if 'id' is currently in use.
func (a *aioManager) newAIOContext(events uint32, id uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.contexts[id]; ok {
		return false
	}

	a.contexts[id] = &AIOContext{
		requestReady:   make(chan struct{}, 1),
		maxOutstanding: events,
	}
	return true
}

// destroyAIOContext destroys an asynchronous I/O context. It doesn't wait for
// for pending requests to complete. Returns the destroyed AIOContext so it can
// be drained.
//
// Nil is returned if the context does not exist.
//
// Precondition: mm.aioManager.mu is locked.
func (mm *MemoryManager) destroyAIOContextLocked(ctx context.Context, id uint64) *AIOContext {
	aioCtx, ok := mm.aioManager.contexts[id]
	if !ok {
		return nil
	}

	delete(mm.aioManager.contexts, id)
	aioCtx.destroy()
	return aioCtx
}

// lookupAIOContext looks up the given context.
//
// Returns false if context does not exist.
func (a *aioManager) lookupAIOContext(id uint64) (*AIOContext, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ctx, ok := a.contexts[id]
	return ctx, ok
}

// ioResult is a completed I/O operation.
//
// +stateify savable
type ioResult struct {
	data any
	ioEntry
}

// AIOContext is a single asynchronous I/O context.
//
// +stateify savable
type AIOContext struct {
	// requestReady is the notification channel used for all requests.
	requestReady chan struct{} `state:"nosave"`

	// mu protects below.
	mu aioContextMutex `state:"nosave"`

	// results is the set of completed requests.
	results ioList

	// maxOutstanding is the maximum number of outstanding entries; this value
	// is immutable.
	maxOutstanding uint32

	// outstanding is the number of requests outstanding; this will effectively
	// be the number of entries in the result list or that are expected to be
	// added to the result list.
	outstanding uint32

	// dead is set when the context is destroyed.
	dead bool `state:"zerovalue"`
}

// destroy marks the context dead.
func (ctx *AIOContext) destroy() {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.dead = true
	ctx.checkForDone()
}

// Preconditions: ctx.mu must be held by caller.
func (ctx *AIOContext) checkForDone() {
	if ctx.dead && ctx.outstanding == 0 {
		close(ctx.requestReady)
		ctx.requestReady = nil
	}
}

// Prepare reserves space for a new request, returning nil if available.
// Returns EAGAIN if the context is busy and EINVAL if the context is dead.
func (ctx *AIOContext) Prepare() error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.dead {
		// Context died after the caller looked it up.
		return linuxerr.EINVAL
	}
	if ctx.outstanding >= ctx.maxOutstanding {
		// Context is busy.
		return linuxerr.EAGAIN
	}
	ctx.outstanding++
	return nil
}

// PopRequest pops a completed request if available, this function does not do
// any blocking. Returns false if no request is available.
func (ctx *AIOContext) PopRequest() (any, bool) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	// Is there anything ready?
	if e := ctx.results.Front(); e != nil {
		if ctx.outstanding == 0 {
			panic("AIOContext outstanding is going negative")
		}
		ctx.outstanding--
		ctx.results.Remove(e)
		ctx.checkForDone()
		return e.data, true
	}
	return nil, false
}

// FinishRequest finishes a pending request. It queues up the data
// and notifies listeners.
func (ctx *AIOContext) FinishRequest(data any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	// Push to the list and notify opportunistically. The channel notify
	// here is guaranteed to be safe because outstanding must be non-zero.
	// The requestReady channel is only closed when outstanding reaches zero.
	ctx.results.PushBack(&ioResult{data: data})

	select {
	case ctx.requestReady <- struct{}{}:
	default:
	}
}

// WaitChannel returns a channel that is notified when an AIO request is
// completed. Returns nil if the context is destroyed and there are no more
// outstanding requests.
func (ctx *AIOContext) WaitChannel() chan struct{} {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.requestReady
}

// Dead returns true if the context has been destroyed.
func (ctx *AIOContext) Dead() bool {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.dead
}

// CancelPendingRequest forgets about a request that hasn't yet completed.
func (ctx *AIOContext) CancelPendingRequest() {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	if ctx.outstanding == 0 {
		panic("AIOContext outstanding is going negative")
	}
	ctx.outstanding--
	ctx.checkForDone()
}

// Drain drops all completed requests. Pending requests remain untouched.
func (ctx *AIOContext) Drain() {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	if ctx.outstanding == 0 {
		return
	}
	size := uint32(ctx.results.Len())
	if ctx.outstanding < size {
		panic("AIOContext outstanding is going negative")
	}
	ctx.outstanding -= size
	ctx.results.Reset()
	ctx.checkForDone()
}

// aioMappable implements memmap.MappingIdentity and memmap.Mappable for AIO
// ring buffers.
//
// +stateify savable
type aioMappable struct {
	aioMappableRefs

	mfp pgalloc.MemoryFileProvider
	fr  memmap.FileRange
}

var aioRingBufferSize = uint64(hostarch.Addr(linux.AIORingSize).MustRoundUp())

func newAIOMappable(ctx context.Context, mfp pgalloc.MemoryFileProvider) (*aioMappable, error) {
	fr, err := mfp.MemoryFile().Allocate(aioRingBufferSize, pgalloc.AllocOpts{Kind: usage.Anonymous, MemCgID: pgalloc.MemoryCgroupIDFromContext(ctx)})
	if err != nil {
		return nil, err
	}
	m := aioMappable{mfp: mfp, fr: fr}
	m.InitRefs()
	return &m, nil
}

// DecRef implements refs.RefCounter.DecRef.
func (m *aioMappable) DecRef(ctx context.Context) {
	m.aioMappableRefs.DecRef(func() {
		m.mfp.MemoryFile().DecRef(m.fr)
	})
}

// MappedName implements memmap.MappingIdentity.MappedName.
func (m *aioMappable) MappedName(ctx context.Context) string {
	return "[aio]"
}

// DeviceID implements memmap.MappingIdentity.DeviceID.
func (m *aioMappable) DeviceID() uint64 {
	return 0
}

// InodeID implements memmap.MappingIdentity.InodeID.
func (m *aioMappable) InodeID() uint64 {
	return 0
}

// Msync implements memmap.MappingIdentity.Msync.
func (m *aioMappable) Msync(ctx context.Context, mr memmap.MappableRange) error {
	// Linux: aio_ring_fops.fsync == NULL
	return linuxerr.EINVAL
}

// AddMapping implements memmap.Mappable.AddMapping.
func (m *aioMappable) AddMapping(_ context.Context, _ memmap.MappingSpace, ar hostarch.AddrRange, offset uint64, _ bool) error {
	// Don't allow mappings to be expanded (in Linux, fs/aio.c:aio_ring_mmap()
	// sets VM_DONTEXPAND).
	if offset != 0 || uint64(ar.Length()) != aioRingBufferSize {
		return linuxerr.EFAULT
	}
	return nil
}

// RemoveMapping implements memmap.Mappable.RemoveMapping.
func (m *aioMappable) RemoveMapping(context.Context, memmap.MappingSpace, hostarch.AddrRange, uint64, bool) {
}

// CopyMapping implements memmap.Mappable.CopyMapping.
func (m *aioMappable) CopyMapping(ctx context.Context, ms memmap.MappingSpace, srcAR, dstAR hostarch.AddrRange, offset uint64, _ bool) error {
	// Don't allow mappings to be expanded (in Linux, fs/aio.c:aio_ring_mmap()
	// sets VM_DONTEXPAND).
	if offset != 0 || uint64(dstAR.Length()) != aioRingBufferSize {
		return linuxerr.EFAULT
	}
	// Require that the mapping correspond to a live AIOContext. Compare
	// Linux's fs/aio.c:aio_ring_mremap().
	mm, ok := ms.(*MemoryManager)
	if !ok {
		return linuxerr.EINVAL
	}
	am := &mm.aioManager
	am.mu.Lock()
	defer am.mu.Unlock()
	oldID := uint64(srcAR.Start)
	aioCtx, ok := am.contexts[oldID]
	if !ok {
		return linuxerr.EINVAL
	}
	aioCtx.mu.Lock()
	defer aioCtx.mu.Unlock()
	if aioCtx.dead {
		return linuxerr.EINVAL
	}
	// Use the new ID for the AIOContext.
	am.contexts[uint64(dstAR.Start)] = aioCtx
	delete(am.contexts, oldID)
	return nil
}

// Translate implements memmap.Mappable.Translate.
func (m *aioMappable) Translate(ctx context.Context, required, optional memmap.MappableRange, at hostarch.AccessType) ([]memmap.Translation, error) {
	var err error
	if required.End > m.fr.Length() {
		err = &memmap.BusError{linuxerr.EFAULT}
	}
	if source := optional.Intersect(memmap.MappableRange{0, m.fr.Length()}); source.Length() != 0 {
		return []memmap.Translation{
			{
				Source: source,
				File:   m.mfp.MemoryFile(),
				Offset: m.fr.Start + source.Start,
				Perms:  hostarch.AnyAccess,
			},
		}, err
	}
	return nil, err
}

// InvalidateUnsavable implements memmap.Mappable.InvalidateUnsavable.
func (m *aioMappable) InvalidateUnsavable(ctx context.Context) error {
	return nil
}

// NewAIOContext creates a new context for asynchronous I/O.
//
// NewAIOContext is analogous to Linux's fs/aio.c:ioctx_alloc().
func (mm *MemoryManager) NewAIOContext(ctx context.Context, events uint32) (uint64, error) {
	// libaio get_ioevents() expects context "handle" to be a valid address.
	// libaio peeks inside looking for a magic number. This function allocates
	// a page per context and keeps it set to zeroes to ensure it will not
	// match AIO_RING_MAGIC and make libaio happy.
	m, err := newAIOMappable(ctx, mm.mfp)
	if err != nil {
		return 0, err
	}
	defer m.DecRef(ctx)
	addr, err := mm.MMap(ctx, memmap.MMapOpts{
		Length:          aioRingBufferSize,
		MappingIdentity: m,
		Mappable:        m,
		// Linux uses "do_mmap_pgoff(..., PROT_READ | PROT_WRITE, ...)" in
		// fs/aio.c:aio_setup_ring(). Since we don't implement AIO_RING_MAGIC,
		// user mode should not write to this page.
		Perms:    hostarch.Read,
		MaxPerms: hostarch.Read,
	})
	if err != nil {
		return 0, err
	}
	id := uint64(addr)
	if !mm.aioManager.newAIOContext(events, id) {
		mm.MUnmap(ctx, addr, aioRingBufferSize)
		return 0, linuxerr.EINVAL
	}
	return id, nil
}

// DestroyAIOContext destroys an asynchronous I/O context. It returns the
// destroyed context. nil if the context does not exist.
func (mm *MemoryManager) DestroyAIOContext(ctx context.Context, id uint64) *AIOContext {
	if !mm.isValidAddr(ctx, id) {
		return nil
	}

	// Only unmaps after it assured that the address is a valid aio context to
	// prevent random memory from been unmapped.
	//
	// Note: It's possible to unmap this address and map something else into
	// the same address. Then it would be unmapping memory that it doesn't own.
	// This is, however, the way Linux implements AIO. Keeps the same [weird]
	// semantics in case anyone relies on it.
	mm.MUnmap(ctx, hostarch.Addr(id), aioRingBufferSize)

	mm.aioManager.mu.Lock()
	defer mm.aioManager.mu.Unlock()
	return mm.destroyAIOContextLocked(ctx, id)
}

// LookupAIOContext looks up the given context. It returns false if the context
// does not exist.
func (mm *MemoryManager) LookupAIOContext(ctx context.Context, id uint64) (*AIOContext, bool) {
	aioCtx, ok := mm.aioManager.lookupAIOContext(id)
	if !ok {
		return nil, false
	}

	// Protect against 'id' that is inaccessible.
	if !mm.isValidAddr(ctx, id) {
		return nil, false
	}

	return aioCtx, true
}

// isValidAddr determines if the address `id` is valid. (Linux also reads 4
// bytes from id).
func (mm *MemoryManager) isValidAddr(ctx context.Context, id uint64) bool {
	var buf [4]byte
	_, err := mm.CopyIn(ctx, hostarch.Addr(id), buf[:], usermem.IOOpts{})
	return err == nil
}
