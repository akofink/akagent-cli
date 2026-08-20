package integration

import (
	"reflect"
	"testing"

	"github.com/akofink/akagent-cli/internal/pi"
)

func TestParsePiArgumentsAppliesDefaultPolicy(t *testing.T) {
	policy, prompt, context, err := parsePiArguments([]string{"--", "@/tmp/prompt.md", "--context", "issue-95"})
	if err != nil {
		t.Fatal(err)
	}
	if policy != (pi.LaunchPolicy{Provider: pi.DefaultProvider, Model: pi.DefaultModel, Thinking: pi.DefaultThinking}) {
		t.Fatalf("policy = %#v, want managed defaults", policy)
	}
	if prompt != "/tmp/prompt.md" || context != "issue-95" {
		t.Fatalf("parsed prompt/context = %q/%q", prompt, context)
	}
}

func TestParsePiArgumentsPreservesExplicitPolicy(t *testing.T) {
	policy, _, _, err := parsePiArguments([]string{"--provider", "anthropic", "--model", "claude-sonnet", "--thinking", "low"})
	if err != nil {
		t.Fatal(err)
	}
	if got := policy.Args(); !reflect.DeepEqual(got, []string{"--provider", "anthropic", "--model", "claude-sonnet", "--thinking", "low"}) {
		t.Fatalf("policy args = %#v, want explicit override", got)
	}
}

func TestParsePiArgumentsRejectsInvalidPolicy(t *testing.T) {
	if _, _, _, err := parsePiArguments([]string{"--thinking", "maximum"}); err == nil {
		t.Fatal("parsePiArguments() accepted unsupported thinking level")
	}
}
