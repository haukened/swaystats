# Copilot Guide – `swaystats`

## Project purpose
`swaystats` is a **status producer** for **swaybar** (i3bar protocol). It writes i3bar-compatible JSON to **stdout** in a loop and reads **click events** from **stdin**. It should be:
- **Fast** (≤1s tick), **low-overhead**, **no GUI**, one static binary.
- **Robust**: no panics, never print non-protocol data to stdout, graceful on errors.
- **Wayland/Sway friendly**: avoid X11 assumptions; shell out only when necessary.

## Output interface (i3bar JSON protocol)
We speak **version 1** of the protocol.

### 1) Header (write once)
```json
{"version":1,"click_events":true}
[
```
Immediately follow the header with an empty array:
```json
[]
```

### 2) Updates (forever)
Print **one JSON array per line**, each array containing block objects. Every line **after** the initial empty array must be **prefixed with a comma**.

Example tick:
```json
,[{"name":"net","instance":"wlan0","full_text":"ssid -55 dBm","color":"#89b4fa","separator":false,"separator_block_width":12},
  {"name":"cpu","full_text":"CPU 7%","color":"#a6e3a1"},
  {"name":"time","full_text":"2025-10-13 11:22:33","color":"#cdd6f4"}]
```

### 3) Clicks (stdin)
When `click_events:true`, swaybar sends one JSON object per click to **stdin**. We must **non-blocking** read and act.

Example click:
```json
{"name":"net","instance":"wlan0","button":1,"x":123,"y":6,"modifiers":["Shift"]}
```

## Block schema (what we emit)
Each block is a JSON object. Fields we commonly use:

- `name`: stable identifier for the block (e.g., `"net"`, `"cpu"`, `"bat"`).
- `instance`: optional disambiguator (e.g., interface name `"wlan0"`, `"BAT0"`).
- `full_text`: required display text.
- `short_text`: optional shorter text used by some bars.
- `color`: hex `#RRGGBB` (foreground).
- `background`: hex background color (optional).
- `separator`: boolean; if `false`, bar won’t draw vertical separator.
- `separator_block_width`: pixels reserved for a separator (we often use `12`).
- `urgent`: boolean for alerting.
- `markup`: `"pango"` to allow `<span>` formatting in `full_text`.

### Pango example (optional)
```json
{"name":"temp","markup":"pango","full_text":"<span color=\"#ff5555\">90°C</span>"}
```

## Behavioral rules (important)
- **Only** i3bar JSON goes to **stdout**.  
  All logs, errors, diagnostics go to **stderr**.
- Never block the output loop waiting on a slow subprocess. Cache and amortize.
- Keep **update cadence** predictable (e.g., 1s clock). Slower sources (Wi-Fi scan, package updates) refresh on longer intervals and reuse cached values between ticks.
- Handle **SIGHUP** to reload config (future) and **SIGTERM** to exit cleanly.
- On any data source error, show a sane placeholder (e.g., `"offline"`, `"N/A"`), do not crash.

## Go conventions (what to generate)
- Standard library only unless a strong reason otherwise.
- Single `main` package with these internal pieces:
  - `blocks/` package: functions returning lightweight structs representing blocks (`Block`).
  - `clicks/` package: stdin reader that emits typed click events over a channel.
  - `proc/` helpers: zero-alloc readers for `/proc` and `/sys`.
  - `theme/` constants for colors (single palette).
- **Event loop** pattern:
  - Write header.
  - Spawn click reader goroutine (scan stdin → send `Click` on channel).
  - Ticker for 1s ticks.
  - Select on `{tick, click}`; rebuild a slice of `Block`, `json.Marshal`, print with leading comma after the first payload.
- **Structs**:
  ```go
  type Block struct {
      Name                string `json:"name,omitempty"`
      Instance            string `json:"instance,omitempty"`
      FullText            string `json:"full_text"`
      ShortText           string `json:"short_text,omitempty"`
      Color               string `json:"color,omitempty"`
      Background          string `json:"background,omitempty"`
      Separator           bool   `json:"separator"`
      SeparatorBlockWidth int    `json:"separator_block_width,omitempty"`
      Urgent              bool   `json:"urgent,omitempty"`
      Markup              string `json:"markup,omitempty"`
  }

  type Click struct {
      Name      string   `json:"name"`
      Instance  string   `json:"instance,omitempty"`
      Button    int      `json:"button"`
      X         int      `json:"x"`
      Y         int      `json:"y"`
      Modifiers []string `json:"modifiers"`
  }
  ```
