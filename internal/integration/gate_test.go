package integration

import "testing"

func TestInspectValue(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		set     bool
		enabled bool
		reason  string
	}{
		{name: "unset", enabled: false, reason: "AKAGENT_ENABLED is unset"},
		{name: "empty", value: "", set: true, enabled: false, reason: "AKAGENT_ENABLED is set to a value other than 1"},
		{name: "other value", value: "true", set: true, enabled: false, reason: "AKAGENT_ENABLED is set to a value other than 1"},
		{name: "one", value: "1", set: true, enabled: true, reason: "AKAGENT_ENABLED is set to 1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status := InspectValue(test.value, test.set)
			if status.Enabled != test.enabled {
				t.Fatalf("InspectValue() enabled = %t, want %t", status.Enabled, test.enabled)
			}
			if status.Signal != EnableEnv {
				t.Errorf("InspectValue() signal = %q, want %q", status.Signal, EnableEnv)
			}
			if status.Reason != test.reason {
				t.Errorf("InspectValue() reason = %q, want %q", status.Reason, test.reason)
			}
		})
	}
}
