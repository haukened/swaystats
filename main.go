package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"swaystats/blocks"
	"swaystats/clicks"
	"swaystats/config"
)

func main() {
	log.SetOutput(os.Stderr)
	cfg, err := config.Load("")
	if err != nil {
		log.Printf("config: %v", err)
	}

	// Providers (ordered). Time first, then CPU.
	providers := []blocks.Provider{}
	if cfg.Modules.Time.Enabled {
		providers = append(providers, blocks.NewTimeProvider(time.Second, cfg.Modules.Time.Format))
	}
	if cfg.Modules.CPU.Enabled {
		providers = append(providers, blocks.NewCpuProvider(cfg))
	}

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

	// firstRow tracks whether we've emitted the first data row after the initial empty array.
	firstRow := true
	buf := bytes.NewBuffer(nil)
	for {
		// Service pending clicks quickly (non-blocking drain) before next tick alignment sleep.
		for {
			select {
			case ev := <-clickCh:
				handleClick(ev)
			default:
				goto render
			}
		}
	render:
		nowNs := time.Now().UnixNano()
		changed := false
		blocksOut := make([]blocks.Block, 0, len(providers))
		for _, p := range providers {
			if p.MaybeRefresh(nowNs) {
				changed = true
			}
			blocksOut = append(blocksOut, p.Current())
		}
		if changed || len(blocksOut) > 0 {
			buf.Reset()
			enc := json.NewEncoder(buf)
			if err := enc.Encode(blocksOut); err != nil {
				log.Printf("encode blocks: %v", err)
			} else {
				outBytes := bytes.TrimRight(buf.Bytes(), "\n")
				if firstRow {
					firstRow = false
				} else {
					fmt.Print(",")
				}
				fmt.Println(string(outBytes))
			}
		}
		waitUntilNextTickInterval(interval, clickCh)
	}
}
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
