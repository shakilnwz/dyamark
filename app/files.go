package app

import (
	"errors"
	"fmt"
	"io/fs"
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

// SaveFile writes data to path atomically: into path.dyamark-tmp beside
// it, synced to disk, then renamed over path. A reader, or a crash, sees
// the old file or the new one, never half of it. An existing file keeps
// its mode, and a symlink keeps pointing at the file it names.
func SaveFile(path string, data []byte) (err error) {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	tmp := path + ".dyamark-tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(tmp)
		}
	}()
	_, err = f.Write(data)
	if err == nil {
		// The umask may have narrowed the mode the file was made with.
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	// Sync the directory too, so the rename itself survives a crash.
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		dir.Sync()
		dir.Close()
	}
	return nil
}
