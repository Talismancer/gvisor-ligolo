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

// Package boot loads the kernel and runs a container.
package boot

import (
	"errors"
	"fmt"
	mrand "math/rand"
	"os"
	"runtime"
	gtime "time"

	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/syndtr/gocapability/capability"
	"github.com/talismancer/gvisor-ligolo/pkg/abi/linux"
	"github.com/talismancer/gvisor-ligolo/pkg/bpf"
	"github.com/talismancer/gvisor-ligolo/pkg/cleanup"
	"github.com/talismancer/gvisor-ligolo/pkg/context"
	"github.com/talismancer/gvisor-ligolo/pkg/coverage"
	"github.com/talismancer/gvisor-ligolo/pkg/cpuid"
	"github.com/talismancer/gvisor-ligolo/pkg/fd"
	"github.com/talismancer/gvisor-ligolo/pkg/log"
	"github.com/talismancer/gvisor-ligolo/pkg/memutil"
	"github.com/talismancer/gvisor-ligolo/pkg/rand"
	"github.com/talismancer/gvisor-ligolo/pkg/refs"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/control"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/fdimport"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/fsimpl/host"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/fsimpl/tmpfs"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/fsimpl/user"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/inet"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/kernel"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/kernel/auth"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/loader"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/pgalloc"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/platform"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/seccheck"
	pb "github.com/talismancer/gvisor-ligolo/pkg/sentry/seccheck/points/points_go_proto"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/socket/netfilter"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/time"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/usage"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/vfs"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/watchdog"
	"github.com/talismancer/gvisor-ligolo/pkg/sighandling"
	"github.com/talismancer/gvisor-ligolo/pkg/sync"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/link/ethernet"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/link/loopback"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/link/packetsocket"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/link/sniffer"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/network/arp"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/network/ipv4"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/network/ipv6"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/stack"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/transport/icmp"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/transport/raw"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/transport/tcp"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/transport/udp"
	"github.com/talismancer/gvisor-ligolo/runsc/boot/filter"
	_ "github.com/talismancer/gvisor-ligolo/runsc/boot/platforms" // register all platforms.
	pf "github.com/talismancer/gvisor-ligolo/runsc/boot/portforward"
	"github.com/talismancer/gvisor-ligolo/runsc/boot/pprof"
	"github.com/talismancer/gvisor-ligolo/runsc/config"
	"github.com/talismancer/gvisor-ligolo/runsc/profile"
	"github.com/talismancer/gvisor-ligolo/runsc/specutils"
	"github.com/talismancer/gvisor-ligolo/runsc/specutils/seccomp"
	"golang.org/x/sys/unix"

	// Top-level inet providers.
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/socket/hostinet"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/socket/netstack"

	// Include other supported socket providers.
	_ "github.com/talismancer/gvisor-ligolo/pkg/sentry/socket/netlink"
	_ "github.com/talismancer/gvisor-ligolo/pkg/sentry/socket/netlink/route"
	_ "github.com/talismancer/gvisor-ligolo/pkg/sentry/socket/netlink/uevent"
	_ "github.com/talismancer/gvisor-ligolo/pkg/sentry/socket/unix"
)

type containerInfo struct {
	conf *config.Config

	// spec is the base configuration for the root container.
	spec *specs.Spec

	// procArgs refers to the container's init task.
	procArgs kernel.CreateProcessArgs

	// stdioFDs contains stdin, stdout, and stderr.
	stdioFDs []*fd.FD

	// passFDs are mappings of user-supplied host to guest file descriptors.
	passFDs []fdMapping

	// execFD is the host file descriptor used for program execution.
	execFD *fd.FD

	// goferFDs are the FDs that attach the sandbox to the gofers.
	goferFDs []*fd.FD

	// overlayFilestoreFDs are the FDs to the regular files that will back the
	// tmpfs upper mount in the overlay mounts.
	overlayFilestoreFDs []*fd.FD

	// overlayMediums contains information about how the gofer mounts have been
	// overlaid. The first entry is for rootfs and the following entries are for
	// bind mounts in spec.Mounts (in the same order).
	overlayMediums []OverlayMedium

	// nvidiaUVMDevMajor is the device major number used for nvidia-uvm.
	nvidiaUVMDevMajor uint32
}

// Loader keeps state needed to start the kernel and run the container.
type Loader struct {
	// k is the kernel.
	k *kernel.Kernel

	// ctrl is the control server.
	ctrl *controller

	// root contains information about the root container in the sandbox.
	root containerInfo

	watchdog *watchdog.Watchdog

	// stopSignalForwarding disables forwarding of signals to the sandboxed
	// container. It should be called when a sandbox is destroyed.
	stopSignalForwarding func()

	// stopProfiling stops profiling started at container creation. It
	// should be called when a sandbox is destroyed.
	stopProfiling func()

	// PreSeccompCallback is called right before installing seccomp filters.
	PreSeccompCallback func()

	// restore is set to true if we are restoring a container.
	restore bool

	// sandboxID is the ID for the whole sandbox.
	sandboxID string

	// mountHints provides extra information about mounts for containers that
	// apply to the entire pod.
	mountHints *PodMountHints

	// productName is the value to show in
	// /sys/devices/virtual/dmi/id/product_name.
	productName string

	// nvidiaUVMDevMajor is the device major number used for nvidia-uvm.
	nvidiaUVMDevMajor uint32

	// mu guards processes and porForwardProxies.
	mu sync.Mutex

	// processes maps containers init process and invocation of exec. Root
	// processes are keyed with container ID and pid=0, while exec invocations
	// have the corresponding pid set.
	//
	// processes is guarded by mu.
	processes map[execID]*execProcess

	// portForwardProxies is a list of active port forwarding connections.
	//
	// portForwardProxies is guarded by mu.
	portForwardProxies []*pf.Proxy
}

// execID uniquely identifies a sentry process that is executed in a container.
type execID struct {
	cid string
	pid kernel.ThreadID
}

// execProcess contains the thread group and host TTY of a sentry process.
type execProcess struct {
	// tg will be nil for containers that haven't started yet.
	tg *kernel.ThreadGroup

	// tty will be nil if the process is not attached to a terminal.
	tty *host.TTYFileDescription

	// pidnsPath is the pid namespace path in spec
	pidnsPath string

	// hostTTY is present when creating a sub-container with terminal enabled.
	// TTY file is passed during container create and must be saved until
	// container start.
	hostTTY *fd.FD
}

// fdMapping maps guest to host file descriptors. Guest file descriptors are
// exposed to the application inside the sandbox through the FD table.
type fdMapping struct {
	guest int
	host  *fd.FD
}

// FDMapping is a helper type to represent a mapping from guest to host file
// descriptors. In contrast to the unexported fdMapping type, it does not imply
// file ownership.
type FDMapping struct {
	Guest int
	Host  int
}

func init() {
	// Initialize the random number generator.
	mrand.Seed(gtime.Now().UnixNano())
}

