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

package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/talismancer/gvisor-ligolo/pkg/log"
	"github.com/talismancer/gvisor-ligolo/pkg/refs"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/watchdog"
	"github.com/talismancer/gvisor-ligolo/runsc/flag"
)

// RegisterFlags registers flags used to populate Config.
func RegisterFlags(flagSet *flag.FlagSet) {
	// Although these flags are not part of the OCI spec, they are used by
	// Docker, and thus should not be changed.
	flagSet.String("root", "", "root directory for storage of container state.")
	flagSet.String("log", "", "file path where internal debug information is written, default is stdout.")
	flagSet.String("log-format", "text", "log format: text (default), json, or json-k8s.")
	flagSet.Bool("debug", false, "enable debug logging.")
	flagSet.Bool("systemd-cgroup", false, "EXPERIMENTAL. Use systemd for cgroups.")

	// These flags are unique to runsc, and are used to configure parts of the
	// system that are not covered by the runtime spec.

	// Debugging flags.
	flagSet.String("debug-log", "", "additional location for logs. If it ends with '/', log files are created inside the directory with default names. The following variables are available: %TIMESTAMP%, %COMMAND%.")
	flagSet.String("debug-command", "", `comma-separated list of commands to be debugged if --debug-log is also set. Empty means debug all. "!" negates the expression. E.g. "create,start" or "!boot,events"`)
	flagSet.String("panic-log", "", "file path where panic reports and other Go's runtime messages are written.")
	flagSet.String("coverage-report", "", "file path where Go coverage reports are written. Reports will only be generated if runsc is built with --collect_code_coverage and --instrumentation_filter Bazel flags.")
	flagSet.Bool("log-packets", false, "enable network packet logging.")
	flagSet.String("pcap-log", "", "location of PCAP log file.")
	flagSet.String("debug-log-format", "text", "log format: text (default), json, or json-k8s.")
	// Only register -alsologtostderr flag if it is not already defined on this flagSet.
	if flagSet.Lookup("alsologtostderr") == nil {
		flagSet.Bool("alsologtostderr", false, "send log messages to stderr.")
	}
	flagSet.Bool("allow-flag-override", false, "allow OCI annotations (dev.gvisor.flag.<name>) to override flags for debugging.")
	flagSet.String("traceback", "system", "golang runtime's traceback level")

	// Metrics flags.
	flagSet.String("metric-server", "", "if set, export metrics on this address. This may either be 1) 'addr:port' to export metrics on a specific network interface address, 2) ':port' for exporting metrics on all interfaces, or 3) an absolute path to a Unix Domain Socket. The substring '%ID%' will be replaced by the container ID, and '%RUNTIME_ROOT%' by the root. This flag must be specified in both `runsc metric-server` and `runsc create`, and their values must match.")

	// Debugging flags: strace related
	flagSet.Bool("strace", false, "enable strace.")
	flagSet.String("strace-syscalls", "", "comma-separated list of syscalls to trace. If --strace is true and this list is empty, then all syscalls will be traced.")
	flagSet.Uint("strace-log-size", 1024, "default size (in bytes) to log data argument blobs.")
	flagSet.Bool("strace-event", false, "send strace to event.")

	// Flags that control sandbox runtime behavior.
	flagSet.String("platform", "systrap", "specifies which platform to use: systrap (default), ptrace, kvm.")
	flagSet.String("platform_device_path", "", "path to a platform-specific device file (e.g. /dev/kvm for KVM platform). If unset, will use a sane platform-specific default.")
	flagSet.Var(watchdogActionPtr(watchdog.LogWarning), "watchdog-action", "sets what action the watchdog takes when triggered: log (default), panic.")
	flagSet.Int("panic-signal", -1, "register signal handling that panics. Usually set to SIGUSR2(12) to troubleshoot hangs. -1 disables it.")
	flagSet.Bool("profile", false, "prepares the sandbox to use Golang profiler. Note that enabling profiler loosens the seccomp protection added to the sandbox (DO NOT USE IN PRODUCTION).")
	flagSet.String("profile-block", "", "collects a block profile to this file path for the duration of the container execution. Requires -profile=true.")
	flagSet.String("profile-cpu", "", "collects a CPU profile to this file path for the duration of the container execution. Requires -profile=true.")
	flagSet.String("profile-heap", "", "collects a heap profile to this file path for the duration of the container execution. Requires -profile=true.")
	flagSet.String("profile-mutex", "", "collects a mutex profile to this file path for the duration of the container execution. Requires -profile=true.")
	flagSet.String("trace", "", "collects a Go runtime execution trace to this file path for the duration of the container execution.")
	flagSet.Bool("rootless", false, "it allows the sandbox to be started with a user that is not root. Sandbox and Gofer processes may run with same privileges as current user.")
	flagSet.Var(leakModePtr(refs.NoLeakChecking), "ref-leak-mode", "sets reference leak check mode: disabled (default), log-names, log-traces.")
	flagSet.Bool("cpu-num-from-quota", false, "set cpu number to cpu quota (least integer greater or equal to quota value, but not less than 2)")
	flagSet.Bool("oci-seccomp", false, "Enables loading OCI seccomp filters inside the sandbox.")
	flagSet.Bool("enable-core-tags", false, "enables core tagging. Requires host linux kernel >= 5.14.")
	flagSet.String("pod-init-config", "", "path to configuration file with additional steps to take during pod creation.")

	// Flags that control sandbox runtime behavior: FS related.
	flagSet.Var(fileAccessTypePtr(FileAccessExclusive), "file-access", "specifies which filesystem validation to use for the root mount: exclusive (default), shared.")
	flagSet.Var(fileAccessTypePtr(FileAccessShared), "file-access-mounts", "specifies which filesystem validation to use for volumes other than the root mount: shared (default), exclusive.")
	flagSet.Bool("overlay", false, "DEPRECATED: use --overlay2=all:memory to achieve the same effect")
	flagSet.Var(defaultOverlay2(), "overlay2", "wrap mounts with overlayfs. Format is {mount}:{medium}, where 'mount' can be 'root' or 'all' and medium can be 'memory', 'self' or 'dir=/abs/dir/path' in which filestore will be created. 'none' will turn overlay mode off.")
	flagSet.Bool("fsgofer-host-uds", false, "DEPRECATED: use host-uds=all")
	flagSet.Var(hostUDSPtr(HostUDSNone), "host-uds", "controls permission to access host Unix-domain sockets. Values: none|open|create|all, default: none")
	flagSet.Var(hostFifoPtr(HostFifoNone), "host-fifo", "controls permission to access host FIFOs (or named pipes). Values: none|open, default: none")

	flagSet.Bool("vfs2", true, "DEPRECATED: this flag has no effect.")
	flagSet.Bool("fuse", true, "DEPRECATED: this flag has no effect.")
	flagSet.Bool("lisafs", true, "DEPRECATED: this flag has no effect.")
	flagSet.Bool("cgroupfs", false, "Automatically mount cgroupfs.")
	flagSet.Bool("ignore-cgroups", false, "don't configure cgroups.")
	flagSet.Int("fdlimit", -1, "Specifies a limit on the number of host file descriptors that can be open. Applies separately to the sentry and gofer. Note: each file in the sandbox holds more than one host FD open.")
	flagSet.Int("dcache", -1, "Set the global dentry cache size. This acts as a coarse-grained control on the number of host FDs simultaneously open by the sentry. If negative, per-mount caches are used.")
	flagSet.Bool("iouring", false, "TEST ONLY; Enables io_uring syscalls in the sentry. Support is experimental and very limited.")
	flagSet.Bool("directfs", true, "directly access the container filesystems from the sentry. Sentry runs with higher privileges.")

	// Flags that control sandbox runtime behavior: network related.
	flagSet.Var(networkTypePtr(NetworkSandbox), "network", "specifies which network to use: sandbox (default), host, none. Using network inside the sandbox is more secure because it's isolated from the host network.")
	flagSet.Bool("net-raw", false, "enable raw sockets. When false, raw sockets are disabled by removing CAP_NET_RAW from containers (`runsc exec` will still be able to utilize raw sockets). Raw sockets allow malicious containers to craft packets and potentially attack the network.")
	flagSet.Bool("gso", true, "enable host segmentation offload if it is supported by a network device.")
	flagSet.Bool("software-gso", true, "enable gVisor segmentation offload when host offload can't be enabled.")
	flagSet.Duration("gvisor-gro", 0, "(e.g. \"20000ns\" or \"1ms\") sets gVisor's generic receive offload timeout. Zero bypasses GRO.")
	flagSet.Bool("tx-checksum-offload", false, "enable TX checksum offload.")
	flagSet.Bool("rx-checksum-offload", true, "enable RX checksum offload.")
	flagSet.Var(queueingDisciplinePtr(QDiscFIFO), "qdisc", "specifies which queueing discipline to apply by default to the non loopback nics used by the sandbox.")
	flagSet.Int("num-network-channels", 1, "number of underlying channels(FDs) to use for network link endpoints.")
	flagSet.Bool("buffer-pooling", true, "enable allocation of buffers from a shared pool instead of the heap.")
	flagSet.Bool("EXPERIMENTAL-afxdp", false, "EXPERIMENTAL. Use an AF_XDP socket to receive packets.")

	// Flags that control sandbox runtime behavior: accelerator related.
	flagSet.Bool("nvproxy", false, "EXPERIMENTAL: enable support for Nvidia GPUs")
	flagSet.Bool("nvproxy-docker", false, "Expose GPUs to containers based on NVIDIA_VISIBLE_DEVICES, as requested by the container or set by `docker --gpus`. Allows containers to self-serve GPU access and thus disabled by default for security. libnvidia-container must be installed on the host. No effect unless --nvproxy is enabled.")
	flagSet.Bool("tpuproxy", false, "EXPERIMENTAL: enable support for TPU device passthrough.")

	// Test flags, not to be used outside tests, ever.
	flagSet.Bool("TESTONLY-unsafe-nonroot", false, "TEST ONLY; do not ever use! This skips many security measures that isolate the host from the sandbox.")
	flagSet.String("TESTONLY-test-name-env", "", "TEST ONLY; do not ever use! Used for automated tests to improve logging.")
	flagSet.Bool("TESTONLY-allow-packet-endpoint-write", false, "TEST ONLY; do not ever use! Used for tests to allow writes on packet sockets.")
	flagSet.Bool("TESTONLY-afs-syscall-panic", false, "TEST ONLY; do not ever use! Used for tests exercising gVisor panic reporting.")
}

