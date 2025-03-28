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

//go:build amd64
// +build amd64

package arch

import (
	"bytes"
	"fmt"
	"math/rand"

	"github.com/talismancer/gvisor-ligolo/pkg/hostarch"
	"github.com/talismancer/gvisor-ligolo/pkg/marshal"
	"github.com/talismancer/gvisor-ligolo/pkg/marshal/primitive"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/arch/fpu"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/limits"
	"golang.org/x/sys/unix"
)

// Host specifies the host architecture.
const Host = AMD64

// These constants come directly from Linux.
const (
	// maxAddr64 is the maximum userspace address. It is TASK_SIZE in Linux
	// for a 64-bit process.
	maxAddr64 hostarch.Addr = (1 << 47) - hostarch.PageSize

	// maxStackRand64 is the maximum randomization to apply to the stack.
	// It is defined by arch/x86/mm/mmap.c:stack_maxrandom_size in Linux.
	maxStackRand64 = 16 << 30 // 16 GB

	// maxMmapRand64 is the maximum randomization to apply to the mmap
	// layout. It is defined by arch/x86/mm/mmap.c:arch_mmap_rnd in Linux.
	maxMmapRand64 = (1 << 28) * hostarch.PageSize

	// minGap64 is the minimum gap to leave at the top of the address space
	// for the stack. It is defined by arch/x86/mm/mmap.c:MIN_GAP in Linux.
	minGap64 = (128 << 20) + maxStackRand64

	// preferredPIELoadAddr is the standard Linux position-independent
	// executable base load address. It is ELF_ET_DYN_BASE in Linux.
	//
	// The Platform {Min,Max}UserAddress() may preclude loading at this
	// address. See other preferredFoo comments below.
	preferredPIELoadAddr hostarch.Addr = maxAddr64 / 3 * 2
)

// These constants are selected as heuristics to help make the Platform's
// potentially limited address space conform as closely to Linux as possible.
const (
	// Select a preferred minimum TopDownBase address.
	//
	// Some applications (TSAN and other *SANs) are very particular about
	// the way the Linux mmap allocator layouts out the address space.
	//
	// TSAN in particular expects top down allocations to be made in the
	// range [0x7e8000000000, 0x800000000000).
	//
	// The minimum TopDownBase on Linux would be:
	// 0x800000000000 - minGap64 - maxMmapRand64 = 0x7efbf8000000.
	//
	// (minGap64 because TSAN uses a small RLIMIT_STACK.)
	//
	// 0x7e8000000000 is selected arbitrarily by TSAN to leave room for
	// allocations below TopDownBase.
	//
	// N.B. ASAN and MSAN are more forgiving; ASAN allows allocations all
	// the way down to 0x10007fff8000, and MSAN down to 0x700000000000.
	//
	// Of course, there is no hard minimum to allocation; an allocator can
	// search all the way from TopDownBase to Min. However, TSAN declared
	// their range "good enough".
	//
	// We would like to pick a TopDownBase such that it is unlikely that an
	// allocator will select an address below TSAN's minimum. We achieve
	// this by trying to leave a sizable gap below TopDownBase.
	//
	// This is all "preferred" because the layout min/max address may not
	// allow us to select such a TopDownBase, in which case we have to fall
	// back to a layout that TSAN may not be happy with.
	preferredTopDownAllocMin hostarch.Addr = 0x7e8000000000
	preferredAllocationGap                 = 128 << 30 // 128 GB
	preferredTopDownBaseMin                = preferredTopDownAllocMin + preferredAllocationGap

	// minMmapRand64 is the smallest we are willing to make the
	// randomization to stay above preferredTopDownBaseMin.
	minMmapRand64 = (1 << 26) * hostarch.PageSize
)

// Context64 represents an AMD64 context.
//
// +stateify savable
type Context64 struct {
	State
}

// Arch implements Context.Arch.
func (c *Context64) Arch() Arch {
	return AMD64
}

// FloatingPointData returns the state of the floating-point unit.
func (c *Context64) FloatingPointData() *fpu.State {
	return &c.State.fpState
}

// Fork returns an exact copy of this context.
func (c *Context64) Fork() *Context64 {
	return &Context64{
		State: c.State.Fork(),
	}
}

// Return returns the current syscall return value.
func (c *Context64) Return() uintptr {
	return uintptr(c.Regs.Rax)
}

// SetReturn sets the syscall return value.
func (c *Context64) SetReturn(value uintptr) {
	c.Regs.Rax = uint64(value)
}

// IP returns the current instruction pointer.
func (c *Context64) IP() uintptr {
	return uintptr(c.Regs.Rip)
}

// SetIP sets the current instruction pointer.
func (c *Context64) SetIP(value uintptr) {
	c.Regs.Rip = uint64(value)
}

// Stack returns the current stack pointer.
func (c *Context64) Stack() uintptr {
	return uintptr(c.Regs.Rsp)
}

// SetStack sets the current stack pointer.
func (c *Context64) SetStack(value uintptr) {
	c.Regs.Rsp = uint64(value)
}

// TLS returns the current TLS pointer.
func (c *Context64) TLS() uintptr {
	return uintptr(c.Regs.Fs_base)
}

// SetTLS sets the current TLS pointer. Returns false if value is invalid.
func (c *Context64) SetTLS(value uintptr) bool {
	if !isValidSegmentBase(uint64(value)) {
		return false
	}

	c.Regs.Fs = 0
	c.Regs.Fs_base = uint64(value)
	return true
}

