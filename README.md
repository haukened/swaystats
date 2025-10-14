# swaystats

Minimal, resilient status line generator for sway/i3 (i3bar protocol v1). Outputs JSON on stdout; reads click events on stdin.

## Status
Core providers implemented: time, CPU, memory. TOML config parsing implemented (BurntSushi/toml). Colors only applied for abnormal states (warn/danger thresholds).

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

## Config
Search order (first existing file wins):
1. `$XDG_CONFIG_HOME/swaystats/config.toml`
2. `~/.config/swaystats/config.toml`

If no file found, defaults are used and a note is logged to stderr.

Fields use snake_case in TOML. Unspecified values inherit defaults. Invalid or out-of-range values are clamped.

### Module Ordering
Provider output order is:
1. The order you declare `[modules.<name>]` tables in the config file.
2. Any remaining built-in modules you did not mention, in their internal registration order.

This means adding a new module requires only dropping a table in your config (or accepting its default position). No numeric order keys needed.

### Example `config.toml`
```
# Global tick frequency (status emission alignment base). 1..20
tick_hz = 1

[modules.time]
enabled = true
format = "2006-01-02 15:04:05"

[modules.cpu]
enabled = true
interval_sec = 2
warn_percent = 70
danger_percent = 90
precision = 0    # 0 or 1 decimal place
prefix = "CPU "

[modules.mem]
enabled = true
interval_sec = 5
warn_percent = 70
danger_percent = 90
precision = 0
prefix = "MEM "
format = "percent" # percent|available|used
```

### Defaults (effective)
Same as the example above. Only include overrides you wish to change.

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

