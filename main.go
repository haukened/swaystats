package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"swaystats/blocks"
	"swaystats/clicks"
	"swaystats/config"

	"github.com/fsnotify/fsnotify"
)

func main() {
	log.SetOutput(os.Stderr)
	cfg, err := config.Load("")
	if err != nil {
		log.Printf("config: %v", err)
	}

	// Build providers using registry + config order (held atomically for live reloads).
	var providers atomic.Value // []blocks.Provider
	providers.Store(blocks.BuildProviders(cfg))

	// i3bar protocol header and opening array.
	fmt.Println(`{"version":1,"click_events":true}`)
	fmt.Println("[")
	fmt.Println("[]")

	clickCh := make(chan clicks.Click, 16)
	go clicks.Read(os.Stdin, clickCh)

	if cfg.TickHz < 1 {
		cfg.TickHz = 1
	}
	if cfg.TickHz > 20 {
		cfg.TickHz = 20
	}
	interval := time.Second / time.Duration(cfg.TickHz)

	// Initial alignment to next fractional interval boundary.
	waitUntilNextTickInterval(interval, nil)

	// After emitting the initial empty array, every subsequent row must be comma-prefixed per i3bar protocol.
	// If we have a real config file, start watcher for automatic reloads.
	if cfg.SourcePath != "" {
		startConfigWatcher(cfg.SourcePath, func() {
			newCfg, err := config.Load(cfg.SourcePath)
			if err != nil {
				log.Printf("config reload failed: %v", err)
				return
			}
			providers.Store(blocks.BuildProviders(newCfg))
			cfg = newCfg
			log.Printf("config reloaded (%s)", cfg.SourcePath)
		})
	}

	buf := bytes.NewBuffer(nil)
	for {
		drainClicks(clickCh)
		current := providers.Load().([]blocks.Provider)
		renderOnce(buf, current)
		waitUntilNextTickInterval(interval, clickCh)
	}
}

// drainClicks consumes all currently queued click events without blocking.
func drainClicks(ch <-chan clicks.Click) {
	for {
		select {
		case ev := <-ch:
			handleClick(ev)
		default:
			return
		}
	}
}

// renderOnce refreshes providers (if due) and emits a JSON row.
func renderOnce(buf *bytes.Buffer, providers []blocks.Provider) {
	nowNs := time.Now().UnixNano()
	changed := false
	blocksOut := make([]blocks.Block, 0, len(providers))
	for _, p := range providers {
		if p.MaybeRefresh(nowNs) {
			changed = true
		}
		blocksOut = append(blocksOut, p.Current())
	}
	if !changed && len(blocksOut) == 0 {
		return
	}
	buf.Reset()
	enc := json.NewEncoder(buf)
	if err := enc.Encode(blocksOut); err != nil {
		log.Printf("encode blocks: %v", err)
		return
	}
	outBytes := bytes.TrimRight(buf.Bytes(), "\n")
	fmt.Print(",")
	fmt.Println(string(outBytes))
}

// startConfigWatcher watches a single file for WRITE/CHMOD events and invokes cb (debounced) on change.
func startConfigWatcher(path string, cb func()) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("config watcher init: %v", err)
		return
	}
	parent := filepath.Dir(path)
	if err := watcher.Add(parent); err != nil {
		log.Printf("config watcher add: %v", err)
		watcher.Close()
		return
	}
	go func() {
		defer watcher.Close()
		var pending bool
		var last time.Time
		for {
			select {
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				if !eventTargetsFile(ev, path) {
					continue
				}
				// debounce ~150ms
				if time.Since(last) < 150*time.Millisecond {
					pending = true
					continue
				}
				last = time.Now()
				cb()
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("config watcher error: %v", err)
			}
			if pending && time.Since(last) >= 150*time.Millisecond {
				pending = false
				last = time.Now()
				cb()
			}
		}
	}()
}

// eventTargetsFile checks if fsnotify event relates to the target file path.
func eventTargetsFile(ev fsnotify.Event, target string) bool {
	return ev.Name == target
}

// dirName is a small helper (since path/filepath not imported here yet) - import path/filepath instead.
// dirName helper removed (filepath.Dir used instead)

func handleClick(c clicks.Click) {
	// Placeholder: just log; future mapping to commands.
	log.Printf("click: %+v", c)
}

// waitUntilNextTickInterval sleeps until the next multiple of interval boundary.
// If clickCh is non-nil it will service a single click arrival without delaying
// the boundary more than necessary (best-effort responsiveness between ticks).
func waitUntilNextTickInterval(interval time.Duration, clickCh <-chan clicks.Click) {
	now := time.Now()
	// Compute next boundary: truncate to interval then add interval.
	next := now.Truncate(interval).Add(interval)
	if !next.After(now) { // rare edge if Truncate already returns future? guard anyway
		next = next.Add(interval)
	}
	for {
		dur := time.Until(next)
		if dur <= 0 {
			return
		}
		// Sleep in at most 100ms chunks to remain responsive for larger intervals.
		step := dur
		if step > 100*time.Millisecond {
			step = 100 * time.Millisecond
		}
		time.Sleep(step)
		// Drain a single click if present (non-blocking) to keep UI responsive.
		if clickCh != nil {
			select {
			case ev := <-clickCh:
				handleClick(ev)
			default:
			}
		}
	}
}
