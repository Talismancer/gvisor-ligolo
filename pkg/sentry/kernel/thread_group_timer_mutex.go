package kernel

import (
	"reflect"

	"github.com/talismancer/gvisor-ligolo/pkg/sync"
	"github.com/talismancer/gvisor-ligolo/pkg/sync/locking"
)

// Mutex is sync.Mutex with the correctness validator.
type threadGroupTimerMutex struct {
	mu sync.Mutex
}

var threadGroupTimerprefixIndex *locking.MutexClass

// lockNames is a list of user-friendly lock names.
// Populated in init.
var threadGroupTimerlockNames []string

// lockNameIndex is used as an index passed to NestedLock and NestedUnlock,
// refering to an index within lockNames.
// Values are specified using the "consts" field of go_template_instance.
type threadGroupTimerlockNameIndex int

// DO NOT REMOVE: The following function automatically replaced with lock index constants.
// LOCK_NAME_INDEX_CONSTANTS
const ()

// Lock locks m.
// +checklocksignore
func (m *threadGroupTimerMutex) Lock() {
	locking.AddGLock(threadGroupTimerprefixIndex, -1)
	m.mu.Lock()
}

// NestedLock locks m knowing that another lock of the same type is held.
// +checklocksignore
func (m *threadGroupTimerMutex) NestedLock(i threadGroupTimerlockNameIndex) {
	locking.AddGLock(threadGroupTimerprefixIndex, int(i))
	m.mu.Lock()
}

// Unlock unlocks m.
// +checklocksignore
func (m *threadGroupTimerMutex) Unlock() {
	locking.DelGLock(threadGroupTimerprefixIndex, -1)
	m.mu.Unlock()
}

// NestedUnlock unlocks m knowing that another lock of the same type is held.
// +checklocksignore
func (m *threadGroupTimerMutex) NestedUnlock(i threadGroupTimerlockNameIndex) {
	locking.DelGLock(threadGroupTimerprefixIndex, int(i))
	m.mu.Unlock()
}

// DO NOT REMOVE: The following function is automatically replaced.
func threadGroupTimerinitLockNames() {}

func init() {
	threadGroupTimerinitLockNames()
	threadGroupTimerprefixIndex = locking.NewMutexClass(reflect.TypeOf(threadGroupTimerMutex{}), threadGroupTimerlockNames)
}