// overrideAllowlist lists all flags that can be changed using OCI
// annotations without an administrator setting `--allow-flag-override` on the
// runtime. Flags in this list can be set by container authors and should not
// make the sandbox less secure.
var overrideAllowlist = map[string]struct {
	check func(name string, value string) error
}{
	"debug":           {},
	"strace":          {},
	"strace-syscalls": {},
	"strace-log-size": {},
	"host-uds":        {},

	"oci-seccomp": {check: checkOciSeccomp},
}

// checkOciSeccomp ensures that seccomp can be enabled but not disabled.
func checkOciSeccomp(name string, value string) error {
	enable, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	if !enable {
		return fmt.Errorf("disabling %q requires flag %q to be enabled", name, "allow-flag-override")
	}
	return nil
}

// isFlagExplicitlySet returns whether the given flag name is explicitly set.
// Doesn't check for flag existence; returns `false` for flags that don't exist.
func isFlagExplicitlySet(flagSet *flag.FlagSet, name string) bool {
	explicit := false

	// The FlagSet.Visit function only visits flags that are explicitly set, as opposed to VisitAll.
	flagSet.Visit(func(fl *flag.Flag) {
		explicit = explicit || fl.Name == name
	})

	return explicit
}

// NewFromFlags creates a new Config with values coming from command line flags.
func NewFromFlags(flagSet *flag.FlagSet) (*Config, error) {
	conf := &Config{explicitlySet: map[string]struct{}{}}

	obj := reflect.ValueOf(conf).Elem()
	st := obj.Type()
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		name, ok := f.Tag.Lookup("flag")
		if !ok {
			// No flag set for this field.
			continue
		}
		fl := flagSet.Lookup(name)
		if fl == nil {
			panic(fmt.Sprintf("Flag %q not found", name))
		}
		x := reflect.ValueOf(flag.Get(fl.Value))
		obj.Field(i).Set(x)
		if isFlagExplicitlySet(flagSet, name) {
			conf.explicitlySet[name] = struct{}{}
		}
	}

	if len(conf.RootDir) == 0 {
		// If not set, set default root dir to something (hopefully) user-writeable.
		conf.RootDir = "/var/run/runsc"
		// NOTE: empty values for XDG_RUNTIME_DIR should be ignored.
		if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
			conf.RootDir = filepath.Join(runtimeDir, "runsc")
		}
	}

	if err := conf.validate(); err != nil {
		return nil, err
	}
	return conf, nil
}

