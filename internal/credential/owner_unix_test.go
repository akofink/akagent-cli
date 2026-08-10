//go:build unix

package credential

import (
	"os"
	"syscall"
	"testing"
)

func TestCheckFileUnsafeOwner(t *testing.T) {
	files := map[string]os.FileInfo{
		"/":        fakeDir(0o700),
		"/secrets": fakeInfo{mode: 0o700 | os.ModeDir, sys: &syscall.Stat_t{Uid: 1000, Gid: 1000}},
		"/secrets/git": fakeInfo{
			mode: 0o600,
			sys:  &syscall.Stat_t{Uid: uint32(os.Geteuid() + 1), Gid: 1000},
		},
	}
	c := fileChecker(files, func(string) string { return "" })
	got := c.Check(Entry{ID: "git", Source: "file:/secrets/git"})
	if got.Status != Unsafe {
		t.Fatalf("status = %q, want unsafe (%s)", got.Status, got.Reason)
	}
	if got.Reason == "" {
		t.Fatal("expected a non-empty unsafe reason")
	}
}

func TestCheckFileOwnedByCurrentUser(t *testing.T) {
	files := map[string]os.FileInfo{
		"/":        fakeDir(0o700),
		"/secrets": fakeInfo{mode: 0o700 | os.ModeDir, sys: &syscall.Stat_t{Uid: uint32(os.Geteuid()), Gid: 1000}},
		"/secrets/git": fakeInfo{
			mode: 0o600,
			sys:  &syscall.Stat_t{Uid: uint32(os.Geteuid()), Gid: 1000},
		},
	}
	c := fileChecker(files, func(string) string { return "" })
	got := c.Check(Entry{ID: "git", Source: "file:/secrets/git"})
	if got.Status != Ready {
		t.Fatalf("status = %q, want ready (%s)", got.Status, got.Reason)
	}
}
