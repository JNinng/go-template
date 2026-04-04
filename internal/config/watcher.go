package config

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

var (
	watcher       *fsnotify.Watcher
	watcherOnce   sync.Once
	debounceMu    sync.Mutex
	debounceTimer *time.Timer
	stopCtx       context.Context
	stopCancel    context.CancelFunc
)

// StartWatcher 启动监控
// 返回 context.CancelFunc 用于优雅关闭 watcher
func StartWatcher() (context.CancelFunc, error) {
	var err error
	watcherOnce.Do(func() {
		stopCtx, stopCancel = context.WithCancel(context.Background())

		watcher, err = fsnotify.NewWatcher()
		if err != nil {
			return
		}

		configDir := filepath.Dir(configPath)
		if err = watcher.Add(configDir); err != nil {
			watcher.Close()
			watcher = nil
			return
		}

		go watchLoop()
	})
	return stopCancel, err
}

func watchLoop() {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			if filepath.Base(event.Name) != filepath.Base(configPath) {
				continue
			}

			// 过滤掉 Chmod 事件，通常只关心 Write 和 Create
			// 注意：Vim 等编辑器保存会触发 Create/Rename
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
				handleFileChange()
			}

		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}

		case <-stopCtx.Done():
			return
		}
	}
}

func handleFileChange() {
	debounceMu.Lock()
	defer debounceMu.Unlock()

	if debounceTimer != nil {
		debounceTimer.Stop()
	}

	debounceTimer = time.AfterFunc(300*time.Millisecond, func() {
		updateConfig()
	})
}

// StopWatcher 资源清理，优雅关闭 watcher
func StopWatcher() {
	if stopCancel != nil {
		stopCancel()
	}

	debounceMu.Lock()
	if debounceTimer != nil {
		debounceTimer.Stop()
		debounceTimer = nil
	}
	debounceMu.Unlock()

	if watcher != nil {
		watcher.Close()
		watcher = nil
	}
}