// NewFromBundle makes a new config from a Bundle.
func NewFromBundle(bundle Bundle) (*Config, error) {
	if err := bundle.Validate(); err != nil {
		return nil, err
	}
	flagSet := flag.NewFlagSet("tmp", flag.ContinueOnError)
	RegisterFlags(flagSet)
	conf := &Config{explicitlySet: map[string]struct{}{}}

	obj := reflect.ValueOf(conf).Elem()
	st := obj.Type()
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		name, ok := f.Tag.Lookup("flag")
		if !ok {
			continue
		}
		fl := flagSet.Lookup(name)
		if fl == nil {
			return nil, fmt.Errorf("flag %q not found", name)
		}
		val, ok := bundle[name]
		if !ok {
			continue
		}
		if err := flagSet.Set(name, val); err != nil {
			return nil, fmt.Errorf("error setting flag %s=%q: %w", name, val, err)
		}
		conf.Override(flagSet, name, val, true)

		conf.explicitlySet[name] = struct{}{}
	}
	return conf, nil
}

// ToFlags returns a slice of flags that correspond to the given Config.
func (c *Config) ToFlags() []string {
	flagSet := flag.NewFlagSet("tmp", flag.ContinueOnError)
	RegisterFlags(flagSet)

	var rv []string
	keyVals := c.keyVals(flagSet, false /*onlyIfSet*/)
	for name, val := range keyVals {
		rv = append(rv, fmt.Sprintf("--%s=%s", name, val))
	}

	// Construct a temporary set for default plumbing.
	return rv
}

