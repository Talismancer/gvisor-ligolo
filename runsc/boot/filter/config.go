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

package filter

import (
	"os"

	"github.com/talismancer/gvisor-ligolo/pkg/abi/linux"
	"github.com/talismancer/gvisor-ligolo/pkg/seccomp"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/link/fdbased"
	"golang.org/x/sys/unix"
)

// allowedSyscalls is the set of syscalls executed by the Sentry to the host OS.
var allowedSyscalls = seccomp.SyscallRules{
	unix.SYS_CLOCK_GETTIME: {},
	unix.SYS_CLOSE:         {},
	unix.SYS_DUP:           {},
	unix.SYS_DUP3: []seccomp.Rule{
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.O_CLOEXEC),
		},
	},
	unix.SYS_EPOLL_CREATE1: {},
	unix.SYS_EPOLL_CTL:     {},
	unix.SYS_EPOLL_PWAIT: []seccomp.Rule{
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.EqualTo(0),
		},
	},
	unix.SYS_EVENTFD2: []seccomp.Rule{
		{
			seccomp.EqualTo(0),
			seccomp.EqualTo(0),
		},
	},
	unix.SYS_EXIT:       {},
	unix.SYS_EXIT_GROUP: {},
	unix.SYS_FALLOCATE:  {},
	unix.SYS_FCHMOD:     {},
	unix.SYS_FCNTL: []seccomp.Rule{
		{
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.F_GETFL),
		},
		{
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.F_SETFL),
		},
		{
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.F_GETFD),
		},
	},
	unix.SYS_FSTAT:     {},
	unix.SYS_FSYNC:     {},
	unix.SYS_FTRUNCATE: {},
	unix.SYS_FUTEX: []seccomp.Rule{
		{
			seccomp.MatchAny{},
			seccomp.EqualTo(linux.FUTEX_WAIT | linux.FUTEX_PRIVATE_FLAG),
			seccomp.MatchAny{},
			seccomp.MatchAny{},
		},
		{
			seccomp.MatchAny{},
			seccomp.EqualTo(linux.FUTEX_WAKE | linux.FUTEX_PRIVATE_FLAG),
			seccomp.MatchAny{},
		},
		// Non-private variants are included for flipcall support. They are otherwise
		// unncessary, as the sentry will use only private futexes internally.
		{
			seccomp.MatchAny{},
			seccomp.EqualTo(linux.FUTEX_WAIT),
			seccomp.MatchAny{},
			seccomp.MatchAny{},
		},
		{
			seccomp.MatchAny{},
			seccomp.EqualTo(linux.FUTEX_WAKE),
			seccomp.MatchAny{},
		},
	},
	// getcpu is used by some versions of the Go runtime and by the hostcpu
	// package on arm64.
	unix.SYS_GETCPU: []seccomp.Rule{
		{
			seccomp.MatchAny{},
			seccomp.EqualTo(0),
			seccomp.EqualTo(0),
		},
	},
	unix.SYS_GETPID:    {},
	unix.SYS_GETRANDOM: {},
	unix.SYS_GETSOCKOPT: []seccomp.Rule{
		{
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.SOL_SOCKET),
			seccomp.EqualTo(unix.SO_DOMAIN),
		},
		{
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.SOL_SOCKET),
			seccomp.EqualTo(unix.SO_TYPE),
		},
		{
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.SOL_SOCKET),
			seccomp.EqualTo(unix.SO_ERROR),
		},
		{
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.SOL_SOCKET),
			seccomp.EqualTo(unix.SO_SNDBUF),
		},
	},
	unix.SYS_GETTID:       {},
	unix.SYS_GETTIMEOFDAY: {},
	unix.SYS_IOCTL: []seccomp.Rule{
		// These commands are needed for host FD.
		{
			seccomp.MatchAny{}, /* fd */
			seccomp.EqualTo(linux.FIONREAD),
			seccomp.MatchAny{}, /* int* */
		},
		// These commands are needed for terminal support, but we only allow
		// setting/getting termios and winsize.
		{
			seccomp.MatchAny{}, /* fd */
			seccomp.EqualTo(linux.TCGETS),
			seccomp.MatchAny{}, /* termios struct */
		},
		{
			seccomp.MatchAny{}, /* fd */
			seccomp.EqualTo(linux.TCSETS),
			seccomp.MatchAny{}, /* termios struct */
		},
		{
			seccomp.MatchAny{}, /* fd */
			seccomp.EqualTo(linux.TCSETSF),
			seccomp.MatchAny{}, /* termios struct */
		},
		{
			seccomp.MatchAny{}, /* fd */
			seccomp.EqualTo(linux.TCSETSW),
			seccomp.MatchAny{}, /* termios struct */
		},
		{
			seccomp.MatchAny{}, /* fd */
			seccomp.EqualTo(linux.TIOCSWINSZ),
			seccomp.MatchAny{}, /* winsize struct */
		},
		{
			seccomp.MatchAny{}, /* fd */
			seccomp.EqualTo(linux.TIOCGWINSZ),
			seccomp.MatchAny{}, /* winsize struct */
		},
		{
			seccomp.MatchAny{}, /* fd */
			seccomp.EqualTo(linux.SIOCGIFTXQLEN),
			seccomp.MatchAny{}, /* ifreq struct */
		},
	},
	unix.SYS_LSEEK:   {},
	unix.SYS_MADVISE: {},
	unix.SYS_MEMBARRIER: []seccomp.Rule{
		{
			seccomp.EqualTo(linux.MEMBARRIER_CMD_GLOBAL),
			seccomp.EqualTo(0),
		},
	},
	unix.SYS_MINCORE: {},
	unix.SYS_MLOCK:   {},
	unix.SYS_MMAP: []seccomp.Rule{
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.MAP_SHARED),
		},
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.MAP_SHARED | unix.MAP_FIXED),
		},
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.MAP_PRIVATE),
		},
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.MAP_PRIVATE | unix.MAP_ANONYMOUS),
		},
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.MAP_PRIVATE | unix.MAP_ANONYMOUS | unix.MAP_STACK),
		},
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.MAP_PRIVATE | unix.MAP_ANONYMOUS | unix.MAP_NORESERVE),
		},
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.PROT_WRITE | unix.PROT_READ),
			seccomp.EqualTo(unix.MAP_PRIVATE | unix.MAP_ANONYMOUS | unix.MAP_FIXED),
		},
	},
	unix.SYS_MPROTECT:  {},
	unix.SYS_MUNLOCK:   {},
	unix.SYS_MUNMAP:    {},
	unix.SYS_NANOSLEEP: {},
	unix.SYS_PPOLL:     {},
	unix.SYS_PREAD64:   {},
	unix.SYS_PREADV:    {},
	unix.SYS_PREADV2:   {},
	unix.SYS_PWRITE64:  {},
	unix.SYS_PWRITEV:   {},
	unix.SYS_PWRITEV2:  {},
	unix.SYS_READ:      {},
	unix.SYS_RECVMSG: []seccomp.Rule{
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.MSG_DONTWAIT | unix.MSG_TRUNC),
		},
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.MSG_DONTWAIT | unix.MSG_TRUNC | unix.MSG_PEEK),
		},
	},
	unix.SYS_RECVMMSG: []seccomp.Rule{
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.EqualTo(fdbased.MaxMsgsPerRecv),
			seccomp.EqualTo(unix.MSG_DONTWAIT),
			seccomp.EqualTo(0),
		},
	},
	unix.SYS_SENDMMSG: []seccomp.Rule{
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.MSG_DONTWAIT),
		},
	},
	unix.SYS_RESTART_SYSCALL: {},
	unix.SYS_RT_SIGACTION:    {},
	unix.SYS_RT_SIGPROCMASK:  {},
	unix.SYS_RT_SIGRETURN:    {},
	unix.SYS_SCHED_YIELD:     {},
	unix.SYS_SENDMSG: []seccomp.Rule{
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.EqualTo(unix.MSG_DONTWAIT | unix.MSG_NOSIGNAL),
		},
	},
	unix.SYS_SETITIMER: {},
	unix.SYS_SHUTDOWN: []seccomp.Rule{
		// Used by fs/host to shutdown host sockets.
		{seccomp.MatchAny{}, seccomp.EqualTo(unix.SHUT_RD)},
		{seccomp.MatchAny{}, seccomp.EqualTo(unix.SHUT_WR)},
		// Used by unet to shutdown connections.
		{seccomp.MatchAny{}, seccomp.EqualTo(unix.SHUT_RDWR)},
	},
	unix.SYS_SIGALTSTACK:     {},
	unix.SYS_STATX:           {},
	unix.SYS_SYNC_FILE_RANGE: {},
	unix.SYS_TEE: []seccomp.Rule{
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.EqualTo(1),                      /* len */
			seccomp.EqualTo(unix.SPLICE_F_NONBLOCK), /* flags */
		},
	},
	unix.SYS_TIMER_CREATE: []seccomp.Rule{
		{
			seccomp.EqualTo(unix.CLOCK_THREAD_CPUTIME_ID), /* which */
			seccomp.MatchAny{},                            /* sevp */
			seccomp.MatchAny{},                            /* timerid */
		},
	},
	unix.SYS_TIMER_DELETE: []seccomp.Rule{},
	unix.SYS_TIMER_SETTIME: []seccomp.Rule{
		{
			seccomp.MatchAny{}, /* timerid */
			seccomp.EqualTo(0), /* flags */
			seccomp.MatchAny{}, /* new_value */
			seccomp.EqualTo(0), /* old_value */
		},
	},
	unix.SYS_TGKILL: []seccomp.Rule{
		{
			seccomp.EqualTo(uint64(os.Getpid())),
		},
	},
	unix.SYS_UTIMENSAT: []seccomp.Rule{
		{
			seccomp.MatchAny{},
			seccomp.EqualTo(0), /* null pathname */
			seccomp.MatchAny{},
			seccomp.EqualTo(0), /* flags */
		},
	},
	unix.SYS_WRITE: {},
	// For rawfile.NonBlockingWriteIovec.
	unix.SYS_WRITEV: []seccomp.Rule{
		{
			seccomp.MatchAny{},
			seccomp.MatchAny{},
			seccomp.GreaterThan(0),
		},
	},
}