// Args are the arguments for New().
type Args struct {
	// Id is the sandbox ID.
	ID string
	// Spec is the sandbox specification.
	Spec *specs.Spec
	// Conf is the system configuration.
	Conf *config.Config
	// ControllerFD is the FD to the URPC controller. The Loader takes ownership
	// of this FD and may close it at any time.
	ControllerFD int
	// Device is an optional argument that is passed to the platform. The Loader
	// takes ownership of this file and may close it at any time.
	Device *os.File
	// GoferFDs is an array of FDs used to connect with the Gofer. The Loader
	// takes ownership of these FDs and may close them at any time.
	GoferFDs []int
	// StdioFDs is the stdio for the application. The Loader takes ownership of
	// these FDs and may close them at any time.
	StdioFDs []int
	// PassFDs are user-supplied FD mappings from host to guest descriptors.
	// The Loader takes ownership of these FDs and may close them at any time.
	PassFDs []FDMapping
	// ExecFD is the host file descriptor used for program execution.
	ExecFD int
	// OverlayFilestoreFDs are the FDs to the regular files that will back the
	// tmpfs upper mount in the overlay mounts.
	OverlayFilestoreFDs []int
	// OverlayMediums contains information about how the gofer mounts have been
	// overlaid. The first entry is for rootfs and the following entries are for
	// bind mounts in Spec.Mounts (in the same order).
	OverlayMediums []OverlayMedium
	// NumCPU is the number of CPUs to create inside the sandbox.
	NumCPU int
	// TotalMem is the initial amount of total memory to report back to the
	// container.
	TotalMem uint64
	// TotalHostMem is the total memory reported by host /proc/meminfo.
	TotalHostMem uint64
	// UserLogFD is the file descriptor to write user logs to.
	UserLogFD int
	// ProductName is the value to show in
	// /sys/devices/virtual/dmi/id/product_name.
	ProductName string
	// PodInitConfigFD is the file descriptor to a file passed in the
	//	--pod-init-config flag
	PodInitConfigFD int
	// SinkFDs is an ordered array of file descriptors to be used by seccheck
	// sinks configured from the --pod-init-config file.
	SinkFDs []int
	// ProfileOpts contains the set of profiles to enable and the
	// corresponding FDs where profile data will be written.
	ProfileOpts profile.Opts
}

// make sure stdioFDs are always the same on initial start and on restore
const startingStdioFD = 256

// New initializes a new kernel loader configured by spec.
// New also handles setting up a kernel for restoring a container.
func New(args Args) (*Loader, error) {
	stopProfiling := profile.Start(args.ProfileOpts)

	// Initialize seccheck points.
	seccheck.Initialize()

	// We initialize the rand package now to make sure /dev/urandom is pre-opened
	// on kernels that do not support getrandom(2).
	if err := rand.Init(); err != nil {
		return nil, fmt.Errorf("setting up rand: %w", err)
	}

	if err := usage.Init(); err != nil {
		return nil, fmt.Errorf("setting up memory usage: %w", err)
	}

	kernel.IOUringEnabled = args.Conf.IOUring

	info := containerInfo{
		conf:           args.Conf,
		spec:           args.Spec,
		overlayMediums: args.OverlayMediums,
	}

	// Make host FDs stable between invocations. Host FDs must map to the exact
	// same number when the sandbox is restored. Otherwise the wrong FD will be
	// used.
	newfd := startingStdioFD

	for _, stdioFD := range args.StdioFDs {
		// Check that newfd is unused to avoid clobbering over it.
		if _, err := unix.FcntlInt(uintptr(newfd), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
			if err != nil {
				return nil, fmt.Errorf("error checking for FD (%d) conflict: %w", newfd, err)
			}
			return nil, fmt.Errorf("unable to remap stdios, FD %d is already in use", newfd)
		}

		err := unix.Dup3(stdioFD, newfd, unix.O_CLOEXEC)
		if err != nil {
			return nil, fmt.Errorf("dup3 of stdios failed: %w", err)
		}
		info.stdioFDs = append(info.stdioFDs, fd.New(newfd))
		_ = unix.Close(stdioFD)
		newfd++
	}
	for _, goferFD := range args.GoferFDs {
		info.goferFDs = append(info.goferFDs, fd.New(goferFD))
	}
	for _, overlayFD := range args.OverlayFilestoreFDs {
		info.overlayFilestoreFDs = append(info.overlayFilestoreFDs, fd.New(overlayFD))
	}

	if args.ExecFD >= 0 {
		info.execFD = fd.New(args.ExecFD)
	}

	for _, customFD := range args.PassFDs {
		info.passFDs = append(info.passFDs, fdMapping{
			host:  fd.New(customFD.Host),
			guest: customFD.Guest,
		})
	}

	// Create kernel and platform.
	p, err := createPlatform(args.Conf, args.Device)
	if err != nil {
		return nil, fmt.Errorf("creating platform: %w", err)
	}
	if args.Conf.NVProxy && p.OwnsPageTables() {
		return nil, fmt.Errorf("--nvproxy is incompatible with platform %s: owns page tables", args.Conf.Platform)
	}
	k := &kernel.Kernel{
		Platform: p,
	}

	// Create memory file.
	mf, err := createMemoryFile()
	if err != nil {
		return nil, fmt.Errorf("creating memory file: %w", err)
	}
	k.SetMemoryFile(mf)

	// Create VDSO.
	//
	// Pass k as the platform since it is savable, unlike the actual platform.
	vdso, err := loader.PrepareVDSO(k)
	if err != nil {
		return nil, fmt.Errorf("creating vdso: %w", err)
	}

	// Create timekeeper.
	tk := kernel.NewTimekeeper(k, vdso.ParamPage.FileRange())
	tk.SetClocks(time.NewCalibratedClocks())

	if err := enableStrace(args.Conf); err != nil {
		return nil, fmt.Errorf("enabling strace: %w", err)
	}

	// Create capabilities.
	caps, err := specutils.Capabilities(args.Conf.EnableRaw, args.Spec.Process.Capabilities)
	if err != nil {
		return nil, fmt.Errorf("converting capabilities: %w", err)
	}

	// Convert the spec's additional GIDs to KGIDs.
	extraKGIDs := make([]auth.KGID, 0, len(args.Spec.Process.User.AdditionalGids))
	for _, GID := range args.Spec.Process.User.AdditionalGids {
		extraKGIDs = append(extraKGIDs, auth.KGID(GID))
	}

	// Create credentials.
	creds := auth.NewUserCredentials(
		auth.KUID(args.Spec.Process.User.UID),
		auth.KGID(args.Spec.Process.User.GID),
		extraKGIDs,
		caps,
		auth.NewRootUserNamespace())

	// Create root network namespace/stack.
	netns, err := newRootNetworkNamespace(args.Conf, tk, k, creds.UserNamespace)
	if err != nil {
		return nil, fmt.Errorf("creating network: %w", err)
	}

	if args.NumCPU == 0 {
		args.NumCPU = runtime.NumCPU()
	}
	log.Infof("CPUs: %d", args.NumCPU)
	runtime.GOMAXPROCS(args.NumCPU)

	if args.TotalHostMem > 0 {
		// As per tmpfs(5), the default size limit is 50% of total physical RAM.
		// See mm/shmem.c:shmem_default_max_blocks().
		tmpfs.SetDefaultSizeLimit(args.TotalHostMem / 2)
	}

	if args.TotalMem > 0 {
		// Adjust the total memory returned by the Sentry so that applications that
		// use /proc/meminfo can make allocations based on this limit.
		usage.MinimumTotalMemoryBytes = args.TotalMem
		usage.MaximumTotalMemoryBytes = args.TotalMem
		log.Infof("Setting total memory to %.2f GB", float64(args.TotalMem)/(1<<30))
	}

	// Initiate the Kernel object, which is required by the Context passed
	// to createVFS in order to mount (among other things) procfs.
	if err = k.Init(kernel.InitKernelArgs{
		FeatureSet:                  cpuid.HostFeatureSet().Fixed(),
		Timekeeper:                  tk,
		RootUserNamespace:           creds.UserNamespace,
		RootNetworkNamespace:        netns,
		ApplicationCores:            uint(args.NumCPU),
		Vdso:                        vdso,
		RootUTSNamespace:            kernel.NewUTSNamespace(args.Spec.Hostname, args.Spec.Hostname, creds.UserNamespace),
		RootIPCNamespace:            kernel.NewIPCNamespace(creds.UserNamespace),
		RootAbstractSocketNamespace: kernel.NewAbstractSocketNamespace(),
		PIDNamespace:                kernel.NewRootPIDNamespace(creds.UserNamespace),
	}); err != nil {
		return nil, fmt.Errorf("initializing kernel: %w", err)
	}

	if err := registerFilesystems(k, &info); err != nil {
		return nil, fmt.Errorf("registering filesystems: %w", err)
	}

	// Turn on packet logging if enabled.
	if args.Conf.LogPackets {
		log.Infof("Packet logging enabled")
		sniffer.LogPackets.Store(1)
	} else {
		log.Infof("Packet logging disabled")
		sniffer.LogPackets.Store(0)
	}

	// Create a watchdog.
	dogOpts := watchdog.DefaultOpts
	dogOpts.TaskTimeoutAction = args.Conf.WatchdogAction
	dog := watchdog.New(k, dogOpts)

	procArgs, err := createProcessArgs(args.ID, args.Spec, creds, k, k.RootPIDNamespace())
	if err != nil {
		return nil, fmt.Errorf("creating init process for root container: %w", err)
	}
	info.procArgs = procArgs

	if err := initCompatLogs(args.UserLogFD); err != nil {
		return nil, fmt.Errorf("initializing compat logs: %w", err)
	}

	mountHints, err := NewPodMountHints(args.Spec)
	if err != nil {
		return nil, fmt.Errorf("creating pod mount hints: %w", err)
	}

	// Set up host mount that will be used for imported fds.
	hostFilesystem, err := host.NewFilesystem(k.VFS())
	if err != nil {
		return nil, fmt.Errorf("failed to create hostfs filesystem: %w", err)
	}
	defer hostFilesystem.DecRef(k.SupervisorContext())
	k.SetHostMount(k.VFS().NewDisconnectedMount(hostFilesystem, nil, &vfs.MountOptions{}))

	if args.PodInitConfigFD >= 0 {
		if err := setupSeccheck(args.PodInitConfigFD, args.SinkFDs); err != nil {
			log.Warningf("unable to configure event session: %v", err)
		}
	}

	eid := execID{cid: args.ID}
	l := &Loader{
		k:                 k,
		watchdog:          dog,
		sandboxID:         args.ID,
		processes:         map[execID]*execProcess{eid: {}},
		mountHints:        mountHints,
		root:              info,
		stopProfiling:     stopProfiling,
		productName:       args.ProductName,
		nvidiaUVMDevMajor: info.nvidiaUVMDevMajor,
	}

	// We don't care about child signals; some platforms can generate a
	// tremendous number of useless ones (I'm looking at you, ptrace).
	if err := sighandling.IgnoreChildStop(); err != nil {
		return nil, fmt.Errorf("ignore child stop signals failed: %w", err)
	}

	// Create the control server using the provided FD.
	//
	// This must be done *after* we have initialized the kernel since the
	// controller is used to configure the kernel's network stack.
	ctrl, err := newController(args.ControllerFD, l)
	if err != nil {
		return nil, fmt.Errorf("creating control server: %w", err)
	}
	l.ctrl = ctrl

	// Only start serving after Loader is set to controller and controller is set
	// to Loader, because they are both used in the urpc methods.
	if err := ctrl.srv.StartServing(); err != nil {
		return nil, fmt.Errorf("starting control server: %w", err)
	}

	return l, nil
}