// KeyVal is a key value pair. It is used so ToContainerdConfigTOML returns
// predictable ordering for runsc flags.
type KeyVal struct {
	Key string
	Val string
}

// ContainerdConfigOptions contains arguments for ToContainerdConfigTOML.
type ContainerdConfigOptions struct {
	BinaryPath string
	RootPath   string
	Options    map[string]string
	RunscFlags []KeyVal
}

// ToContainerdConfigTOML turns a given config into a format for a k8s containerd config.toml file.
// See: https://gvisor.dev/docs/user_guide/containerd/quick_start/
func (c *Config) ToContainerdConfigTOML(opts ContainerdConfigOptions) (string, error) {
	flagSet := flag.NewFlagSet("tmp", flag.ContinueOnError)
	RegisterFlags(flagSet)
	keyVals := c.keyVals(flagSet, true /*onlyIfSet*/)
	keys := []string{}
	for k := range keyVals {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		opts.RunscFlags = append(opts.RunscFlags, KeyVal{k, keyVals[k]})
	}

	const temp = `{{if .BinaryPath}}binary_name = "{{.BinaryPath}}"{{end}}
{{if .RootPath}}root = "{{.RootPath}}"{{end}}
{{if .Options}}{{ range $key, $value := .Options}}{{$key}} = "{{$value}}"
{{end}}{{end}}{{if .RunscFlags}}[runsc_config]
{{ range $fl:= .RunscFlags}}  {{$fl.Key}} = "{{$fl.Val}}"
{{end}}{{end}}`

	t := template.New("temp")
	t, err := t.Parse(temp)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, opts); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (c *Config) keyVals(flagSet *flag.FlagSet, onlyIfSet bool) map[string]string {
	keyVals := make(map[string]string)

	obj := reflect.ValueOf(c).Elem()
	st := obj.Type()
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		name, ok := f.Tag.Lookup("flag")
		if !ok {
			// No flag set for this field.
			continue
		}
		val := getVal(obj.Field(i))

		fl := flagSet.Lookup(name)
		if fl == nil {
			panic(fmt.Sprintf("Flag %q not found", name))
		}
		if val == fl.DefValue || onlyIfSet {
			// If this config wasn't populated from a FlagSet, don't plumb through default flags.
			if c.explicitlySet == nil {
				continue
			}
			// If this config was populated from a FlagSet, plumb through only default flags which were
			// explicitly specified.
			if _, explicit := c.explicitlySet[name]; !explicit {
				continue
			}
		}
		keyVals[fl.Name] = val
	}
	return keyVals
}

