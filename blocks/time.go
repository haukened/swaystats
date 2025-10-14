package blocks

import (
	"time"
)

// TimeProvider implements Provider for the clock.
type TimeProvider struct {
	interval int64 // desired minimum refresh interval (ns)
	format   string
	lastSec  int64 // last rendered wall-clock second
	blk      Block
}

func NewTimeProvider(interval time.Duration, format string) *TimeProvider {
	tp := &TimeProvider{interval: int64(interval), format: format}
	now := time.Now()
	tp.lastSec = now.Unix() - 1 // force first refresh
	tp.MaybeRefresh(now.UnixNano())
	return tp
}

func (t *TimeProvider) Name() string { return "time" }

func (t *TimeProvider) MaybeRefresh(now int64) bool {
	sec := now / int64(time.Second)
	if sec == t.lastSec { // same second, nothing to do
		return false
	}
	// Optionally enforce minimum interval (useful if TickHz>1) by checking now - lastSec*1s
	if t.interval > 0 && now-(t.lastSec*int64(time.Second)) < t.interval {
		// Even if second changed, respect minimum custom interval (rare for clock)
	}
	t.lastSec = sec
	txt := time.Unix(sec, 0).Format(t.format)
	if t.blk.FullText == txt { // defensive
		return false
	}
	t.blk = Block{
		Name:                "time",
		FullText:            txt,
		Separator:           false,
		SeparatorBlockWidth: SeparatorWidth,
	}
	return true
}

func (t *TimeProvider) Current() Block { return t.blk }
