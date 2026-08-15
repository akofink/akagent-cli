package credential

import (
	"strings"
	"testing"
)

func TestBuildEnvironmentAllowsOnlyRequestedCredential(t *testing.T) {
	secret := "openai-secret-value"
	manifest := &Manifest{Version: 1, Entries: []Entry{{ID: "openai", Type: "api_token", Source: "env:OPENAI_API_KEY"}}}
	base := []string{
		"HOME=/home/test",
		"LANG=en_US.UTF-8",
		"PATH=/usr/bin",
		"OPENAI_API_KEY=" + secret,
		"UNREQUESTED_TOKEN=should-not-pass",
		"RANDOM=should-not-pass",
	}

	environment, err := BuildEnvironment(manifest, []string{}, base)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(environment, "\n")
	if strings.Contains(joined, secret) || strings.Contains(joined, "UNREQUESTED_TOKEN") {
		t.Fatalf("unrequested credential reached managed environment: %q", joined)
	}

	environment, err = BuildEnvironment(manifest, []string{"openai"}, base)
	if err != nil {
		t.Fatal(err)
	}
	joined = strings.Join(environment, "\n")
	if !strings.Contains(joined, "OPENAI_API_KEY="+secret) {
		t.Fatalf("requested credential was not injected: %q", joined)
	}
}

func TestBuildEnvironmentRejectsFileCredentialInjection(t *testing.T) {
	manifest := &Manifest{Version: 1, Entries: []Entry{{ID: "ssh", Type: "ssh_key", Source: "file:/tmp/key"}}}
	if _, err := BuildEnvironment(manifest, []string{"ssh"}, nil); err == nil || strings.Contains(err.Error(), "/tmp/key") {
		t.Fatalf("file credential error = %v, want generic redaction-safe error", err)
	}
}
