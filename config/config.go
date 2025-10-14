package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	TickHz      int      `toml:"tick_hz"`
	Modules     Modules  `toml:"modules"`
	moduleOrder []string // order of module tables as they appeared in TOML
}

type Modules struct {
	Time TimeModule   `toml:"time"`
	CPU  CPUModule    `toml:"cpu"`
	Mem  MemoryModule `toml:"mem"`
}

type TimeModule struct {
	Enabled bool   `toml:"enabled"`
	Format  string `toml:"format"`
}

type CPUModule struct {
	Enabled       bool   `toml:"enabled"`
	IntervalSec   int    `toml:"interval_sec"`   // sampling interval seconds (default 2)
	WarnPercent   int    `toml:"warn_percent"`   // warn threshold (default 70)
	DangerPercent int    `toml:"danger_percent"` // danger threshold (default 90)
	Precision     int    `toml:"precision"`      // decimals (0 or 1)
	Prefix        string `toml:"prefix"`         // text/icon prefix before percentage (default "CPU")
}

type MemoryModule struct {
	Enabled       bool   `toml:"enabled"`
	IntervalSec   int    `toml:"interval_sec"`   // sampling interval seconds (default 5)
	WarnPercent   int    `toml:"warn_percent"`   // warn threshold (default 70)
	DangerPercent int    `toml:"danger_percent"` // danger threshold (default 90)
	Precision     int    `toml:"precision"`      // percent decimals (0 or 1) for percent format
	Prefix        string `toml:"prefix"`         // text/icon prefix (default "MEM")
	Format        string `toml:"format"`         // one of: percent, available, used
}

func Defaults() *Config {
	return &Config{
		TickHz: 1,
		Modules: Modules{
			Time: TimeModule{Enabled: true, Format: "2006-01-02 15:04:05"},
			CPU:  CPUModule{Enabled: true, IntervalSec: 2, WarnPercent: 70, DangerPercent: 90, Precision: 0, Prefix: "CPU"},
			Mem:  MemoryModule{Enabled: true, IntervalSec: 5, WarnPercent: 70, DangerPercent: 90, Precision: 0, Prefix: "MEM", Format: "percent"},
		},
	}
}

// Load loads configuration from explicit path or discovered search path.
// Precedence: provided path (if exists) else first existing search path else defaults.
// Missing file yields defaults and an error; parse errors also return defaults + error.
func Load(path string) (*Config, error) {
	defaults := Defaults()
	var chosen string
	if path != "" {
		chosen = path
	} else {
		for _, p := range searchPaths() {
			if _, err := os.Stat(p); err == nil {
				chosen = p
				break
			}
		}
	}
	if chosen == "" { // no file found
		return defaults, errors.New("no config file found; using defaults")
	}
	data, err := os.ReadFile(chosen)
	if err != nil {
		return defaults, fmt.Errorf("read config: %w", err)
	}
	md, err := toml.Decode(string(data), defaults) // decode overlays onto defaults
	if err != nil {
		return defaults, fmt.Errorf("parse config: %w", err)
	}
	// Capture module order from metadata keys: modules.<name>
	seen := map[string]struct{}{}
	for _, k := range md.Keys() {
		if len(k) == 2 && k[0] == "modules" {
			name := k[1]
			if _, ok := seen[name]; !ok {
				defaults.moduleOrder = append(defaults.moduleOrder, name)
				seen[name] = struct{}{}
			}
		}
	}
	defaults.normalize()
	return defaults, nil
}

func searchPaths() []string {
	var out []string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		out = append(out, filepath.Join(xdg, "swaystats", "config.toml"))
	}
	if home, _ := os.UserHomeDir(); home != "" {
		out = append(out, filepath.Join(home, ".config", "swaystats", "config.toml"))
	}
	return out
}

// normalize clamps and validates config values after decoding.
func (c *Config) normalize() {
	c.normalizeTick()
	c.normalizeCPU()
	c.normalizeMem()
}

// ModuleOrder returns a copy of the module order slice (may be empty).
func (c *Config) ModuleOrder() []string {
	if len(c.moduleOrder) == 0 {
		return nil
	}
	out := make([]string, len(c.moduleOrder))
	copy(out, c.moduleOrder)
	return out
}

func (c *Config) normalizeTick() {
	c.TickHz = clampInt(c.TickHz, 1, 20, 1)
}

func (c *Config) normalizeCPU() {
	if c.Modules.CPU.IntervalSec <= 0 {
		c.Modules.CPU.IntervalSec = 2
	}
	c.Modules.CPU.Precision = clampInt(c.Modules.CPU.Precision, 0, 1, 0)
}

func (c *Config) normalizeMem() {
	if c.Modules.Mem.IntervalSec <= 0 {
		c.Modules.Mem.IntervalSec = 5
	}
	c.Modules.Mem.Precision = clampInt(c.Modules.Mem.Precision, 0, 1, 0)
	if c.Modules.Mem.Format == "" {
		c.Modules.Mem.Format = "percent"
	}
	if !validMemFormat(c.Modules.Mem.Format) {
		c.Modules.Mem.Format = "percent"
	}
}

func clampInt(val, min, max, fallback int) int {
	if val == 0 && fallback != 0 { // allow zero to trigger fallback when min>0
		val = fallback
	}
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func validMemFormat(f string) bool {
	switch f {
	case "percent", "available", "used":
		return true
	}
	return false
}
