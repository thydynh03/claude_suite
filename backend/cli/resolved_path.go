package cli

import "sync"

// resolvedPath holds a CLI executable path that may be re-resolved at run time.
//
// The runners are single shared instances and the orchestrator dispatches every
// task in its own goroutine, so the "cached answer was the bare fallback name,
// try resolving again" path had several goroutines reading and writing the same
// string field at once — a data race the Linux race-detector job would catch,
// and a torn read of a string header is not a theoretical concern.
type resolvedPath struct {
	mu   sync.Mutex
	path string
}

func newResolvedPath(p string) *resolvedPath {
	return &resolvedPath{path: p}
}

func (r *resolvedPath) get() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.path
}

// ensure returns a usable path, re-resolving once when what is cached is only
// the bare fallback name. resolve is called under the lock, so concurrent
// starters do the work at most once each rather than racing on the field.
func (r *resolvedPath) ensure(resolve func() string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if CLIInstalled(r.path) {
		return r.path, true
	}
	if fresh := resolve(); CLIInstalled(fresh) {
		r.path = fresh
		return fresh, true
	}
	return r.path, false
}
