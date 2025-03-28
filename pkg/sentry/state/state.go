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

// Package state provides high-level state wrappers.
package state

import (
	"fmt"
	"io"

	"github.com/talismancer/gvisor-ligolo/pkg/context"
	"github.com/talismancer/gvisor-ligolo/pkg/errors/linuxerr"
	"github.com/talismancer/gvisor-ligolo/pkg/log"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/inet"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/kernel"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/time"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/vfs"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/watchdog"
	"github.com/talismancer/gvisor-ligolo/pkg/state/statefile"
)

var previousMetadata map[string]string

// ErrStateFile is returned when an error is encountered writing the statefile
// (which may occur during open or close calls in addition to write).
type ErrStateFile struct {
	err error
}

// Error implements error.Error().
func (e ErrStateFile) Error() string {
	return fmt.Sprintf("statefile error: %v", e.err)
}

// SaveOpts contains save-related options.
type SaveOpts struct {
	// Destination is the save target.
	Destination io.Writer

	// Key is used for state integrity check.
	Key []byte

	// Metadata is save metadata.
	Metadata map[string]string

	// Callback is called prior to unpause, with any save error.
	Callback func(err error)
}

// Save saves the system state.
func (opts SaveOpts) Save(ctx context.Context, k *kernel.Kernel, w *watchdog.Watchdog) error {
	log.Infof("Sandbox save started, pausing all tasks.")
	k.Pause()
	k.ReceiveTaskStates()
	defer func() {
		k.Unpause()
		log.Infof("Tasks resumed after save.")
	}()

	w.Stop()
	defer w.Start()

	// Supplement the metadata.
	if opts.Metadata == nil {
		opts.Metadata = make(map[string]string)
	}
	addSaveMetadata(opts.Metadata)

	// Open the statefile.
	wc, err := statefile.NewWriter(opts.Destination, opts.Key, opts.Metadata)
	if err != nil {
		err = ErrStateFile{err}
	} else {
		// Save the kernel.
		err = k.SaveTo(ctx, wc)

		// ENOSPC is a state file error. This error can only come from
		// writing the state file, and not from fs.FileOperations.Fsync
		// because we wrap those in kernel.TaskSet.flushWritesToFiles.
		if linuxerr.Equals(linuxerr.ENOSPC, err) {
			err = ErrStateFile{err}
		}

		if closeErr := wc.Close(); err == nil && closeErr != nil {
			err = ErrStateFile{closeErr}
		}
	}
	opts.Callback(err)
	return err
}

// LoadOpts contains load-related options.
type LoadOpts struct {
	// Destination is the load source.
	Source io.Reader

	// Key is used for state integrity check.
	Key []byte
}

// Load loads the given kernel, setting the provided platform and stack.
func (opts LoadOpts) Load(ctx context.Context, k *kernel.Kernel, timeReady chan struct{}, n inet.Stack, clocks time.Clocks, vfsOpts *vfs.CompleteRestoreOptions) error {
	// Open the file.
	r, m, err := statefile.NewReader(opts.Source, opts.Key)
	if err != nil {
		return ErrStateFile{err}
	}

	previousMetadata = m

	// Restore the Kernel object graph.
	return k.LoadFrom(ctx, r, timeReady, n, clocks, vfsOpts)
}
