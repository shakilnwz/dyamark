package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher watches a file on disk for external changes and triggers a reload.
type FileWatcher struct {
	watcher  *fsnotify.Watcher
	filePath string
	onReload func([]byte)

	mu    sync.Mutex
	timer *time.Timer
}

// WatchFile creates a watcher for filePath that calls onReload when the file changes.
func WatchFile(filePath string, onReload func([]byte)) (*FileWatcher, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(absPath)
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, fmt.Errorf("watching dir %s: %w", dir, err)
	}

	fw := &FileWatcher{
		watcher:  w,
		filePath: absPath,
		onReload: onReload,
	}
	go fw.run()
	return fw, nil
}

// Close stops the watcher.
func (fw *FileWatcher) Close() error {
	fw.mu.Lock()
	if fw.timer != nil {
		fw.timer.Stop()
	}
	fw.mu.Unlock()
	return fw.watcher.Close()
}

func (fw *FileWatcher) run() {
	targetBase := filepath.Base(fw.filePath)
	for {
		select {
		case ev, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			if filepath.Base(ev.Name) == targetBase && (ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create)) {
				fw.scheduleReload()
			}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			fmt.Fprintf(os.Stderr, "dyamark: file watcher: %v\n", err)
		}
	}
}

func (fw *FileWatcher) scheduleReload() {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.timer != nil {
		fw.timer.Stop()
	}
	fw.timer = time.AfterFunc(60*time.Millisecond, func() {
		data, err := os.ReadFile(fw.filePath)
		if err == nil && fw.onReload != nil {
			fw.onReload(data)
		}
	})
}