// createProcessArgs creates args that can be used with kernel.CreateProcess.
func createProcessArgs(id string, spec *specs.Spec, creds *auth.Credentials, k *kernel.Kernel, pidns *kernel.PIDNamespace) (kernel.CreateProcessArgs, error) {
	// Create initial limits.
	ls, err := createLimitSet(spec)
	if err != nil {
		return kernel.CreateProcessArgs{}, fmt.Errorf("creating limits: %w", err)
	}
	env, err := specutils.ResolveEnvs(spec.Process.Env)
	if err != nil {
		return kernel.CreateProcessArgs{}, fmt.Errorf("resolving env: %w", err)
	}

	wd := spec.Process.Cwd
	if wd == "" {
		wd = "/"
	}

	// Create the process arguments.
	procArgs := kernel.CreateProcessArgs{
		Argv:                    spec.Process.Args,
		Envv:                    env,
		WorkingDirectory:        wd,
		Credentials:             creds,
		Umask:                   0022,
		Limits:                  ls,
		MaxSymlinkTraversals:    linux.MaxSymlinkTraversals,
		UTSNamespace:            k.RootUTSNamespace(),
		IPCNamespace:            k.RootIPCNamespace(),
		AbstractSocketNamespace: k.RootAbstractSocketNamespace(),
		ContainerID:             id,
		PIDNamespace:            pidns,
	}

	return procArgs, nil
}

// Destroy cleans up all resources used by the loader.
//
// Note that this will block until all open control server connections have
// been closed. For that reason, this should NOT be called in a defer, because
// a panic in a control server rpc would then hang forever.
func (l *Loader) Destroy() {
	if l.stopSignalForwarding != nil {
		l.stopSignalForwarding()
	}
	l.watchdog.Stop()

	// Stop the control server. This will indirectly stop any
	// long-running control operations that are in flight, e.g.
	// profiling operations.
	l.ctrl.stop()

	// Release all kernel resources. This is only safe after we can no longer
	// save/restore.
	l.k.Release()

	// Release any dangling tcp connections.
	tcpip.ReleaseDanglingEndpoints()

	// In the success case, stdioFDs and goferFDs will only contain
	// released/closed FDs that ownership has been passed over to host FDs and
	// gofer sessions. Close them here in case of failure.
	for _, f := range l.root.stdioFDs {
		_ = f.Close()
	}
	for _, f := range l.root.passFDs {
		_ = f.host.Close()
	}
	for _, f := range l.root.goferFDs {
		_ = f.Close()
	}

	l.stopProfiling()
}

func createPlatform(conf *config.Config, deviceFile *os.File) (platform.Platform, error) {
	p, err := platform.Lookup(conf.Platform)
	if err != nil {
		panic(fmt.Sprintf("invalid platform %s: %s", conf.Platform, err))
	}
	log.Infof("Platform: %s", conf.Platform)
	return p.New(deviceFile)
}

func createMemoryFile() (*pgalloc.MemoryFile, error) {
	const memfileName = "runsc-memory"
	memfd, err := memutil.CreateMemFD(memfileName, 0)
	if err != nil {
		return nil, fmt.Errorf("error creating memfd: %w", err)
	}
	memfile := os.NewFile(uintptr(memfd), memfileName)
	// We can't enable pgalloc.MemoryFileOpts.UseHostMemcgPressure even if
	// there are memory cgroups specified, because at this point we're already
	// in a mount namespace in which the relevant cgroupfs is not visible.
	mf, err := pgalloc.NewMemoryFile(memfile, pgalloc.MemoryFileOpts{})
	if err != nil {
		_ = memfile.Close()
		return nil, fmt.Errorf("error creating pgalloc.MemoryFile: %w", err)
	}
	return mf, nil
}

