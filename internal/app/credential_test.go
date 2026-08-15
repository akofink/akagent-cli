package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akofink/akagent-cli/internal/credential"
)

func writeManifest(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.toon")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(credential.ConfigEnv, path)
	return dir
}

func TestCredentialList(t *testing.T) {
	writeManifest(t, `version: 1
credentials[2]{id,type,source,required_for}:
  git-ssh,ssh_key,file:/secrets/git,git
  gh-token,api_token,env:GITHUB_TOKEN,
`)
	var stdout bytes.Buffer
	if code := Run([]string{"credential", "list"}, &stdout); code != 0 {
		t.Fatalf("Run() exit = %d, stdout = %q", code, stdout.String())
	}
	for _, expected := range []string{"credentials[2]", "git-ssh", "gh-token", "missing", "unavailable", "optional credential gh-token is unavailable"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("output = %q, want to contain %q", stdout.String(), expected)
		}
	}
	if strings.Contains(stdout.String(), "GITHUB_TOKEN") {
		t.Errorf("output leaked source reference value: %q", stdout.String())
	}
}

func TestCredentialListEmptyState(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "no-such-manifest.toon")
	t.Setenv(credential.ConfigEnv, missing)

	var stdout bytes.Buffer
	if code := Run([]string{"credential", "list"}, &stdout); code != 0 {
		t.Fatalf("Run() exit = %d, stdout = %q", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "credentials: []") {
		t.Fatalf("output = %q, want definitive empty credentials array", stdout.String())
	}
}

func TestCredentialInspect(t *testing.T) {
	writeManifest(t, `credentials{id,type,source,required_for}:
  git-ssh,ssh_key,env:AGENT_SSH,
`)
	var stdout bytes.Buffer
	if code := Run([]string{"credential", "inspect", "git-ssh"}, &stdout); code != 0 {
		t.Fatalf("Run() exit = %d, stdout = %q", code, stdout.String())
	}
	for _, expected := range []string{"id: git-ssh", "type: ssh_key", "status: unavailable", "source: env", "reference: AGENT_SSH"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("output = %q, want to contain %q", stdout.String(), expected)
		}
	}
}

func TestCredentialInspectNotFound(t *testing.T) {
	writeManifest(t, `credentials{id,type,source,required_for}:
  git-ssh,ssh_key,env:AGENT_SSH,
`)
	var stdout bytes.Buffer
	if code := Run([]string{"credential", "inspect", "nope"}, &stdout); code != 1 {
		t.Fatalf("Run() exit = %d, want 1 for not_found", code)
	}
	for _, expected := range []string{"error:", "category: not_found"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("output = %q, want to contain %q", stdout.String(), expected)
		}
	}
}

func TestCredentialDoctorRequiredMissingFails(t *testing.T) {
	writeManifest(t, `credentials{id,type,source,required_for}:
  git-ssh,ssh_key,env:AGENT_SSH,git
  gh-token,api_token,env:GITHUB_TOKEN,
`)
	var stdout bytes.Buffer
	if code := Run([]string{"credential", "doctor"}, &stdout); code != 1 {
		t.Fatalf("Run() exit = %d, want 1 for required failure", code)
	}
	out := stdout.String()
	for _, expected := range []string{"overall: failed", "errors[1]", "required (git) credential git-ssh is unavailable", "warnings[1]", "optional credential gh-token is unavailable"} {
		if !strings.Contains(out, expected) {
			t.Errorf("output = %q, want to contain %q", out, expected)
		}
	}
}

func TestCredentialDoctorOptionalOnlyDegrades(t *testing.T) {
	writeManifest(t, `credentials{id,type,source,required_for}:
  gh-token,api_token,env:GITHUB_TOKEN,
`)
	var stdout bytes.Buffer
	if code := Run([]string{"credential", "doctor"}, &stdout); code != 0 {
		t.Fatalf("Run() exit = %d, want 0", code)
	}
	out := stdout.String()
	for _, expected := range []string{"overall: degraded", "warnings[1]"} {
		if !strings.Contains(out, expected) {
			t.Errorf("output = %q, want to contain %q", out, expected)
		}
	}
}

