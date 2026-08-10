//go:build !unix

package credential

import (
	"os"
	"testing"
)

func TestFileSafetyNonUnixStillRequiresExactPermissions(t *testing.T) {
	for _, mode := range []os.FileMode{0o000, 0o200, 0o644} {
		if reason := fileSafety(fakeInfo{mode: mode}, true); reason == "" {
			t.Errorf("mode %04o was accepted on non-Unix policy", mode.Perm())
		}
	}
	if reason := fileSafety(fakeInfo{mode: 0o600}, true); reason != "" {
		t.Errorf("mode 0600 rejected on non-Unix policy: %s", reason)
	}
	if reason := fileSafety(fakeDir(0o700), false); reason != "" {
		t.Errorf("mode 0700 rejected on non-Unix policy: %s", reason)
	}
	checker := &Checker{
		LookupEnv: func(string) string { return "" },
		Stat:      func(string) (os.FileInfo, error) { return fakeInfo{mode: 0o600}, nil },
	}
	if got := checker.Check(Entry{ID: "file", Source: "file:C:/credential"}); got.Status != Unsupported {
		t.Errorf("non-Unix file status = %q, want unsupported", got.Status)
	}
}
