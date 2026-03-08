package config

import (
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

var (
	watcher     *fsnotify.Watcher
	watcherOnce sync.Once
	debounceMu  sync.Mutex
	debounceTimer *time.Timer
)

func StartWatcher() error {
	var err error
	watcherOnce.Do(func() {
		watcher, err = fsnotify.NewWatcher()
		if err != nil {
			return
		}

		err = watcher.Add(configPath)
		if err != nil {
			watcher.Close()
			watcher = nil
			return
		}

		go watchLoop()
	})
	return err
}

func watchLoop() {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				handleFileChange()
			}
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
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
