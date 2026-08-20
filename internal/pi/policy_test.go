package pi

import (
	"reflect"
	"testing"
)

func TestResolveLaunchPolicyDefaults(t *testing.T) {
	policy, err := ResolveLaunchPolicy("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := LaunchPolicy{Provider: DefaultProvider, Model: DefaultModel, Thinking: DefaultThinking}
	if policy != want {
		t.Fatalf("ResolveLaunchPolicy() = %#v, want %#v", policy, want)
	}
	if got := policy.Args(); !reflect.DeepEqual(got, []string{"--provider", DefaultProvider, "--model", DefaultModel, "--thinking", DefaultThinking}) {
		t.Fatalf("Args() = %#v, want explicit default policy", got)
	}
}

func TestResolveLaunchPolicyPreservesOverrides(t *testing.T) {
	policy, err := ResolveLaunchPolicy("anthropic", "claude-sonnet", "low")
	if err != nil {
		t.Fatal(err)
	}
	if policy != (LaunchPolicy{Provider: "anthropic", Model: "claude-sonnet", Thinking: "low"}) {
		t.Fatalf("ResolveLaunchPolicy() = %#v, want caller overrides", policy)
	}
}

func TestResolveLaunchPolicyRejectsUnsafeOrUnsupportedValues(t *testing.T) {
	for name, values := range map[string][3]string{
		"empty provider override": {"\n", "model", "high"},
		"invalid model override":  {"provider", "model with spaces", "high"},
		"unsupported thinking":    {"provider", "model", "maximum"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ResolveLaunchPolicy(values[0], values[1], values[2]); err == nil {
				t.Fatal("ResolveLaunchPolicy() succeeded with invalid policy")
			}
		})
	}
}