func TestCredentialDoctorReady(t *testing.T) {
	dir := t.TempDir()
	credsDir := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(credsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	tokenFile := filepath.Join(credsDir, "github")
	if err := os.WriteFile(tokenFile, []byte("placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, "credentials{id,type,source,required_for}:\n  github,api_token,file:"+tokenFile+",github\n")

	var stdout bytes.Buffer
	if code := Run([]string{"credential", "doctor"}, &stdout); code != 0 {
		t.Fatalf("Run() exit = %d, stdout = %q", code, stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "overall: ok") {
		t.Fatalf("output = %q, want overall ok", out)
	}
	// The doctor must never read or emit the file contents.
	if strings.Contains(out, "placeholder") {
		t.Errorf("output leaked secret content")
	}
}

func TestCredentialMalformedManifest(t *testing.T) {
	writeManifest(t, "credentials{bad}:\n")
	var stdout bytes.Buffer
	if code := Run([]string{"credential", "list"}, &stdout); code != 1 {
		t.Fatalf("Run() exit = %d, want 1", code)
	}
	for _, expected := range []string{"error:", "category: capability", "Credential manifest is invalid"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("output = %q, want to contain %q", stdout.String(), expected)
		}
	}
}

func TestCredentialUnknownFlagFailsBeforeSideEffects(t *testing.T) {
	writeManifest(t, `credentials{id,type,source,required_for}:
  git-ssh,ssh_key,env:AGENT_SSH,
`)
	var stdout bytes.Buffer
	if code := Run([]string{"credential", "inspect", "--bogus"}, &stdout); code != 2 {
		t.Fatalf("Run() exit = %d, want 2", code)
	}
	for _, expected := range []string{"error:", "category: usage", "Unknown flag"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("output = %q, want to contain %q", stdout.String(), expected)
		}
	}
}

func TestCredentialUnknownSubcommand(t *testing.T) {
	writeManifest(t, "")
	var stdout bytes.Buffer
	if code := Run([]string{"credential", "frobnicate"}, &stdout); code != 2 {
		t.Fatalf("Run() exit = %d, want 2", code)
	}
	if !strings.Contains(stdout.String(), "Unknown credential command") {
		t.Errorf("output = %q, want unknown subcommand error", stdout.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("writer failed")
}

func TestCredentialDoctorPropagatesOutputFailure(t *testing.T) {
	t.Setenv(credential.ConfigEnv, filepath.Join(t.TempDir(), "missing.toon"))
	if code := Run([]string{"credential", "doctor"}, failingWriter{}); code != 1 {
		t.Fatalf("Run() exit = %d, want 1 when output fails", code)
	}
}

func TestCredentialUsagePropagatesOutputFailure(t *testing.T) {
	if code := Run([]string{"credential", "list", "--unexpected"}, failingWriter{}); code != 1 {
		t.Fatalf("Run() exit = %d, want 1 when usage output fails", code)
	}
}

func TestCredentialMalformedManifestRedactsUntrustedContent(t *testing.T) {
	secret := "manifest-secret-value"
	writeManifest(t, "credentials[1]{id,type,source,required_for}:\n  \""+secret+"\n")
	var stdout bytes.Buffer
	if code := Run([]string{"credential", "list"}, &stdout); code != 1 {
		t.Fatalf("Run() exit = %d, want 1", code)
	}
	if strings.Contains(stdout.String(), secret) {
		t.Fatalf("output leaked malformed manifest content")
	}
}

func TestCredentialConfiguredManifestPathIsNotLeaked(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "manifest-secret-path")
	if err := os.Mkdir(secretPath, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(credential.ConfigEnv, secretPath)
	var stdout bytes.Buffer
	if code := Run([]string{"credential", "list"}, &stdout); code != 1 {
		t.Fatalf("Run() exit = %d, want 1", code)
	}
	if strings.Contains(stdout.String(), secretPath) || strings.Contains(stdout.String(), "manifest-secret-path") {
		t.Fatalf("configured manifest path leaked")
	}
	if !strings.Contains(stdout.String(), "configured credential manifest") {
		t.Fatalf("output did not use generic credential-manifest recovery")
	}
}
