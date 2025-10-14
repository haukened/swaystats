package config

import (
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	TickHz  int
	Modules Modules
}

type Modules struct {
	Time TimeModule
}

type TimeModule struct {
	Enabled bool
	Format  string
}

func Defaults() *Config {
	return &Config{
		TickHz: 1,
		Modules: Modules{
			Time: TimeModule{Enabled: true, Format: "2006-01-02 15:04:05"},
		},
	}
}

// Load loads configuration from explicit path or search paths; currently returns defaults only.
func Load(path string) (*Config, error) {
	if path != "" {
		if _, statErr := os.Stat(path); statErr == nil {
			return Defaults(), nil // TODO parse actual file
		} else {
			return Defaults(), statErr
		}
	}
	for _, p := range searchPaths() {
		if _, err := os.Stat(p); err == nil {
			return Defaults(), nil // TODO parse
		}
	}
	return Defaults(), errors.New("no config found; using defaults")
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
