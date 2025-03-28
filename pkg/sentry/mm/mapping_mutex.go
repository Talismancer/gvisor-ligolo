package mm

import (
	"reflect"

	"github.com/talismancer/gvisor-ligolo/pkg/sync"
	"github.com/talismancer/gvisor-ligolo/pkg/sync/locking"
)

// RWMutex is sync.RWMutex with the correctness validator.
type mappingRWMutex struct {
	mu sync.RWMutex
}

// lockNames is a list of user-friendly lock names.
// Populated in init.
var mappinglockNames []string

// lockNameIndex is used as an index passed to NestedLock and NestedUnlock,
// refering to an index within lockNames.
// Values are specified using the "consts" field of go_template_instance.
type mappinglockNameIndex int

// DO NOT REMOVE: The following function automatically replaced with lock index constants.
// LOCK_NAME_INDEX_CONSTANTS
const ()

// Lock locks m.
// +checklocksignore
func (m *mappingRWMutex) Lock() {
	locking.AddGLock(mappingprefixIndex, -1)
	m.mu.Lock()
}

// NestedLock locks m knowing that another lock of the same type is held.
// +checklocksignore
func (m *mappingRWMutex) NestedLock(i mappinglockNameIndex) {
	locking.AddGLock(mappingprefixIndex, int(i))
	m.mu.Lock()
}

// Unlock unlocks m.
// +checklocksignore
func (m *mappingRWMutex) Unlock() {
	m.mu.Unlock()
	locking.DelGLock(mappingprefixIndex, -1)
}

// NestedUnlock unlocks m knowing that another lock of the same type is held.
// +checklocksignore
func (m *mappingRWMutex) NestedUnlock(i mappinglockNameIndex) {
	m.mu.Unlock()
	locking.DelGLock(mappingprefixIndex, int(i))
}

// RLock locks m for reading.
// +checklocksignore
func (m *mappingRWMutex) RLock() {
	locking.AddGLock(mappingprefixIndex, -1)
	m.mu.RLock()
}

// RUnlock undoes a single RLock call.
// +checklocksignore
func (m *mappingRWMutex) RUnlock() {
	m.mu.RUnlock()
	locking.DelGLock(mappingprefixIndex, -1)
}

// RLockBypass locks m for reading without executing the validator.
// +checklocksignore
func (m *mappingRWMutex) RLockBypass() {
	m.mu.RLock()
}

// RUnlockBypass undoes a single RLockBypass call.
// +checklocksignore
func (m *mappingRWMutex) RUnlockBypass() {
	m.mu.RUnlock()
}

// DowngradeLock atomically unlocks rw for writing and locks it for reading.
// +checklocksignore
func (m *mappingRWMutex) DowngradeLock() {
	m.mu.DowngradeLock()
}

var mappingprefixIndex *locking.MutexClass

// DO NOT REMOVE: The following function is automatically replaced.
func mappinginitLockNames() {}

func init() {
	mappinginitLockNames()
	mappingprefixIndex = locking.NewMutexClass(reflect.TypeOf(mappingRWMutex{}), mappinglockNames)
}
