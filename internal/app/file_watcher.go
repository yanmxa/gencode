package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yanmxa/gencode/internal/hooks"
)

const defaultFileWatcherInterval = 500 * time.Millisecond

type fileWatcher struct {
	engine    *hooks.Engine
	onOutcome func(hooks.HookOutcome)
	interval  time.Duration

	mu      sync.Mutex
	started bool
	stopCh  chan struct{}
	paths   map[string]fileSnapshot
}

type fileSnapshot struct {
	exists  bool
	size    int64
	modTime time.Time
}

func newFileWatcher(engine *hooks.Engine, onOutcome func(hooks.HookOutcome)) *fileWatcher {
	return &fileWatcher{
		engine:    engine,
		onOutcome: onOutcome,
		interval:  defaultFileWatcherInterval,
		stopCh:    make(chan struct{}),
		paths:     make(map[string]fileSnapshot),
	}
}

func (w *fileWatcher) SetPaths(paths []string) {
	if w == nil {
		return
	}

	normalized := make(map[string]fileSnapshot)
	for _, path := range paths {
		if !filepath.IsAbs(path) || path == "" {
			continue
		}
		clean := filepath.Clean(path)
		normalized[clean] = snapshotFile(clean)
	}

	w.mu.Lock()
	w.paths = normalized
	shouldStart := len(normalized) > 0 && !w.started
	if shouldStart {
		w.started = true
	}
	w.mu.Unlock()

	if shouldStart {
		go w.loop()
	}
}

func (w *fileWatcher) CurrentPaths() []string {
	if w == nil {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	paths := make([]string, 0, len(w.paths))
	for path := range w.paths {
		paths = append(paths, path)
	}
	return paths
}

func (w *fileWatcher) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return
	}
	stopCh := w.stopCh
	w.started = false
	w.stopCh = make(chan struct{})
	w.mu.Unlock()
	close(stopCh)
}

func (w *fileWatcher) loop() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.poll()
		case <-w.stopCh:
			return
		}
	}
}

func (w *fileWatcher) poll() {
	type change struct {
		path  string
		event string
		next  fileSnapshot
	}

	w.mu.Lock()
	paths := make(map[string]fileSnapshot, len(w.paths))
	for path, snap := range w.paths {
		paths[path] = snap
	}
	w.mu.Unlock()

	var changes []change
	for path, prev := range paths {
		next := snapshotFile(path)
		if event, ok := detectFileEvent(prev, next); ok {
			changes = append(changes, change{path: path, event: event, next: next})
		}
	}

	if len(changes) == 0 {
		return
	}

	w.mu.Lock()
	for _, change := range changes {
		if _, ok := w.paths[change.path]; ok {
			w.paths[change.path] = change.next
		}
	}
	w.mu.Unlock()

	for _, change := range changes {
		if w.engine == nil {
			return
		}
		outcome := w.engine.Execute(context.Background(), hooks.FileChanged, hooks.HookInput{
			FilePath: change.path,
			Event:    change.event,
		})
		if len(outcome.WatchPaths) > 0 {
			w.SetPaths(outcome.WatchPaths)
		}
		if w.onOutcome != nil {
			w.onOutcome(outcome)
		}
	}
}

func snapshotFile(path string) fileSnapshot {
	info, err := os.Stat(path)
	if err != nil {
		return fileSnapshot{}
	}
	return fileSnapshot{
		exists:  true,
		size:    info.Size(),
		modTime: info.ModTime(),
	}
}

func detectFileEvent(prev, next fileSnapshot) (string, bool) {
	switch {
	case !prev.exists && next.exists:
		return "add", true
	case prev.exists && !next.exists:
		return "unlink", true
	case prev.exists && next.exists && (prev.size != next.size || !prev.modTime.Equal(next.modTime)):
		return "change", true
	default:
		return "", false
	}
}