// SetOldRSeqInterruptedIP implements Context.SetOldRSeqInterruptedIP.
func (c *Context64) SetOldRSeqInterruptedIP(value uintptr) {
	c.Regs.R10 = uint64(value)
}

// Native returns the native type for the given val.
func (c *Context64) Native(val uintptr) marshal.Marshallable {
	v := primitive.Uint64(val)
	return &v
}

// Value returns the generic val for the given native type.
func (c *Context64) Value(val marshal.Marshallable) uintptr {
	return uintptr(*val.(*primitive.Uint64))
}

// Width returns the byte width of this architecture.
func (c *Context64) Width() uint {
	return 8
}

// mmapRand returns a random adjustment for randomizing an mmap layout.
func mmapRand(max uint64) hostarch.Addr {
	return hostarch.Addr(rand.Int63n(int64(max))).RoundDown()
}

// NewMmapLayout implements Context.NewMmapLayout consistently with Linux.
func (c *Context64) NewMmapLayout(min, max hostarch.Addr, r *limits.LimitSet) (MmapLayout, error) {
	min, ok := min.RoundUp()
	if !ok {
		return MmapLayout{}, unix.EINVAL
	}
	if max > maxAddr64 {
		max = maxAddr64
	}
	max = max.RoundDown()

	if min > max {
		return MmapLayout{}, unix.EINVAL
	}

	stackSize := r.Get(limits.Stack)

	// MAX_GAP in Linux.
	maxGap := (max / 6) * 5
	gap := hostarch.Addr(stackSize.Cur)
	if gap < minGap64 {
		gap = minGap64
	}
	if gap > maxGap {
		gap = maxGap
	}
	defaultDir := MmapTopDown
	if stackSize.Cur == limits.Infinity {
		defaultDir = MmapBottomUp
	}

	topDownMin := max - gap - maxMmapRand64
	maxRand := hostarch.Addr(maxMmapRand64)
	if topDownMin < preferredTopDownBaseMin {
		// Try to keep TopDownBase above preferredTopDownBaseMin by
		// shrinking maxRand.
		maxAdjust := maxRand - minMmapRand64
		needAdjust := preferredTopDownBaseMin - topDownMin
		if needAdjust <= maxAdjust {
			maxRand -= needAdjust
		}
	}

	rnd := mmapRand(uint64(maxRand))
	l := MmapLayout{
		MinAddr: min,
		MaxAddr: max,
		// TASK_UNMAPPED_BASE in Linux.
		BottomUpBase:     (max/3 + rnd).RoundDown(),
		TopDownBase:      (max - gap - rnd).RoundDown(),
		DefaultDirection: defaultDir,
		// We may have reduced the maximum randomization to keep
		// TopDownBase above preferredTopDownBaseMin while maintaining
		// our stack gap. Stack allocations must use that max
		// randomization to avoiding eating into the gap.
		MaxStackRand: uint64(maxRand),
	}

	// Final sanity check on the layout.
	if !l.Valid() {
		panic(fmt.Sprintf("Invalid MmapLayout: %+v", l))
	}

	return l, nil
}

// PIELoadAddress implements Context.PIELoadAddress.
func (c *Context64) PIELoadAddress(l MmapLayout) hostarch.Addr {
	base := preferredPIELoadAddr
	max, ok := base.AddLength(maxMmapRand64)
	if !ok {
		panic(fmt.Sprintf("preferredPIELoadAddr %#x too large", base))
	}

	if max > l.MaxAddr {
		// preferredPIELoadAddr won't fit; fall back to the standard
		// Linux behavior of 2/3 of TopDownBase. TSAN won't like this.
		//
		// Don't bother trying to shrink the randomization for now.
		base = l.TopDownBase / 3 * 2
	}

	return base + mmapRand(maxMmapRand64)
}

// userStructSize is the size in bytes of Linux's struct user on amd64.
const userStructSize = 928

// PtracePeekUser implements Context.PtracePeekUser.
func (c *Context64) PtracePeekUser(addr uintptr) (marshal.Marshallable, error) {
	if addr&7 != 0 || addr >= userStructSize {
		return nil, unix.EIO
	}
	// PTRACE_PEEKUSER and PTRACE_POKEUSER are only effective on regs and
	// u_debugreg, returning 0 or silently no-oping for other fields
	// respectively.
	if addr < uintptr(ptraceRegistersSize) {
		regs := c.ptraceGetRegs()
		buf := make([]byte, regs.SizeBytes())
		regs.MarshalUnsafe(buf)
		return c.Native(uintptr(hostarch.ByteOrder.Uint64(buf[addr:]))), nil
	}
	// Note: x86 debug registers are missing.
	return c.Native(0), nil
}

// PtracePokeUser implements Context.PtracePokeUser.
func (c *Context64) PtracePokeUser(addr, data uintptr) error {
	if addr&7 != 0 || addr >= userStructSize {
		return unix.EIO
	}
	if addr < uintptr(ptraceRegistersSize) {
		regs := c.ptraceGetRegs()
		buf := make([]byte, regs.SizeBytes())
		regs.MarshalUnsafe(buf)
		hostarch.ByteOrder.PutUint64(buf[addr:], uint64(data))
		_, err := c.PtraceSetRegs(bytes.NewBuffer(buf))
		return err
	}
	// Note: x86 debug registers are missing.
	return nil
}
