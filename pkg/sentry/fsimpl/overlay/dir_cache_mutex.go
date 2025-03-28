package overlay

import (
	"reflect"

	"github.com/talismancer/gvisor-ligolo/pkg/sync"
	"github.com/talismancer/gvisor-ligolo/pkg/sync/locking"
)

// Mutex is sync.Mutex with the correctness validator.
type dirInoCacheMutex struct {
	mu sync.Mutex
}

var dirInoCacheprefixIndex *locking.MutexClass

// lockNames is a list of user-friendly lock names.
// Populated in init.
var dirInoCachelockNames []string

// lockNameIndex is used as an index passed to NestedLock and NestedUnlock,
// refering to an index within lockNames.
// Values are specified using the "consts" field of go_template_instance.
type dirInoCachelockNameIndex int

// DO NOT REMOVE: The following function automatically replaced with lock index constants.
// LOCK_NAME_INDEX_CONSTANTS
const ()

// Lock locks m.
// +checklocksignore
func (m *dirInoCacheMutex) Lock() {
	locking.AddGLock(dirInoCacheprefixIndex, -1)
	m.mu.Lock()
}

// NestedLock locks m knowing that another lock of the same type is held.
// +checklocksignore
func (m *dirInoCacheMutex) NestedLock(i dirInoCachelockNameIndex) {
	locking.AddGLock(dirInoCacheprefixIndex, int(i))
	m.mu.Lock()
}

// Unlock unlocks m.
// +checklocksignore
func (m *dirInoCacheMutex) Unlock() {
	locking.DelGLock(dirInoCacheprefixIndex, -1)
	m.mu.Unlock()
}

// NestedUnlock unlocks m knowing that another lock of the same type is held.
// +checklocksignore
func (m *dirInoCacheMutex) NestedUnlock(i dirInoCachelockNameIndex) {
	locking.DelGLock(dirInoCacheprefixIndex, int(i))
	m.mu.Unlock()
}

// DO NOT REMOVE: The following function is automatically replaced.
func dirInoCacheinitLockNames() {}

func init() {
	dirInoCacheinitLockNames()
	dirInoCacheprefixIndex = locking.NewMutexClass(reflect.TypeOf(dirInoCacheMutex{}), dirInoCachelockNames)
}
