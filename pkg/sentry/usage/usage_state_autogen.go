// automatically generated by stateify.

package usage

import (
	"github.com/talismancer/gvisor-ligolo/pkg/state"
)

func (s *CPUStats) StateTypeName() string {
	return "pkg/sentry/usage.CPUStats"
}

func (s *CPUStats) StateFields() []string {
	return []string{
		"UserTime",
		"SysTime",
		"VoluntarySwitches",
	}
}

func (s *CPUStats) beforeSave() {}

// +checklocksignore
func (s *CPUStats) StateSave(stateSinkObject state.Sink) {
	s.beforeSave()
	stateSinkObject.Save(0, &s.UserTime)
	stateSinkObject.Save(1, &s.SysTime)
	stateSinkObject.Save(2, &s.VoluntarySwitches)
}

func (s *CPUStats) afterLoad() {}

// +checklocksignore
func (s *CPUStats) StateLoad(stateSourceObject state.Source) {
	stateSourceObject.Load(0, &s.UserTime)
	stateSourceObject.Load(1, &s.SysTime)
	stateSourceObject.Load(2, &s.VoluntarySwitches)
}

func (i *IO) StateTypeName() string {
	return "pkg/sentry/usage.IO"
}

func (i *IO) StateFields() []string {
	return []string{
		"CharsRead",
		"CharsWritten",
		"ReadSyscalls",
		"WriteSyscalls",
		"BytesRead",
		"BytesWritten",
		"BytesWriteCancelled",
	}
}

func (i *IO) beforeSave() {}

// +checklocksignore
func (i *IO) StateSave(stateSinkObject state.Sink) {
	i.beforeSave()
	stateSinkObject.Save(0, &i.CharsRead)
	stateSinkObject.Save(1, &i.CharsWritten)
	stateSinkObject.Save(2, &i.ReadSyscalls)
	stateSinkObject.Save(3, &i.WriteSyscalls)
	stateSinkObject.Save(4, &i.BytesRead)
	stateSinkObject.Save(5, &i.BytesWritten)
	stateSinkObject.Save(6, &i.BytesWriteCancelled)
}

func (i *IO) afterLoad() {}

// +checklocksignore
func (i *IO) StateLoad(stateSourceObject state.Source) {
	stateSourceObject.Load(0, &i.CharsRead)
	stateSourceObject.Load(1, &i.CharsWritten)
	stateSourceObject.Load(2, &i.ReadSyscalls)
	stateSourceObject.Load(3, &i.WriteSyscalls)
	stateSourceObject.Load(4, &i.BytesRead)
	stateSourceObject.Load(5, &i.BytesWritten)
	stateSourceObject.Load(6, &i.BytesWriteCancelled)
}

func init() {
	state.Register((*CPUStats)(nil))
	state.Register((*IO)(nil))
}