// Override writes a new value to a flag.
func (c *Config) Override(flagSet *flag.FlagSet, name string, value string, force bool) error {
	obj := reflect.ValueOf(c).Elem()
	st := obj.Type()
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		fieldName, ok := f.Tag.Lookup("flag")
		if !ok || fieldName != name {
			// Not a flag field, or flag name doesn't match.
			continue
		}
		fl := flagSet.Lookup(name)
		if fl == nil {
			// Flag must exist if there is a field match above.
			panic(fmt.Sprintf("Flag %q not found", name))
		}
		if !force {
			if err := c.isOverrideAllowed(name, value); err != nil {
				return fmt.Errorf("error setting flag %s=%q: %w", name, value, err)
			}
		}

		// Use flag to convert the string value to the underlying flag type, using
		// the same rules as the command-line for consistency.
		if err := fl.Value.Set(value); err != nil {
			return fmt.Errorf("error setting flag %s=%q: %w", name, value, err)
		}
		x := reflect.ValueOf(flag.Get(fl.Value))
		obj.Field(i).Set(x)

		// Validates the config again to ensure it's left in a consistent state.
		return c.validate()
	}
	return fmt.Errorf("flag %q not found. Cannot set it to %q", name, value)
}

func (c *Config) isOverrideAllowed(name string, value string) error {
	if c.AllowFlagOverride {
		return nil
	}
	// If the global override flag is not enabled, check if the individual flag is
	// safe to apply.
	if allow, ok := overrideAllowlist[name]; ok {
		if allow.check != nil {
			if err := allow.check(name, value); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("flag override disabled, use --allow-flag-override to enable it")
}

// ApplyBundles applies the given bundles by name.
// It returns an error if a bundle doesn't exist, or if the given
// bundles have conflicting flag values.
// Config values which are already specified prior to calling ApplyBundles are overridden.
func (c *Config) ApplyBundles(flagSet *flag.FlagSet, bundleNames ...BundleName) error {
	// Populate a map from flag name to flag value to bundle name.
	flagToValueToBundleName := make(map[string]map[string]BundleName)
	for _, bundleName := range bundleNames {
		b := Bundles[bundleName]
		if b == nil {
			return fmt.Errorf("no such bundle: %q", bundleName)
		}
		for flagName, val := range b {
			valueToBundleName := flagToValueToBundleName[flagName]
			if valueToBundleName == nil {
				valueToBundleName = make(map[string]BundleName)
				flagToValueToBundleName[flagName] = valueToBundleName
			}
			valueToBundleName[val] = bundleName
		}
	}
	// Check for conflicting flag values between the bundles.
	for flagName, valueToBundleName := range flagToValueToBundleName {
		if len(valueToBundleName) == 1 {
			continue
		}
		bundleNameToValue := make(map[string]string)
		for val, bundleName := range valueToBundleName {
			bundleNameToValue[string(bundleName)] = val
		}
		var sb strings.Builder
		first := true
		for _, bundleName := range bundleNames {
			if val, ok := bundleNameToValue[string(bundleName)]; ok {
				if !first {
					sb.WriteString(", ")
				}
				sb.WriteString(fmt.Sprintf("bundle %q sets --%s=%q", bundleName, flagName, val))
				first = false
			}
		}
		return fmt.Errorf("flag --%s is specified by multiple bundles: %s", flagName, sb.String())
	}

	// Actually apply flag values.
	for flagName, valueToBundleName := range flagToValueToBundleName {
		fl := flagSet.Lookup(flagName)
		if fl == nil {
			return fmt.Errorf("flag --%s not found", flagName)
		}
		prevValue := fl.Value.String()
		// Note: We verified earlier that valueToBundleName has length 1,
		// so this loop executes exactly once per flag.
		for val, bundleName := range valueToBundleName {
			if prevValue == val {
				continue
			}
			if isFlagExplicitlySet(flagSet, flagName) {
				log.Infof("Flag --%s has explicitly-set value %q, but bundle %s takes precedence and is overriding its value to --%s=%q.", flagName, prevValue, bundleName, flagName, val)
			} else {
				log.Infof("Overriding flag --%s=%q from applying bundle %s.", flagName, val, bundleName)
			}
			if err := c.Override(flagSet, flagName, val /* force= */, true); err != nil {
				return err
			}
		}
	}

	return c.validate()
}

func getVal(field reflect.Value) string {
	if str, ok := field.Addr().Interface().(fmt.Stringer); ok {
		return str.String()
	}
	switch field.Kind() {
	case reflect.Bool:
		return strconv.FormatBool(field.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(field.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(field.Uint(), 10)
	case reflect.String:
		return field.String()
	default:
		panic("unknown type " + field.Kind().String())
	}
}
