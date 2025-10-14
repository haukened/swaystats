package theme

// Minimal color policy: only color abnormal (warn/danger) states.
// Normal blocks omit the Color field so the bar theme handles appearance.
// Future-proofing: introduce a Palette struct so we can override later via config
// without changing call sites. For now, overrides are NOT implemented; we just
// use the defaults.

type Palette struct {
	Warn   string
	Danger string
}

var DefaultPalette = Palette{
	Warn:   "#d08770", // orange
	Danger: "#bf616a", // red
}

// Current holds the active palette; swapping this in the future will update colors.
var Current = DefaultPalette

// Backwards compatibility constants (retain existing names) referencing Current.
// These stay so existing code using ColorWarn / ColorDanger still compiles.
var (
	ColorWarn   = Current.Warn
	ColorDanger = Current.Danger
)

type Severity int

const (
	SeverityNormal Severity = iota
	SeverityWarn
	SeverityDanger
)

// ColorFor returns the hex color and true if severity maps to a color.
func ColorFor(sev Severity) (string, bool) {
	switch sev {
	case SeverityWarn:
		return Current.Warn, true
	case SeverityDanger:
		return Current.Danger, true
	default:
		return "", false
	}
}

// NOTE: In a future release we may add ApplyOverrides(warn, danger string) to mutate
// Current and update ColorWarn/ColorDanger. For now we purposely avoid mutability.
