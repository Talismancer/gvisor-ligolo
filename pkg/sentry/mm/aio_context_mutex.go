package mm

import (
	"reflect"

	"github.com/talismancer/gvisor-ligolo/pkg/sync"
	"github.com/talismancer/gvisor-ligolo/pkg/sync/locking"
)

// Mutex is sync.Mutex with the correctness validator.
type aioContextMutex struct {
	mu sync.Mutex
}

var aioContextprefixIndex *locking.MutexClass

// lockNames is a list of user-friendly lock names.
// Populated in init.
var aioContextlockNames []string

// lockNameIndex is used as an index passed to NestedLock and NestedUnlock,
// refering to an index within lockNames.
// Values are specified using the "consts" field of go_template_instance.
type aioContextlockNameIndex int

// DO NOT REMOVE: The following function automatically replaced with lock index constants.
// LOCK_NAME_INDEX_CONSTANTS
const ()

// Lock locks m.
// +checklocksignore
func (m *aioContextMutex) Lock() {
	locking.AddGLock(aioContextprefixIndex, -1)
	m.mu.Lock()
}

// NestedLock locks m knowing that another lock of the same type is held.
// +checklocksignore
func (m *aioContextMutex) NestedLock(i aioContextlockNameIndex) {
	locking.AddGLock(aioContextprefixIndex, int(i))
	m.mu.Lock()
}

// Unlock unlocks m.
// +checklocksignore
func (m *aioContextMutex) Unlock() {
	locking.DelGLock(aioContextprefixIndex, -1)
	m.mu.Unlock()
}

// NestedUnlock unlocks m knowing that another lock of the same type is held.
// +checklocksignore
func (m *aioContextMutex) NestedUnlock(i aioContextlockNameIndex) {
	locking.DelGLock(aioContextprefixIndex, int(i))
	m.mu.Unlock()
}

// DO NOT REMOVE: The following function is automatically replaced.
func aioContextinitLockNames() {}

func init() {
	aioContextinitLockNames()
	aioContextprefixIndex = locking.NewMutexClass(reflect.TypeOf(aioContextMutex{}), aioContextlockNames)
}
