package blocks

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"swaystats/config"
	"swaystats/theme"
	"time"
)

// MemoryProvider provides memory utilization / availability stats.
type MemoryProvider struct {
	intervalNs      int64
	lastSampleNs    int64
	lastPercent     float64
	blk             Block
	warnThreshold   float64
	dangerThreshold float64
	precision       int
	prefix          string
	format          string // percent|available|used
}

func NewMemoryProvider(cfg *config.Config) *MemoryProvider {
	mcfg := cfg.Modules.Mem
	iv := mcfg.IntervalSec
	if iv <= 0 {
		iv = 5
	}
	if iv > 60 {
		iv = 60
	}
	warn := mcfg.WarnPercent
	if warn <= 0 {
		warn = 70
	}
	danger := mcfg.DangerPercent
	if danger <= warn {
		danger = warn + 10
	}
	if danger > 100 {
		danger = 100
	}
	precision := mcfg.Precision
	if precision < 0 || precision > 1 {
		precision = 0
	}
	format := strings.ToLower(mcfg.Format)
	switch format {
	case "percent", "available", "used":
	default:
		format = "percent"
	}
	prefix := mcfg.Prefix
	if prefix == "" {
		prefix = "MEM"
	}
	mp := &MemoryProvider{
		intervalNs:      int64(time.Duration(iv) * time.Second),
		warnThreshold:   float64(warn),
		dangerThreshold: float64(danger),
		precision:       precision,
		prefix:          prefix,
		format:          format,
	}
	mp.sample(time.Now().UnixNano())
	return mp
}

func (m *MemoryProvider) Name() string { return "mem" }

func (m *MemoryProvider) MaybeRefresh(now int64) bool {
	if now-m.lastSampleNs < m.intervalNs {
		return false
	}
	return m.sample(now)
}

func (m *MemoryProvider) Current() Block { return m.blk }

func (m *MemoryProvider) sample(now int64) bool {
	total, available, used, percent, err := readMemInfo()
	if err != nil {
		if m.blk.FullText == "" {
			m.blk = ErrorBlock("mem", "mem err")
		}
		m.lastSampleNs = now
		return false
	}
	// Round percent for change detection based on precision.
	formattedPercent := formatPercent(percent, m.precision)
	text := m.buildText(total, available, used, formattedPercent)
	if text == m.blk.FullText { // no visible change
		m.lastSampleNs = now
		m.lastPercent = percent
		return false
	}
	m.lastSampleNs = now
	m.lastPercent = percent
	sev := theme.SeverityNormal
	if percent >= m.dangerThreshold {
		sev = theme.SeverityDanger
	} else if percent >= m.warnThreshold {
		sev = theme.SeverityWarn
	}
	color, ok := theme.ColorFor(sev)
	blk := Block{Name: "mem", FullText: text, Separator: false, SeparatorBlockWidth: SeparatorWidth}
	if ok {
		blk.Color = color
	}
	m.blk = blk
	return true
}

func (m *MemoryProvider) buildText(total, available, used uint64, percentStr string) string {
	switch m.format {
	case "available":
		return fmt.Sprintf("%s %s free", m.prefix, humanBytes(available))
	case "used":
		return fmt.Sprintf("%s %s used", m.prefix, humanBytes(used))
	default: // percent
		return fmt.Sprintf("%s %s", m.prefix, percentStr)
	}
}

// readMemInfo returns total, available, used, percentUsed based on /proc/meminfo.
func readMemInfo() (total, available, used uint64, percent float64, err error) {
	f, e := os.Open("/proc/meminfo")
	if e != nil {
		return 0, 0, 0, 0, e
	}
	defer f.Close()
	var memTotal, memAvailable, memFree, buffers, cached uint64
	haveAvailable := false
	rd := bufio.NewReader(f)
	for {
		line, e := rd.ReadBytes('\n')
		if len(line) == 0 && e != nil {
			break
		}
		// Lines look like: Key:  Value kB
		// We only care about a few; simple prefix checks.
		if hasPrefix(line, "MemTotal:") {
			memTotal = parseMeminfoValue(line)
		} else if hasPrefix(line, "MemAvailable:") {
			memAvailable = parseMeminfoValue(line)
			haveAvailable = true
		} else if hasPrefix(line, "MemFree:") {
			memFree = parseMeminfoValue(line)
		} else if hasPrefix(line, "Buffers:") {
			buffers = parseMeminfoValue(line)
		} else if hasPrefix(line, "Cached:") {
			cached = parseMeminfoValue(line)
		}
		if e != nil {
			break
		}
		if memTotal > 0 && haveAvailable && memAvailable > 0 { /* can early exit if desired */
		}
	}
	if memTotal == 0 {
		return 0, 0, 0, 0, errors.New("no MemTotal")
	}
	if haveAvailable {
		used = (memTotal - memAvailable) * 1024
		total = memTotal * 1024
		available = memAvailable * 1024
	} else {
		// Fallback heuristic
		availableKb := memFree + buffers + cached
		used = (memTotal - availableKb) * 1024
		total = memTotal * 1024
		available = availableKb * 1024
	}
	percent = (float64(used) / float64(total)) * 100
	return total, available, used, percent, nil
}

func hasPrefix(line []byte, prefix string) bool {
	if len(line) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if line[i] != prefix[i] {
			return false
		}
	}
	return true
}

// parseMeminfoValue extracts the numeric kB value from a meminfo line.
func parseMeminfoValue(line []byte) uint64 {
	// Find first digit
	i := 0
	for i < len(line) && (line[i] < '0' || line[i] > '9') {
		i++
	}
	val := uint64(0)
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		val = val*10 + uint64(line[i]-'0')
		i++
	}
	return val // still in kB units
}

// humanBytes converts bytes to a short human string (KiB, MiB, GiB) with up to one decimal.
func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit && exp < 4; n /= unit {
		div *= unit
		exp++
	}
	value := float64(b) / float64(div)
	// Up to one decimal for values <10, else integer.
	if value < 10 {
		return fmt.Sprintf("%.1f%ciB", value, "KMGTPE"[exp])
	}
	return fmt.Sprintf("%.0f%ciB", value, "KMGTPE"[exp])
}
