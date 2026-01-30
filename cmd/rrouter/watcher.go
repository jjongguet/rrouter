package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// ConfigWatcher caches mode and config, reloading on filesystem changes.
// Watches the DIRECTORY ~/.rrouter/ (not individual files) to handle
// macOS inode replacement when files are overwritten.
type ConfigWatcher struct {
	mu      sync.RWMutex
	mode    string
	config  *Config
	watcher *fsnotify.Watcher
	dir     string // ~/.rrouter/
}

func newConfigWatcher(dir string, defaultConfig *Config) *ConfigWatcher {
	cw := &ConfigWatcher{
		dir:    dir,
		config: defaultConfig,
	}

	// Initial read
	cw.mode = cw.readModeFile()
	if cfg := cw.readConfigFile(); cfg != nil {
		cw.config = cfg
	}

	// Start watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[WATCHER] Failed to create fsnotify watcher: %v (falling back to per-request reads)", err)
		return cw
	}
	cw.watcher = watcher

	// Watch the DIRECTORY, not individual files
	// This handles macOS inode replacement (echo "x" > file creates new inode)
	if err := watcher.Add(dir); err != nil {
		log.Printf("[WATCHER] Failed to watch directory %s: %v (falling back to per-request reads)", dir, err)
		watcher.Close()
		cw.watcher = nil
		return cw
	}

	go cw.watchLoop()
	log.Printf("[WATCHER] Watching directory: %s", dir)
	return cw
}

func (cw *ConfigWatcher) watchLoop() {
	for {
		select {
		case event, ok := <-cw.watcher.Events:
			if !ok {
				return
			}
			basename := filepath.Base(event.Name)
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				switch basename {
				case "mode":
					newMode := cw.readModeFile()
					cw.mu.Lock()
					oldMode := cw.mode
					cw.mode = newMode

					// Clear auto state on explicit mode switch
					if oldMode == "auto" && newMode != "auto" && autoSwitch != nil {
						log.Printf("[AUTO] Mode changed from 'auto' to '%s' -- clearing auto-switch state", newMode)
						autoSwitch.reset()
					}

					cw.mu.Unlock()
					if oldMode != newMode {
						log.Println("=======================================================")
						log.Printf("  MODE CHANGED: %s -> %s", oldMode, newMode)
						log.Println("=======================================================")
					}
				case "config.json":
					if cfg := cw.readConfigFile(); cfg != nil {
						cw.mu.Lock()
						cw.config = cfg
						cw.mu.Unlock()
						log.Printf("[WATCHER] Config reloaded")
					}
				}
			}
		case err, ok := <-cw.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[WATCHER] Error: %v", err)
		}
	}
}

func (cw *ConfigWatcher) readModeFile() string {
	path := filepath.Join(cw.dir, "mode")
	content, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[WATCHER] Error reading mode file: %v", err)
		}
		return appConfig.DefaultMode
	}
	mode := strings.TrimSpace(string(content))
	// "auto" is valid for auto-routing, plus any mode in config
	if mode == "auto" {
		return mode
	}
	if _, ok := appConfig.Modes[mode]; !ok {
		log.Printf("[WATCHER] Unknown mode '%s', defaulting to %s", mode, appConfig.DefaultMode)
		return appConfig.DefaultMode
	}
	return mode
}

func (cw *ConfigWatcher) readConfigFile() *Config {
	path := filepath.Join(cw.dir, "config.json")
	cfg, err := loadConfig(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[WATCHER] Error reading config.json: %v", err)
		}
		return nil
	}
	return cfg
}

// GetMode returns the cached mode (no I/O).
func (cw *ConfigWatcher) GetMode() string {
	if cw.watcher == nil {
		// Fallback: read from disk if watcher failed
		return cw.readModeFile()
	}
	cw.mu.RLock()
	defer cw.mu.RUnlock()
	return cw.mode
}

// GetConfig returns the cached config (no I/O).
func (cw *ConfigWatcher) GetConfig() *Config {
	if cw.watcher == nil {
		return cw.config
	}
	cw.mu.RLock()
	defer cw.mu.RUnlock()
	return cw.config
}

// Close stops the filesystem watcher.
func (cw *ConfigWatcher) Close() {
	if cw.watcher != nil {
		cw.watcher.Close()
	}
}
