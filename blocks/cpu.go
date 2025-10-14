package blocks

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"swaystats/config"
	"swaystats/theme"
)

// CpuProvider implements aggregate CPU utilization using /proc/stat deltas.
type CpuProvider struct {
	intervalNs      int64
	lastSampleNs    int64
	prevTotal       uint64
	prevIdle        uint64
	havePrev        bool
	lastPercent     float64
	blk             Block
	warnThreshold   float64
	dangerThreshold float64
	precision       int // 0 or 1
	prefix          string
}

func NewCpuProvider(cfg *config.Config) *CpuProvider {
	iv := cfg.Modules.CPU.IntervalSec
	if iv <= 0 {
		iv = 2
	}
	if iv > 30 {
		iv = 30
	}
	warn := cfg.Modules.CPU.WarnPercent
	if warn <= 0 {
		warn = 70
	}
	danger := cfg.Modules.CPU.DangerPercent
	if danger <= warn {
		danger = warn + 10
	}
	if danger > 100 {
		danger = 100
	}
	precision := cfg.Modules.CPU.Precision
	if precision < 0 || precision > 1 {
		precision = 0
	}
	prefix := cfg.Modules.CPU.Prefix
	if prefix == "" {
		prefix = "CPU"
	}
	cp := &CpuProvider{
		intervalNs:      int64(time.Duration(iv) * time.Second),
		warnThreshold:   float64(warn),
		dangerThreshold: float64(danger),
		precision:       precision,
		prefix:          prefix,
	}
	// Force initial sample so we have a baseline (will likely show 0% first time).
	cp.sample(time.Now().UnixNano())
	return cp
}

func (c *CpuProvider) Name() string { return "cpu" }

func (c *CpuProvider) MaybeRefresh(now int64) bool {
	if now-c.lastSampleNs < c.intervalNs {
		return false
	}
	changed := c.sample(now)
	return changed
}

func (c *CpuProvider) Current() Block { return c.blk }

func (c *CpuProvider) sample(now int64) bool {
	user, nice, system, idle, iowait, irq, softirq, steal, err := readProcStat()
	if err != nil {
		// On error, keep existing block; if we never had one, create error block.
		if c.blk.FullText == "" {
			c.blk = ErrorBlock("cpu", "cpu err")
		}
		c.lastSampleNs = now
		return false
	}
	idleAll := idle + iowait
	nonIdle := user + nice + system + irq + softirq + steal
	total := idleAll + nonIdle
	var percent float64
	if c.havePrev {
		deltaTotal := float64(total - c.prevTotal)
		deltaIdle := float64(idleAll - c.prevIdle)
		if deltaTotal > 0 {
			percent = (deltaTotal - deltaIdle) / deltaTotal * 100.0
		} else {
			percent = c.lastPercent // reuse
		}
	} else {
		percent = 0
		c.havePrev = true
	}
	c.prevTotal = total
	c.prevIdle = idleAll
	c.lastSampleNs = now

	// Round according to precision.
	formattedPercent := formatPercent(percent, c.precision)
	if formattedPercent == formatPercent(c.lastPercent, c.precision) && c.blk.FullText != "" {
		c.lastPercent = percent
		return false // no visible change
	}
	c.lastPercent = percent

	sev := theme.SeverityNormal
	if percent >= c.dangerThreshold {
		sev = theme.SeverityDanger
	} else if percent >= c.warnThreshold {
		sev = theme.SeverityWarn
	}
	color, ok := theme.ColorFor(sev)
	full := fmt.Sprintf("%s %s", c.prefix, formattedPercent)
	blk := Block{
		Name:                "cpu",
		FullText:            full,
		Separator:           false,
		SeparatorBlockWidth: SeparatorWidth,
	}
	if ok {
		blk.Color = color
	}
	c.blk = blk
	return true
}

func formatPercent(p float64, precision int) string {
	if precision == 0 {
		return strconv.FormatInt(int64(p+0.5), 10) + "%"
	}
	// one decimal
	return fmt.Sprintf("%.1f%%", p)
}

// readProcStat reads the first line of /proc/stat and returns selected fields.
func readProcStat() (user, nice, system, idle, iowait, irq, softirq, steal uint64, err error) {
	f, e := os.Open("/proc/stat")
	if e != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, e
	}
	defer f.Close()
	rd := bufio.NewReader(f)
	line, e := rd.ReadBytes('\n')
	if e != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, e
	}
	// Expected: cpu  numbers...
	// We'll parse manually for first 8 numeric fields.
	// Skip leading "cpu" token.
	i := 0
	// Skip initial spaces
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	// Expect 'c'
	if i >= len(line) || line[i] != 'c' {
		return 0, 0, 0, 0, 0, 0, 0, 0, errors.New("no cpu prefix")
	}
	// Move to after token
	for i < len(line) && line[i] != ' ' && line[i] != '\t' {
		i++
	}
	// Consume spaces
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	fields := make([]uint64, 0, 8)
	start := i
	for i <= len(line) {
		if i == len(line) || line[i] == ' ' || line[i] == '\t' || line[i] == '\n' { // end token
			if start < i {
				v, perr := parseUint(line[start:i])
				if perr != nil {
					return 0, 0, 0, 0, 0, 0, 0, 0, perr
				}
				fields = append(fields, v)
				if len(fields) == 8 {
					break
				}
			}
			i++
			for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
				i++
			}
			start = i
			continue
		}
		i++
	}
	if len(fields) < 8 {
		return 0, 0, 0, 0, 0, 0, 0, 0, errors.New("short cpu stat")
	}
	return fields[0], fields[1], fields[2], fields[3], fields[4], fields[5], fields[6], fields[7], nil
}

func parseUint(b []byte) (uint64, error) {
	var n uint64
	if len(b) == 0 {
		return 0, errors.New("empty")
	}
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, errors.New("nan")
		}
		n = n*10 + uint64(c-'0')
	}
	return n, nil
}