// installSeccompFilters installs sandbox seccomp filters with the host.
func (l *Loader) installSeccompFilters() error {
	if l.PreSeccompCallback != nil {
		l.PreSeccompCallback()
	}
	if l.root.conf.DisableSeccomp {
		filter.Report("syscall filter is DISABLED. Running in less secure mode.")
	} else {
		hostnet := l.root.conf.Network == config.NetworkHost
		opts := filter.Options{
			Platform:              l.k.Platform,
			HostNetwork:           hostnet,
			HostNetworkRawSockets: hostnet && l.root.conf.EnableRaw,
			HostFilesystem:        l.root.conf.DirectFS,
			ProfileEnable:         l.root.conf.ProfileEnable,
			NVProxy:               l.root.conf.NVProxy,
			TPUProxy:              l.root.conf.TPUProxy,
			ControllerFD:          l.ctrl.srv.FD(),
		}
		if err := filter.Install(opts); err != nil {
			return fmt.Errorf("installing seccomp filters: %w", err)
		}
	}
	return nil
}

// Run runs the root container.
func (l *Loader) Run() error {
	err := l.run()
	l.ctrl.manager.startResultChan <- err
	if err != nil {
		// Give the controller some time to send the error to the
		// runtime. If we return too quickly here the process will exit
		// and the control connection will be closed before the error
		// is returned.
		gtime.Sleep(2 * gtime.Second)
		return err
	}
	return nil
}