func controlServerFilters(fd int) seccomp.SyscallRules {
	return seccomp.SyscallRules{
		unix.SYS_ACCEPT4: []seccomp.Rule{
			{
				seccomp.EqualTo(fd),
			},
		},
		unix.SYS_LISTEN: []seccomp.Rule{
			{
				seccomp.EqualTo(fd),
				seccomp.EqualTo(16 /* unet.backlog */),
			},
		},
		unix.SYS_GETSOCKOPT: []seccomp.Rule{
			{
				seccomp.MatchAny{},
				seccomp.EqualTo(unix.SOL_SOCKET),
				seccomp.EqualTo(unix.SO_PEERCRED),
			},
		},
	}
}

// hostFilesystemFilters contains syscalls that are needed by directfs.
func hostFilesystemFilters() seccomp.SyscallRules {
	// Directfs allows FD-based filesystem syscalls. We deny these syscalls with
	// negative FD values (like AT_FDCWD or invalid FD numbers). We try to be as
	// restrictive as possible because any restriction here improves security. We
	// don't know what set of arguments will trigger a future vulnerability.
	validFDCheck := seccomp.NonNegativeFDCheck()
	return seccomp.SyscallRules{
		unix.SYS_FCHOWNAT: []seccomp.Rule{
			{
				validFDCheck,
				seccomp.MatchAny{},
				seccomp.MatchAny{},
				seccomp.MatchAny{},
				seccomp.EqualTo(unix.AT_EMPTY_PATH | unix.AT_SYMLINK_NOFOLLOW),
			},
		},
		unix.SYS_FCHMODAT: []seccomp.Rule{
			{
				validFDCheck,
				seccomp.MatchAny{},
				seccomp.MatchAny{},
			},
		},
		unix.SYS_UNLINKAT: []seccomp.Rule{
			{
				validFDCheck,
				seccomp.MatchAny{},
				seccomp.MatchAny{},
			},
		},
		unix.SYS_GETDENTS64: []seccomp.Rule{
			{
				validFDCheck,
				seccomp.MatchAny{},
				seccomp.MatchAny{},
			},
		},
		unix.SYS_OPENAT: []seccomp.Rule{
			{
				validFDCheck,
				seccomp.MatchAny{},
				seccomp.MaskedEqual(unix.O_NOFOLLOW, unix.O_NOFOLLOW),
				seccomp.MatchAny{},
			},
		},
		unix.SYS_LINKAT: []seccomp.Rule{
			{
				validFDCheck,
				seccomp.MatchAny{},
				validFDCheck,
				seccomp.MatchAny{},
				seccomp.EqualTo(0),
			},
		},
		unix.SYS_MKDIRAT: []seccomp.Rule{
			{
				validFDCheck,
				seccomp.MatchAny{},
				seccomp.MatchAny{},
			},
		},
		unix.SYS_MKNODAT: []seccomp.Rule{
			{
				validFDCheck,
				seccomp.MatchAny{},
				seccomp.MatchAny{},
				seccomp.MatchAny{},
			},
		},
		unix.SYS_SYMLINKAT: []seccomp.Rule{
			{
				seccomp.MatchAny{},
				validFDCheck,
				seccomp.MatchAny{},
			},
		},
		unix.SYS_FSTATFS: []seccomp.Rule{
			{
				validFDCheck,
				seccomp.MatchAny{},
			},
		},
		unix.SYS_READLINKAT: []seccomp.Rule{
			{
				validFDCheck,
				seccomp.MatchAny{},
				seccomp.MatchAny{},
				seccomp.MatchAny{},
			},
		},
		unix.SYS_UTIMENSAT: []seccomp.Rule{
			{
				validFDCheck,
				seccomp.MatchAny{},
				seccomp.MatchAny{},
				seccomp.MatchAny{},
			},
		},
		unix.SYS_RENAMEAT: []seccomp.Rule{
			{
				validFDCheck,
				seccomp.MatchAny{},
				validFDCheck,
				seccomp.MatchAny{},
			},
		},
		archFstatAtSysNo(): []seccomp.Rule{
			{
				validFDCheck,
				seccomp.MatchAny{},
				seccomp.MatchAny{},
				seccomp.MatchAny{},
			},
		},
	}
}
