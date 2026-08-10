package credential

import (
	"strings"
	"testing"
)

func TestRedactMasksKnownSecrets(t *testing.T) {
	secret := "ghp_super-secret-value-1234"
	message := "credential github failed using " + secret
	got := Redact(message, secret)
	if strings.Contains(got, secret) {
		t.Fatalf("Redact() leaked the secret: %q", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("Redact() = %q, want [redacted] marker", got)
	}
}

func TestRedactIgnoresEmptySecrets(t *testing.T) {
	message := "no secrets here"
	if got := Redact(message, "", ""); got != message {
		t.Fatalf("Redact() = %q, want unchanged", got)
	}
}