func (l *Loader) run() error {
	if l.root.conf.Network == config.NetworkHost {
		// Delay host network configuration to this point because network namespace
		// is configured after the loader is created and before Run() is called.
		log.Debugf("Configuring host network")
		s := l.k.RootNetworkNamespace().Stack().(*hostinet.Stack)
		if err := s.Configure(l.root.conf.EnableRaw); err != nil {
			return err
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	eid := execID{cid: l.sandboxID}
	ep, ok := l.processes[eid]
	if !ok {
		return fmt.Errorf("trying to start deleted container %q", l.sandboxID)
	}

	// If we are restoring, we do not want to create a process.
	// l.restore is set by the container manager when a restore call is made.
	if !l.restore {
		if l.root.conf.ProfileEnable {
			pprof.Initialize()
		}

		// Finally done with all configuration. Setup filters before user code
		// is loaded.
		if err := l.installSeccompFilters(); err != nil {
			return err
		}

		// Create the root container init task. It will begin running
		// when the kernel is started.
		var (
			tg  *kernel.ThreadGroup
			err error
		)
		tg, ep.tty, err = l.createContainerProcess(true, l.sandboxID, &l.root)
		if err != nil {
			return err
		}

		if seccheck.Global.Enabled(seccheck.PointContainerStart) {
			evt := pb.Start{
				Id:       l.sandboxID,
				Cwd:      l.root.spec.Process.Cwd,
				Args:     l.root.spec.Process.Args,
				Terminal: l.root.spec.Process.Terminal,
			}
			fields := seccheck.Global.GetFieldSet(seccheck.PointContainerStart)
			if fields.Local.Contains(seccheck.FieldContainerStartEnv) {
				evt.Env = l.root.spec.Process.Env
			}
			if !fields.Context.Empty() {
				evt.ContextData = &pb.ContextData{}
				kernel.LoadSeccheckData(tg.Leader(), fields.Context, evt.ContextData)
			}
			_ = seccheck.Global.SentToSinks(func(c seccheck.Sink) error {
				return c.ContainerStart(context.Background(), fields, &evt)
			})
		}
	}

	ep.tg = l.k.GlobalInit()
	if ns, ok := specutils.GetNS(specs.PIDNamespace, l.root.spec); ok {
		ep.pidnsPath = ns.Path
	}

	// Handle signals by forwarding them to the root container process
	// (except for panic signal, which should cause a panic).
	l.stopSignalForwarding = sighandling.StartSignalForwarding(func(sig linux.Signal) {
		// Panic signal should cause a panic.
		if l.root.conf.PanicSignal != -1 && sig == linux.Signal(l.root.conf.PanicSignal) {
			panic("Signal-induced panic")
		}

		// Otherwise forward to root container.
		deliveryMode := DeliverToProcess
		if l.root.spec.Process.Terminal {
			// Since we are running with a console, we should forward the signal to
			// the foreground process group so that job control signals like ^C can
			// be handled properly.
			deliveryMode = DeliverToForegroundProcessGroup
		}
		log.Infof("Received external signal %d, mode: %s", sig, deliveryMode)
		if err := l.signal(l.sandboxID, 0, int32(sig), deliveryMode); err != nil {
			log.Warningf("error sending signal %s to container %q: %s", sig, l.sandboxID, err)
		}
	})

	log.Infof("Process should have started...")
	l.watchdog.Start()
	return l.k.Start()
}

// createSubcontainer creates a new container inside the sandbox.
func (l *Loader) createSubcontainer(cid string, tty *fd.FD) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	eid := execID{cid: cid}
	if _, ok := l.processes[eid]; ok {
		return fmt.Errorf("container %q already exists", cid)
	}
	l.processes[eid] = &execProcess{hostTTY: tty}
	return nil
}

// startSubcontainer starts a child container. It returns the thread group ID of
// the newly created process. Used FDs are either closed or released. It's safe
// for the caller to close any remaining files upon return.
func (l *Loader) startSubcontainer(spec *specs.Spec, conf *config.Config, cid string, stdioFDs, goferFDs, overlayFilestoreFDs []*fd.FD, overlayMediums []OverlayMedium) error {
	// Create capabilities.
	caps, err := specutils.Capabilities(conf.EnableRaw, spec.Process.Capabilities)
	if err != nil {
		return fmt.Errorf("creating capabilities: %w", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	ep := l.processes[execID{cid: cid}]
	if ep == nil {
		return fmt.Errorf("trying to start a deleted container %q", cid)
	}

	// Convert the spec's additional GIDs to KGIDs.
	extraKGIDs := make([]auth.KGID, 0, len(spec.Process.User.AdditionalGids))
	for _, GID := range spec.Process.User.AdditionalGids {
		extraKGIDs = append(extraKGIDs, auth.KGID(GID))
	}

	// Create credentials. We reuse the root user namespace because the
	// sentry currently supports only 1 mount namespace, which is tied to a
	// single user namespace. Thus we must run in the same user namespace
	// to access mounts.
	creds := auth.NewUserCredentials(
		auth.KUID(spec.Process.User.UID),
		auth.KGID(spec.Process.User.GID),
		extraKGIDs,
		caps,
		l.k.RootUserNamespace())

	var pidns *kernel.PIDNamespace
	if ns, ok := specutils.GetNS(specs.PIDNamespace, spec); ok {
		if ns.Path != "" {
			for _, p := range l.processes {
				if ns.Path == p.pidnsPath {
					log.Debugf("Joining PID namespace named %q", ns.Path)
					pidns = p.tg.PIDNamespace()
					break
				}
			}
		}
		if pidns == nil {
			log.Warningf("PID namespace %q not found, running in new PID namespace", ns.Path)
			pidns = l.k.RootPIDNamespace().NewChild(l.k.RootUserNamespace())
		}
		ep.pidnsPath = ns.Path
	} else {
		pidns = l.k.RootPIDNamespace()
	}

	info := &containerInfo{
		conf:                conf,
		spec:                spec,
		goferFDs:            goferFDs,
		overlayFilestoreFDs: overlayFilestoreFDs,
		overlayMediums:      overlayMediums,
		nvidiaUVMDevMajor:   l.nvidiaUVMDevMajor,
	}
	info.procArgs, err = createProcessArgs(cid, spec, creds, l.k, pidns)
	if err != nil {
		return fmt.Errorf("creating new process: %w", err)
	}

	// Use stdios or TTY depending on the spec configuration.
	if spec.Process.Terminal {
		if l := len(stdioFDs); l != 0 {
			return fmt.Errorf("using TTY, stdios not expected: %d", l)
		}
		if ep.hostTTY == nil {
			return fmt.Errorf("terminal enabled but no TTY provided. Did you set --console-socket on create?")
		}
		info.stdioFDs = []*fd.FD{ep.hostTTY, ep.hostTTY, ep.hostTTY}
		ep.hostTTY = nil
	} else {
		info.stdioFDs = stdioFDs
	}

	ep.tg, ep.tty, err = l.createContainerProcess(false, cid, info)
	if err != nil {
		return err
	}

	if seccheck.Global.Enabled(seccheck.PointContainerStart) {
		evt := pb.Start{
			Id:       cid,
			Cwd:      spec.Process.Cwd,
			Args:     spec.Process.Args,
			Terminal: spec.Process.Terminal,
		}
		fields := seccheck.Global.GetFieldSet(seccheck.PointContainerStart)
		if fields.Local.Contains(seccheck.FieldContainerStartEnv) {
			evt.Env = spec.Process.Env
		}
		if !fields.Context.Empty() {
			evt.ContextData = &pb.ContextData{}
			kernel.LoadSeccheckData(ep.tg.Leader(), fields.Context, evt.ContextData)
		}
		_ = seccheck.Global.SentToSinks(func(c seccheck.Sink) error {
			return c.ContainerStart(context.Background(), fields, &evt)
		})
	}

	l.k.StartProcess(ep.tg)
	return nil
}

func (l *Loader) createContainerProcess(root bool, cid string, info *containerInfo) (*kernel.ThreadGroup, *host.TTYFileDescription, error) {
	// Create the FD map, which will set stdin, stdout, and stderr.
	ctx := info.procArgs.NewContext(l.k)
	fdTable, ttyFile, err := createFDTable(ctx, info.spec.Process.Terminal, info.stdioFDs, info.passFDs, info.spec.Process.User)
	if err != nil {
		return nil, nil, fmt.Errorf("importing fds: %w", err)
	}
	// CreateProcess takes a reference on fdTable if successful. We won't need
	// ours either way.
	info.procArgs.FDTable = fdTable

	if info.execFD != nil {
		if info.procArgs.Filename != "" {
			return nil, nil, fmt.Errorf("process must either be started from a file or a filename, not both")
		}
		file, err := host.NewFD(ctx, l.k.HostMount(), info.execFD.FD(), &host.NewFDOptions{
			Readonly:     true,
			Savable:      true,
			VirtualOwner: true,
			UID:          auth.KUID(info.spec.Process.User.UID),
			GID:          auth.KGID(info.spec.Process.User.GID),
		})
		if err != nil {
			return nil, nil, err
		}
		defer file.DecRef(ctx)
		info.execFD.Release()

		info.procArgs.File = file
	}

	// Gofer FDs must be ordered and the first FD is always the rootfs.
	if len(info.goferFDs) < 1 {
		return nil, nil, fmt.Errorf("rootfs gofer FD not found")
	}
	l.startGoferMonitor(cid, int32(info.goferFDs[0].FD()))

	mntr := newContainerMounter(info, l.k, l.mountHints, l.productName, l.sandboxID)
	if root {
		if err := mntr.processHints(info.conf, info.procArgs.Credentials); err != nil {
			return nil, nil, err
		}
	}
	if err := setupContainerVFS(ctx, info, mntr, &info.procArgs); err != nil {
		return nil, nil, err
	}

	// Add the HOME environment variable if it is not already set.
	info.procArgs.Envv, err = user.MaybeAddExecUserHome(ctx, info.procArgs.MountNamespace,
		info.procArgs.Credentials.RealKUID, info.procArgs.Envv)
	if err != nil {
		return nil, nil, err
	}

	// Create and start the new process.
	tg, _, err := l.k.CreateProcess(info.procArgs)
	if err != nil {
		return nil, nil, fmt.Errorf("creating process: %w", err)
	}
	// CreateProcess takes a reference on FDTable if successful.
	info.procArgs.FDTable.DecRef(ctx)

	// Set the foreground process group on the TTY to the global init process
	// group, since that is what we are about to start running.
	if ttyFile != nil {
		ttyFile.InitForegroundProcessGroup(tg.ProcessGroup())
	}

	// Install seccomp filters with the new task if there are any.
	if info.conf.OCISeccomp {
		if info.spec.Linux != nil && info.spec.Linux.Seccomp != nil {
			program, err := seccomp.BuildProgram(info.spec.Linux.Seccomp)
			if err != nil {
				return nil, nil, fmt.Errorf("building seccomp program: %w", err)
			}

			if log.IsLogging(log.Debug) {
				out, _ := bpf.DecodeProgram(program)
				log.Debugf("Installing OCI seccomp filters\nProgram:\n%s", out)
			}

			task := tg.Leader()
			// NOTE: It seems Flags are ignored by runc so we ignore them too.
			if err := task.AppendSyscallFilter(program, true); err != nil {
				return nil, nil, fmt.Errorf("appending seccomp filters: %w", err)
			}
		}
	} else {
		if info.spec.Linux != nil && info.spec.Linux.Seccomp != nil {
			log.Warningf("Seccomp spec is being ignored")
		}
	}

	return tg, ttyFile, nil
}

// startGoferMonitor runs a goroutine to monitor gofer's health. It polls on
// the gofer FD looking for disconnects, and kills the container processes if
// the rootfs FD disconnects.
//
// Note that other gofer mounts are allowed to be unmounted and disconnected.
func (l *Loader) startGoferMonitor(cid string, rootfsGoferFD int32) {
	if rootfsGoferFD < 0 {
		panic(fmt.Sprintf("invalid FD: %d", rootfsGoferFD))
	}
	go func() {
		log.Debugf("Monitoring gofer health for container %q", cid)
		events := []unix.PollFd{
			{
				Fd:     rootfsGoferFD,
				Events: unix.POLLHUP | unix.POLLRDHUP,
			},
		}
		_, _, err := specutils.RetryEintr(func() (uintptr, uintptr, error) {
			// Use ppoll instead of poll because it's already allowed in seccomp.
			n, err := unix.Ppoll(events, nil, nil)
			return uintptr(n), 0, err
		})
		if err != nil {
			panic(fmt.Sprintf("Error monitoring gofer FDs: %s", err))
		}

		l.mu.Lock()
		defer l.mu.Unlock()

		// The gofer could have been stopped due to a normal container shutdown.
		// Check if the container has not stopped yet.
		if tg, _ := l.tryThreadGroupFromIDLocked(execID{cid: cid}); tg != nil {
			log.Infof("Gofer socket disconnected, killing container %q", cid)
			if err := l.signalAllProcesses(cid, int32(linux.SIGKILL)); err != nil {
				log.Warningf("Error killing container %q after gofer stopped: %s", cid, err)
			}
		}
	}()
}

// destroySubcontainer stops a container if it is still running and cleans up
// its filesystem.
func (l *Loader) destroySubcontainer(cid string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	tg, err := l.tryThreadGroupFromIDLocked(execID{cid: cid})
	if err != nil {
		// Container doesn't exist.
		return err
	}

	// The container exists, but has it been started?
	if tg != nil {
		if err := l.signalAllProcesses(cid, int32(linux.SIGKILL)); err != nil {
			return fmt.Errorf("sending SIGKILL to all container processes: %w", err)
		}
		// Wait for all processes that belong to the container to exit (including
		// exec'd processes).
		for _, t := range l.k.TaskSet().Root.Tasks() {
			if t.ContainerID() == cid {
				t.ThreadGroup().WaitExited()
			}
		}
	}

	// No more failure from this point on. Remove all container thread groups
	// from the map.
	for key := range l.processes {
		if key.cid == cid {
			delete(l.processes, key)
		}
	}

	log.Debugf("Container destroyed, cid: %s", cid)
	return nil
}

func (l *Loader) executeAsync(args *control.ExecArgs) (kernel.ThreadID, error) {
	// Hold the lock for the entire operation to ensure that exec'd process is
	// added to 'processes' in case it races with destroyContainer().
	l.mu.Lock()
	defer l.mu.Unlock()

	tg, err := l.tryThreadGroupFromIDLocked(execID{cid: args.ContainerID})
	if err != nil {
		return 0, err
	}
	if tg == nil {
		return 0, fmt.Errorf("container %q not started", args.ContainerID)
	}

	// Get the container MountNamespace from the Task. Try to acquire ref may fail
	// in case it raced with task exit.
	// task.MountNamespace() does not take a ref, so we must do so ourselves.
	args.MountNamespace = tg.Leader().MountNamespace()
	if args.MountNamespace == nil || !args.MountNamespace.TryIncRef() {
		return 0, fmt.Errorf("container %q has stopped", args.ContainerID)
	}

	args.Envv, err = specutils.ResolveEnvs(args.Envv)
	if err != nil {
		return 0, fmt.Errorf("resolving env: %w", err)
	}

	// Add the HOME environment variable if it is not already set.
	ctx := vfs.WithRoot(l.k.SupervisorContext(), args.MountNamespace.Root())
	defer args.MountNamespace.DecRef(ctx)
	args.Envv, err = user.MaybeAddExecUserHome(ctx, args.MountNamespace, args.KUID, args.Envv)
	if err != nil {
		return 0, err
	}
	args.PIDNamespace = tg.PIDNamespace()

	args.Limits, err = createLimitSet(l.root.spec)
	if err != nil {
		return 0, fmt.Errorf("creating limits: %w", err)
	}

	// Start the process.
	proc := control.Proc{Kernel: l.k}
	newTG, tgid, ttyFile, err := control.ExecAsync(&proc, args)
	if err != nil {
		return 0, err
	}

	eid := execID{cid: args.ContainerID, pid: tgid}
	l.processes[eid] = &execProcess{
		tg:  newTG,
		tty: ttyFile,
	}
	log.Debugf("updated processes: %v", l.processes)

	return tgid, nil
}

// waitContainer waits for the init process of a container to exit.
func (l *Loader) waitContainer(cid string, waitStatus *uint32) error {
	// Don't defer unlock, as doing so would make it impossible for
	// multiple clients to wait on the same container.
	tg, err := l.threadGroupFromID(execID{cid: cid})
	if err != nil {
		return fmt.Errorf("can't wait for container %q: %w", cid, err)
	}

	// If the thread either has already exited or exits during waiting,
	// consider the container exited.
	ws := l.wait(tg)
	*waitStatus = ws

	// Check for leaks and write coverage report after the root container has
	// exited. This guarantees that the report is written in cases where the
	// sandbox is killed by a signal after the ContMgrWait request is completed.
	if l.root.procArgs.ContainerID == cid {
		// All sentry-created resources should have been released at this point.
		refs.DoLeakCheck()
		_ = coverage.Report()
	}
	return nil
}

func (l *Loader) waitPID(tgid kernel.ThreadID, cid string, waitStatus *uint32) error {
	if tgid <= 0 {
		return fmt.Errorf("PID (%d) must be positive", tgid)
	}

	// Try to find a process that was exec'd
	eid := execID{cid: cid, pid: tgid}
	execTG, err := l.threadGroupFromID(eid)
	if err == nil {
		ws := l.wait(execTG)
		*waitStatus = ws

		l.mu.Lock()
		delete(l.processes, eid)
		log.Debugf("updated processes (removal): %v", l.processes)
		l.mu.Unlock()
		return nil
	}

	// The caller may be waiting on a process not started directly via exec.
	// In this case, find the process in the container's PID namespace.
	initTG, err := l.threadGroupFromID(execID{cid: cid})
	if err != nil {
		return fmt.Errorf("waiting for PID %d: %w", tgid, err)
	}
	tg := initTG.PIDNamespace().ThreadGroupWithID(tgid)
	if tg == nil {
		return fmt.Errorf("waiting for PID %d: no such process", tgid)
	}
	if tg.Leader().ContainerID() != cid {
		return fmt.Errorf("process %d is part of a different container: %q", tgid, tg.Leader().ContainerID())
	}
	ws := l.wait(tg)
	*waitStatus = ws
	return nil
}

// wait waits for the process with TGID 'tgid' in a container's PID namespace
// to exit.
func (l *Loader) wait(tg *kernel.ThreadGroup) uint32 {
	tg.WaitExited()
	return uint32(tg.ExitStatus())
}

// WaitForStartSignal waits for a start signal from the control server.
func (l *Loader) WaitForStartSignal() {
	<-l.ctrl.manager.startChan
}

// WaitExit waits for the root container to exit, and returns its exit status.
func (l *Loader) WaitExit() linux.WaitStatus {
	// Wait for container.
	l.k.WaitExited()

	// Check all references.
	refs.OnExit()

	return l.k.GlobalInit().ExitStatus()
}

func newRootNetworkNamespace(conf *config.Config, clock tcpip.Clock, uniqueID stack.UniqueID, userns *auth.UserNamespace) (*inet.Namespace, error) {
	// Create an empty network stack because the network namespace may be empty at
	// this point. Netns is configured before Run() is called. Netstack is
	// configured using a control uRPC message. Host network is configured inside
	// Run().
	switch conf.Network {
	case config.NetworkHost:
		// If configured for raw socket support with host network
		// stack, make sure that we have CAP_NET_RAW the host,
		// otherwise we can't make raw sockets.
		if conf.EnableRaw && !specutils.HasCapabilities(capability.CAP_NET_RAW) {
			return nil, fmt.Errorf("configuring network=host with raw sockets requires CAP_NET_RAW capability")
		}
		// No network namespacing support for hostinet yet, hence creator is nil.
		return inet.NewRootNamespace(hostinet.NewStack(), nil, userns), nil

	case config.NetworkNone, config.NetworkSandbox:
		s, err := newEmptySandboxNetworkStack(clock, uniqueID, conf.AllowPacketEndpointWrite)
		if err != nil {
			return nil, err
		}
		creator := &sandboxNetstackCreator{
			clock:                    clock,
			uniqueID:                 uniqueID,
			allowPacketEndpointWrite: conf.AllowPacketEndpointWrite,
		}
		return inet.NewRootNamespace(s, creator, userns), nil

	default:
		panic(fmt.Sprintf("invalid network configuration: %v", conf.Network))
	}

}

func newEmptySandboxNetworkStack(clock tcpip.Clock, uniqueID stack.UniqueID, allowPacketEndpointWrite bool) (inet.Stack, error) {
	netProtos := []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol, arp.NewProtocol}
	transProtos := []stack.TransportProtocolFactory{
		tcp.NewProtocol,
		udp.NewProtocol,
		icmp.NewProtocol4,
		icmp.NewProtocol6,
	}
	s := netstack.Stack{Stack: stack.New(stack.Options{
		NetworkProtocols:   netProtos,
		TransportProtocols: transProtos,
		Clock:              clock,
		Stats:              netstack.Metrics,
		HandleLocal:        true,
		// Enable raw sockets for users with sufficient
		// privileges.
		RawFactory:               raw.EndpointFactory{},
		AllowPacketEndpointWrite: allowPacketEndpointWrite,
		UniqueID:                 uniqueID,
		DefaultIPTables:          netfilter.DefaultLinuxTables,
	})}

	// Enable SACK Recovery.
	{
		opt := tcpip.TCPSACKEnabled(true)
		if err := s.Stack.SetTransportProtocolOption(tcp.ProtocolNumber, &opt); err != nil {
			return nil, fmt.Errorf("SetTransportProtocolOption(%d, &%T(%t)): %s", tcp.ProtocolNumber, opt, opt, err)
		}
	}

	// Set default TTLs as required by socket/netstack.
	{
		opt := tcpip.DefaultTTLOption(netstack.DefaultTTL)
		if err := s.Stack.SetNetworkProtocolOption(ipv4.ProtocolNumber, &opt); err != nil {
			return nil, fmt.Errorf("SetNetworkProtocolOption(%d, &%T(%d)): %s", ipv4.ProtocolNumber, opt, opt, err)
		}
		if err := s.Stack.SetNetworkProtocolOption(ipv6.ProtocolNumber, &opt); err != nil {
			return nil, fmt.Errorf("SetNetworkProtocolOption(%d, &%T(%d)): %s", ipv6.ProtocolNumber, opt, opt, err)
		}
	}

	// Enable Receive Buffer Auto-Tuning.
	{
		opt := tcpip.TCPModerateReceiveBufferOption(true)
		if err := s.Stack.SetTransportProtocolOption(tcp.ProtocolNumber, &opt); err != nil {
			return nil, fmt.Errorf("SetTransportProtocolOption(%d, &%T(%t)): %s", tcp.ProtocolNumber, opt, opt, err)
		}
	}

	return &s, nil
}

// sandboxNetstackCreator implements kernel.NetworkStackCreator.
//
// +stateify savable
type sandboxNetstackCreator struct {
	clock                    tcpip.Clock
	uniqueID                 stack.UniqueID
	allowPacketEndpointWrite bool
}

// CreateStack implements kernel.NetworkStackCreator.CreateStack.
func (f *sandboxNetstackCreator) CreateStack() (inet.Stack, error) {
	s, err := newEmptySandboxNetworkStack(f.clock, f.uniqueID, f.allowPacketEndpointWrite)
	if err != nil {
		return nil, err
	}

	// Setup loopback.
	n := &Network{Stack: s.(*netstack.Stack).Stack}
	nicID := tcpip.NICID(f.uniqueID.UniqueID())
	link := DefaultLoopbackLink
	linkEP := packetsocket.New(ethernet.New(loopback.New()))
	opts := stack.NICOptions{Name: link.Name}

	if err := n.createNICWithAddrs(nicID, linkEP, opts, link.Addresses); err != nil {
		return nil, err
	}

	return s, nil
}

// signal sends a signal to one or more processes in a container. If PID is 0,
// then the container init process is used. Depending on the SignalDeliveryMode
// option, the signal may be sent directly to the indicated process, to all
// processes in the container, or to the foreground process group. pid is
// relative to the root PID namespace, not the container's.
func (l *Loader) signal(cid string, pid, signo int32, mode SignalDeliveryMode) error {
	if pid < 0 {
		return fmt.Errorf("PID (%d) must be positive", pid)
	}

	switch mode {
	case DeliverToProcess:
		if err := l.signalProcess(cid, kernel.ThreadID(pid), signo); err != nil {
			return fmt.Errorf("signaling process in container %q PID %d: %w", cid, pid, err)
		}
		return nil

	case DeliverToForegroundProcessGroup:
		if err := l.signalForegrondProcessGroup(cid, kernel.ThreadID(pid), signo); err != nil {
			return fmt.Errorf("signaling foreground process group in container %q PID %d: %w", cid, pid, err)
		}
		return nil

	case DeliverToAllProcesses:
		if pid != 0 {
			return fmt.Errorf("PID (%d) cannot be set when signaling all processes", pid)
		}
		// Check that the container has actually started before signaling it.
		if _, err := l.threadGroupFromID(execID{cid: cid}); err != nil {
			return err
		}
		if err := l.signalAllProcesses(cid, signo); err != nil {
			return fmt.Errorf("signaling all processes in container %q: %w", cid, err)
		}
		return nil

	default:
		panic(fmt.Sprintf("unknown signal delivery mode %v", mode))
	}
}

// signalProcess sends signal to process in the given container. tgid is
// relative to the root PID namespace, not the container's.
func (l *Loader) signalProcess(cid string, tgid kernel.ThreadID, signo int32) error {
	execTG, err := l.threadGroupFromID(execID{cid: cid, pid: tgid})
	if err == nil {
		// Send signal directly to the identified process.
		return l.k.SendExternalSignalThreadGroup(execTG, &linux.SignalInfo{Signo: signo})
	}

	// The caller may be signaling a process not started directly via exec.
	// In this case, find the process and check that the process belongs to the
	// container in question.
	tg := l.k.RootPIDNamespace().ThreadGroupWithID(tgid)
	if tg == nil {
		return fmt.Errorf("no such process with PID %d", tgid)
	}
	if tg.Leader().ContainerID() != cid {
		return fmt.Errorf("process %d belongs to a different container: %q", tgid, tg.Leader().ContainerID())
	}
	return l.k.SendExternalSignalThreadGroup(tg, &linux.SignalInfo{Signo: signo})
}

// signalForegrondProcessGroup looks up foreground process group from the TTY
// for the given "tgid" inside container "cid", and send the signal to it.
func (l *Loader) signalForegrondProcessGroup(cid string, tgid kernel.ThreadID, signo int32) error {
	l.mu.Lock()
	tg, err := l.tryThreadGroupFromIDLocked(execID{cid: cid, pid: tgid})
	if err != nil {
		l.mu.Unlock()
		return fmt.Errorf("no thread group found: %w", err)
	}
	if tg == nil {
		l.mu.Unlock()
		return fmt.Errorf("container %q not started", cid)
	}

	tty, err := l.ttyFromIDLocked(execID{cid: cid, pid: tgid})
	l.mu.Unlock()
	if err != nil {
		return fmt.Errorf("no thread group found: %w", err)
	}
	if tty == nil {
		return fmt.Errorf("no TTY attached")
	}
	pg := tty.ForegroundProcessGroup()
	if pg == nil {
		// No foreground process group has been set. Signal the
		// original thread group.
		log.Warningf("No foreground process group for container %q and PID %d. Sending signal directly to PID %d.", cid, tgid, tgid)
		return l.k.SendExternalSignalThreadGroup(tg, &linux.SignalInfo{Signo: signo})
	}
	// Send the signal to all processes in the process group.
	var lastErr error
	for _, tg := range l.k.TaskSet().Root.ThreadGroups() {
		if tg.ProcessGroup() != pg {
			continue
		}
		if err := l.k.SendExternalSignalThreadGroup(tg, &linux.SignalInfo{Signo: signo}); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// signalAllProcesses that belong to specified container. It's a noop if the
// container hasn't started or has exited.
func (l *Loader) signalAllProcesses(cid string, signo int32) error {
	// Pause the kernel to prevent new processes from being created while
	// the signal is delivered. This prevents process leaks when SIGKILL is
	// sent to the entire container.
	l.k.Pause()
	defer l.k.Unpause()
	return l.k.SendContainerSignal(cid, &linux.SignalInfo{Signo: signo})
}

// threadGroupFromID is similar to tryThreadGroupFromIDLocked except that it
// acquires mutex before calling it and fails in case container hasn't started
// yet.
func (l *Loader) threadGroupFromID(key execID) (*kernel.ThreadGroup, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	tg, err := l.tryThreadGroupFromIDLocked(key)
	if err != nil {
		return nil, err
	}
	if tg == nil {
		return nil, fmt.Errorf("container %q not started", key.cid)
	}
	return tg, nil
}

// tryThreadGroupFromIDLocked returns the thread group for the given execution
// ID. It may return nil in case the container has not started yet. Returns
// error if execution ID is invalid or if the container cannot be found (maybe
// it has been deleted). Caller must hold 'mu'.
func (l *Loader) tryThreadGroupFromIDLocked(key execID) (*kernel.ThreadGroup, error) {
	ep := l.processes[key]
	if ep == nil {
		return nil, fmt.Errorf("container %q not found", key.cid)
	}
	return ep.tg, nil
}

// ttyFromIDLocked returns the TTY files for the given execution ID. It may
// return nil in case the container has not started yet. Returns error if
// execution ID is invalid or if the container cannot be found (maybe it has
// been deleted). Caller must hold 'mu'.
func (l *Loader) ttyFromIDLocked(key execID) (*host.TTYFileDescription, error) {
	ep := l.processes[key]
	if ep == nil {
		return nil, fmt.Errorf("container %q not found", key.cid)
	}
	return ep.tty, nil
}

func createFDTable(ctx context.Context, console bool, stdioFDs []*fd.FD, passFDs []fdMapping, user specs.User) (*kernel.FDTable, *host.TTYFileDescription, error) {
	if len(stdioFDs) != 3 {
		return nil, nil, fmt.Errorf("stdioFDs should contain exactly 3 FDs (stdin, stdout, and stderr), but %d FDs received", len(stdioFDs))
	}
	fdMap := map[int]*fd.FD{
		0: stdioFDs[0],
		1: stdioFDs[1],
		2: stdioFDs[2],
	}

	// Create the entries for the host files that were passed to our app.
	for _, customFD := range passFDs {
		if customFD.guest < 0 {
			return nil, nil, fmt.Errorf("guest file descriptors must be 0 or greater")
		}
		fdMap[customFD.guest] = customFD.host
	}

	k := kernel.KernelFromContext(ctx)
	fdTable := k.NewFDTable()
	ttyFile, err := fdimport.Import(ctx, fdTable, console, auth.KUID(user.UID), auth.KGID(user.GID), fdMap)
	if err != nil {
		fdTable.DecRef(ctx)
		return nil, nil, err
	}
	return fdTable, ttyFile, nil
}

// portForward implements initiating a portForward connection in the sandbox. portForwardProxies
// represent a two connections each copying to each other (read ends to write ends) in goroutines.
// The proxies are stored and can be cleaned up, or clean up after themselves if the connection
// is broken.
func (l *Loader) portForward(opts *PortForwardOpts) error {
	// Validate that we have a stream FD to write to. If this happens then
	// it means there is a misbehaved urpc client or a bug has occurred.
	if len(opts.Files) != 1 {
		return fmt.Errorf("stream FD is required for port forward")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	cid := opts.ContainerID
	tg, err := l.tryThreadGroupFromIDLocked(execID{cid: cid})
	if err != nil {
		return fmt.Errorf("failed to get threadgroup from %q: %w", cid, err)
	}
	if tg == nil {
		return fmt.Errorf("container %q not started", cid)
	}

	// Import the fd for the UDS.
	ctx := l.k.SupervisorContext()
	fd, err := l.importFD(ctx, opts.Files[0])
	if err != nil {
		return fmt.Errorf("importing stream fd: %w", err)
	}
	cu := cleanup.Make(func() { fd.DecRef(ctx) })
	defer cu.Clean()

	fdConn := pf.NewFileDescriptionConn(fd)

	// Create a proxy to forward data between the fdConn and the sandboxed application.
	pair := pf.ProxyPair{To: fdConn}

	switch l.root.conf.Network {
	case config.NetworkSandbox:
		stack := l.k.RootNetworkNamespace().Stack().(*netstack.Stack).Stack
		nsConn, err := pf.NewNetstackConn(stack, opts.Port)
		if err != nil {
			return fmt.Errorf("creating netstack port forward connection: %w", err)
		}
		pair.From = nsConn
	case config.NetworkHost:
		hConn, err := pf.NewHostInetConn(opts.Port)
		if err != nil {
			return fmt.Errorf("creating hostinet port forward connection: %w", err)
		}
		pair.From = hConn
	default:
		return fmt.Errorf("unsupported network type %q for container %q", l.root.conf.Network, cid)
	}
	cu.Release()
	proxy := pf.NewProxy(pair, opts.ContainerID)

	// Add to the list of port forward connections and remove when the
	// connection closes.
	l.portForwardProxies = append(l.portForwardProxies, proxy)
	proxy.AddCleanup(func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		for i := range l.portForwardProxies {
			if l.portForwardProxies[i] == proxy {
				l.portForwardProxies = append(l.portForwardProxies[:i], l.portForwardProxies[i+1:]...)
				break
			}
		}
	})

	// Start forwarding on the connection.
	proxy.Start(ctx)
	return nil
}

// importFD generically imports a host file descriptor without adding it to any
// fd table.
func (l *Loader) importFD(ctx context.Context, f *os.File) (*vfs.FileDescription, error) {
	hostFD, err := fd.NewFromFile(f)
	if err != nil {
		return nil, err
	}
	defer hostFD.Close()
	fd, err := host.NewFD(ctx, l.k.HostMount(), hostFD.FD(), &host.NewFDOptions{
		Savable:      false, // We disconnect and close on save.
		IsTTY:        false,
		VirtualOwner: false, // FD not visible to the sandboxed app so user can't be changed.
	})

	if err != nil {
		return nil, err
	}
	hostFD.Release()
	return fd, nil
}

func (l *Loader) containerCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	containers := 0
	for id := range l.processes {
		if id.pid == 0 {
			// pid==0 represents the init process of a container. There is
			// only one of such process per container.
			containers++
		}
	}
	return containers
}
