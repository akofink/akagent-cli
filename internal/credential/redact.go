package credential

import "strings"

// Redact masks occurrences of known secret values in a message so no value can
// leak through warnings, errors, or diagnostics. It is defense in depth: the
// credential package never reads source values, so callers normally have no
// secrets to pass. Empty secrets are ignored.
func Redact(message string, secrets ...string) string {
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		message = strings.ReplaceAll(message, secret, "[redacted]")
	}
	return message
}
