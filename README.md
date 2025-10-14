# swaystats

Minimal, resilient status line generator for sway/i3 (i3bar protocol v1). Outputs JSON on stdout; reads click events on stdin.

## Status
Early scaffold: time block only, provider architecture in place, config defaults in code, no external deps. Colors only applied for error / abnormal states (future thresholds).

## Build

```bash
go build
```

Produces `swaystats` binary.

## Run (sway config)

In your `~/.config/sway/config`:

```
bar {
	status_command /home/you/bin/swaystats
}
```

## Output Protocol
Prints header then a forever-growing JSON array per i3bar spec:

```
{"version":1,"click_events":true}
[
[]
[, {"full_text":"2025-10-14 13:37:42"} ...
```

## Config (planned)
Search order (first existing file wins):
1. `$XDG_CONFIG_HOME/swaystats/config.toml`
2. `$XDG_CONFIG_HOME/swaystats.toml`
3. `~/.config/swaystats/config.toml`
4. `~/.config/swaystats.toml`

Currently: no file parsing yet; defaults are compiled in.

### Defaults
```
tick_hz = 1
[blocks.time]
format = "2006-01-02 15:04:05"
```

## Roadmap (short)
- Parse TOML config (BurntSushi/toml).
- Add CPU, memory, battery, network, temperature blocks with caching.
- Threshold-based coloring (warn/danger only).
- SIGHUP reload.
- Click handlers mapped to configured commands.

## Philosophy
Avoid crashes. On data errors, emit placeholder blocks and keep going. Log to stderr only; never pollute stdout.

## Provider Pattern
Each statistic source implements a small `Provider` interface:

```
type Provider interface {
	Name() string
	MaybeRefresh(now int64) (changed bool)
	Current() blocks.Block
}
```

Main loop ticks at configurable `tick_hz` (default 1). For each provider:
1. Call `MaybeRefresh(now)` letting it decide if enough time passed or data changed.
2. Collect `Current()` block regardless (ensures stable ordering).
3. Marshal and emit the full ordered slice.

### Why this design?
* Heterogeneous intervals: battery might refresh every 30s, time every 1s, CPU every 2s—without goroutine sprawl.
* Cheap change detection: providers skip work if not due; time provider always changes; others can compare values.
* Simple future event integration: a provider can set an internal "dirty" flag from a watcher and force a refresh next tick.
* Deterministic ordering: output order is fixed by provider slice, independent of update timing.
* Low allocations: one pass builds blocks; internal state stays cached.

### Emission Strategy
Currently we still output every tick (time changes). Later we can suppress output if no provider changed (except time) to reduce churn for slow setups, or keep heartbeat for predictability.

### Adding a Provider (soon)
Implement the interface, store: interval (ns), last refresh timestamp, last block, data fields. Example skeleton:

```
type CpuProvider struct {
	interval int64
	lastNs   int64
	blk      blocks.Block
	// cached prev counters...
}
```

Future blocks will follow this template.

