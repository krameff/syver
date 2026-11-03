package resource

// DiscoveryMeta holds optional discovery, dependency and skip metadata shared
// by every resource type. It is embedded inline, so a key added here is
// accepted by strict per-resource decoding on all types at once.
type DiscoveryMeta struct {
	Register  string   `json:"register,omitempty" yaml:"register,omitempty"`
	DependsOn []string `json:"depends-on,omitempty" yaml:"depends-on,omitempty"`
	// SkipReason is the author's explanation for `skip: true`. It is copied
	// onto the resource's skipped results and ignored otherwise.
	SkipReason string `json:"skip-reason,omitempty" yaml:"skip-reason,omitempty"`
}

func (d DiscoveryMeta) GetRegister() string {
	return d.Register
}

func (d DiscoveryMeta) GetDependsOn() []string {
	return d.DependsOn
}

func (d DiscoveryMeta) GetSkipReason() string {
	return d.SkipReason
}