- **Logging**: `log.SetOutput(os.Stderr)`. Never print to stdout except the protocol.
- **Error tolerance**: missing files in `/sys` or `/proc` must not be fatal.

## Minimal skeleton (outline)
```go
func main() {
    log.SetOutput(os.Stderr)
    // 1) Header
    fmt.Println(`{"version":1,"click_events":true}`)
    fmt.Println("[")
    fmt.Println("[]")

    clicks := make(chan Click, 16)
    go readClicks(clicks) // scans stdin, json.Unmarshal per line

    first := true
    tick := time.NewTicker(time.Second)
    defer tick.Stop()

    for {
        select {
        case c := <-clicks:
            handleClick(c) // non-blocking: spawn execs in goroutines if needed
        case <-tick.C:
            blocks := buildBlocks() // read proc/sys quickly; reuse caches
            b, _ := json.Marshal(blocks)
            if first { first = false } else { fmt.Print(",") }
            fmt.Println(string(b))
        }
    }
}
```

## Blocks to implement (initial set)
- **Time**: `time.Now().Format("2006-01-02 15:04:05")`
- **CPU**: compute from `/proc/stat` delta (store last totals; 1s sample).
- **Memory**: parse `MemAvailable`/`MemTotal` from `/proc/meminfo`.
- **Battery**: read `/sys/class/power_supply/BAT*/capacity` + `status`.
- **Network**: prefer `/sys/class/net/*/operstate` and `wireless/` RSSI; fallback to `nmcli` if present (but avoid calling it every tick).
- **Temperature**: `hwmon` under `/sys/class/hwmon` (max sensor).

Each block returns `(Block, error)` and higher-level aggregator handles errors → `N/A`.

Blocks should follow good software architecture patterns, CLEAN / Hexagonal / Ports
- Must follow SOLID principles

## Config
- Should be a separate module
- Should return parsed config when Load() is called
- May be passed an optional config file path.
- should fall back in the following order:
  - path provided by user as CLI argument
  - `$XDG_CONFIG_HOME/swaystats/config.toml`
  - `$HOME/.config/swaystats/config.toml`
  - internal defaults
- Should support at least:
  - color pallete (normal, warning, danger, etc...)
  - modules to enable/disable
  - module specific settings (e.g., battery thresholds, network interface names, etc...)
- Should support reloading on SIGHUP

## Separators & spacing
Set `Separator:false` on all blocks and a uniform `SeparatorBlockWidth` (e.g., `12`). If you want visual gaps, add a dedicated spacer block with `FullText:" "` and the bar background color.

## Click handling (examples)
- `net` left-click → open `nmtui` in terminal.
- `cpu` left-click → open `htop`.
- `bat` left-click → show `upower` details.
Implement via `exec.Command(...).Start()` inside a goroutine. Never block the tick.

## Sway integration
In `~/.config/sway/config`:
```conf
bar {
  position top
  font pango:Noto Sans 10
  status_command /usr/local/bin/swaystats
  colors {
    background #323232
    statusline #ffffff
  }
}
```

## Testing
- `go vet`, `golangci-lint` (optional), unit tests for parsers (e.g., `/proc/stat` delta).
- Manual test:
  ```bash
  ./swaystats 2>~/swaystats.log | jq .
  ```
  (Replace swaybar with a pipe to `jq` to validate well-formed JSON.)
- Fuzz block builders with empty/malformed `/proc` inputs.

## Performance targets
- CPU time negligible at 1 Hz; no persistent goroutine leaks.
- No more than a few KiB allocations per tick; reuse buffers where reasonable.
- Subprocess invocations (e.g., `nmcli`) **≤ 1/15s** and cached.

## Non-goals
- We’re not a bar (no rendering), not Waybar, not a system tray.
- No privacy-sensitive network scanning by default.
- No root requirements.

---

**Copilot: when generating code**
- Prefer stdlib; keep it small. use third-party libraries sparingly, when justified.
- Follow Go idioms and best practices.
- Write clear, maintainable, well-structured code.
- Use proper error handling; avoid panics.
- Write comments for complex logic, but avoid obvious comments.
- Use consistent formatting and naming conventions.
- Write modular code; separate concerns into functions and packages.
- Write unit tests for critical functions and edge cases.
- Ensure the code adheres to the behavioral rules outlined above:
- Never print anything except i3bar JSON on stdout.
- Use buffered readers for stdin clicks; do not block the tick.
- Fail soft and keep the bar alive.
