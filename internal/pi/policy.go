// Package pi defines the non-secret launch policy shared by Pi integrations.
package pi

import (
	"fmt"
	"regexp"
)

const (
	DefaultProvider = "openai-codex"
	DefaultModel    = "gpt-5.6-luna"
	DefaultThinking = "high"
)

var launchValuePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,127}$`)

// LaunchPolicy selects the Pi provider, model, and thinking level.
// These values are safe to persist and pass as process arguments because they
// identify a launch configuration and never contain credentials.
type LaunchPolicy struct {
	Provider string
	Model    string
	Thinking string
}

// ResolveLaunchPolicy fills omitted fields with the managed-launch defaults
// and validates the complete policy.
func ResolveLaunchPolicy(provider, model, thinking string) (LaunchPolicy, error) {
	policy := LaunchPolicy{Provider: provider, Model: model, Thinking: thinking}
	if policy.Provider == "" {
		policy.Provider = DefaultProvider
	}
	if policy.Model == "" {
		policy.Model = DefaultModel
	}
	if policy.Thinking == "" {
		policy.Thinking = DefaultThinking
	}
	if err := policy.Validate(); err != nil {
		return LaunchPolicy{}, err
	}
	return policy, nil
}

func (p LaunchPolicy) Validate() error {
	if !launchValuePattern.MatchString(p.Provider) {
		return fmt.Errorf("Pi provider must be a non-empty identifier")
	}
	if !launchValuePattern.MatchString(p.Model) {
		return fmt.Errorf("Pi model must be a non-empty identifier")
	}
	switch p.Thinking {
	case "off", "minimal", "low", "medium", "high", "xhigh":
		return nil
	default:
		return fmt.Errorf("Pi thinking level must be one of off, minimal, low, medium, high, or xhigh")
	}
}

// Args returns the explicit Pi command-line policy.
func (p LaunchPolicy) Args() []string {
	return []string{"--provider", p.Provider, "--model", p.Model, "--thinking", p.Thinking}
}
