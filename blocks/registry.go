package blocks

import "swaystats/config"

// ProviderSpec describes how to enable and build a provider.
type ProviderSpec struct {
	Name   string
	Enable func(*config.Config) bool
	Build  func(*config.Config) Provider
}

var (
	reg      = map[string]ProviderSpec{}
	regOrder []string
)

// Register adds a provider spec if not already present. Subsequent registrations
// with the same name overwrite the spec but preserve original ordering.
func Register(spec ProviderSpec) {
	if _, exists := reg[spec.Name]; !exists {
		regOrder = append(regOrder, spec.Name)
	}
	reg[spec.Name] = spec
}

// BuildProviders returns provider instances in the order:
// 1. Order of module tables as specified in config file.
// 2. Remaining registered providers (those not present in config order) in registration order.
func BuildProviders(cfg *config.Config) []Provider {
	order := cfg.ModuleOrder()
	seen := map[string]struct{}{}
	providers := []Provider{}
	appendIf := func(name string) {
		spec, ok := reg[name]
		if !ok {
			return // unknown name in config
		}
		if spec.Enable != nil && !spec.Enable(cfg) {
			return
		}
		providers = append(providers, spec.Build(cfg))
		seen[name] = struct{}{}
	}
	if len(order) > 0 { // explicit config file: only build those listed and enabled
		for _, n := range order {
			appendIf(n)
		}
		return providers
	}
	// No explicit file order (defaults case): use registration order
	for _, n := range regOrder {
		appendIf(n)
	}
	return providers
}
