package credential

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeInfo struct {
	mode os.FileMode
	sys  any
}

func (f fakeInfo) Name() string       { return "" }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() os.FileMode  { return f.mode }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeInfo) Sys() any           { return f.sys }
func (f fakeInfo) String() string     { return "fakeInfo" }

func fakeDir(mode os.FileMode) fakeInfo { return fakeInfo{mode: mode | os.ModeDir} }

func statIndex(files map[string]os.FileInfo) FileInfo {
	return func(path string) (os.FileInfo, error) {
		if info, ok := files[path]; ok {
			return info, nil
		}
		return nil, os.ErrNotExist
	}
}

func fileChecker(files map[string]os.FileInfo, env EnvLookup) *Checker {
	return &Checker{Stat: statIndex(files), LookupEnv: env}
}

func TestCheckFileReady(t *testing.T) {
	c := fileChecker(map[string]os.FileInfo{
		"/":            fakeDir(0o700),
		"/secrets":     fakeDir(0o700),
		"/secrets/git": fakeInfo{mode: 0o600},
	}, func(string) string { return "" })

	got := c.Check(Entry{ID: "git", Source: "file:/secrets/git", RequiredFor: "git"})
	if got.Status != Ready {
		t.Fatalf("status = %q, want ready (%s)", got.Status, got.Reason)
	}
}

func TestCheckFileMissing(t *testing.T) {
	c := fileChecker(map[string]os.FileInfo{}, func(string) string { return "" })
	got := c.Check(Entry{ID: "git", Source: "file:/secrets/git"})
	if got.Status != Missing {
		t.Fatalf("status = %q, want missing", got.Status)
	}
}

func TestCheckFileUnsafeMode(t *testing.T) {
	c := fileChecker(map[string]os.FileInfo{
		"/":            fakeDir(0o700),
		"/secrets":     fakeDir(0o700),
		"/secrets/git": fakeInfo{mode: 0o644},
	}, func(string) string { return "" })

	got := c.Check(Entry{ID: "git", Source: "file:/secrets/git"})
	if got.Status != Unsafe {
		t.Fatalf("status = %q, want unsafe (%s)", got.Status, got.Reason)
	}
}

func TestCheckFileUnsafeDirectoryMode(t *testing.T) {
	c := fileChecker(map[string]os.FileInfo{
		"/":            fakeDir(0o700),
		"/secrets":     fakeDir(0o755),
		"/secrets/git": fakeInfo{mode: 0o600},
	}, func(string) string { return "" })

	got := c.Check(Entry{ID: "git", Source: "file:/secrets/git"})
	if got.Status != Unsafe {
		t.Fatalf("status = %q, want unsafe (%s)", got.Status, got.Reason)
	}
}

func TestCheckFileSourceIsDirectory(t *testing.T) {
	c := fileChecker(map[string]os.FileInfo{
		"/":            fakeDir(0o700),
		"/secrets":     fakeDir(0o700),
		"/secrets/git": fakeDir(0o700),
	}, func(string) string { return "" })

	got := c.Check(Entry{ID: "git", Source: "file:/secrets/git"})
	if got.Status != Unsafe {
		t.Fatalf("status = %q, want unsafe (%s)", got.Status, got.Reason)
	}
}

func TestCheckEnvReady(t *testing.T) {
	c := fileChecker(map[string]os.FileInfo{}, func(name string) string {
		if name == "GITHUB_TOKEN" {
			return "present"
		}
		return ""
	})
	got := c.Check(Entry{ID: "gh", Type: "api_token", Source: "env:GITHUB_TOKEN"})
	if got.Status != Ready {
		t.Fatalf("status = %q, want ready (%s)", got.Status, got.Reason)
	}
}

func TestCheckEnvUnavailable(t *testing.T) {
	c := fileChecker(map[string]os.FileInfo{}, func(string) string { return "" })
	got := c.Check(Entry{ID: "gh", Type: "api_token", Source: "env:GITHUB_TOKEN"})
	if got.Status != Unavailable {
		t.Fatalf("status = %q, want unavailable", got.Status)
	}
}

func TestCheckUnsupportedKind(t *testing.T) {
	c := fileChecker(map[string]os.FileInfo{}, func(string) string { return "" })
	got := c.Check(Entry{ID: "sign", Source: "gpg:SUBKEY"})
	if got.Status != Unavailable {
		t.Fatalf("status = %q, want unavailable", got.Status)
	}
}

func TestDoctorEvaluatesAllEntries(t *testing.T) {
	c := fileChecker(map[string]os.FileInfo{
		"/":            fakeDir(0o700),
		"/secrets":     fakeDir(0o700),
		"/secrets/git": fakeInfo{mode: 0o600},
	}, func(name string) string {
		if name == "LIVE" {
			return "x"
		}
		return ""
	})
	manifest := &Manifest{Entries: []Entry{
		{ID: "git-ssh", Source: "file:/secrets/git", RequiredFor: "git"},
		{ID: "gh", Source: "env:LIVE"},
		{ID: "oa", Source: "env:GONE"},
	}}
	checks := Doctor(manifest, c)
	byID := map[string]Status{}
	for _, check := range checks {
		byID[check.Entry.ID] = check.Status
	}
	if byID["git-ssh"] != Ready || byID["gh"] != Ready || byID["oa"] != Unavailable {
		t.Fatalf("checks = %+v", checks)
	}
}

func TestDoctorEmptyManifest(t *testing.T) {
	c := fileChecker(map[string]os.FileInfo{}, func(string) string { return "" })
	checks := Doctor(&Manifest{}, c)
	if len(checks) != 0 {
		t.Fatalf("len(checks) = %d, want 0", len(checks))
	}
}

func TestCheckFileRejectsOwnerOnlyInsufficientModes(t *testing.T) {
	for _, mode := range []os.FileMode{0o000, 0o200, 0o400, 0o600 | os.ModeSetuid} {
		c := fileChecker(map[string]os.FileInfo{
			"/":            fakeDir(0o700),
			"/secrets":     fakeDir(0o700),
			"/secrets/git": fakeInfo{mode: mode},
		}, func(string) string { return "" })
		got := c.Check(Entry{ID: "git", Source: "file:/secrets/git"})
		if got.Status != Unsafe {
			t.Errorf("mode %04o status = %q, want unsafe (%s)", mode.Perm(), got.Status, got.Reason)
		}
	}
}

func TestCheckFileExpandsHomeReference(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".local", "share", "akagent", "credentials", "git")
	// The checker performs a file stat followed by a parent directory stat.
	// Track all calls separately below and verify the expanded file path.
	calls := []string{}
	c := &Checker{
		LookupEnv: func(string) string { return "" },
		Stat: func(path string) (os.FileInfo, error) {
			calls = append(calls, path)
			if path == filepath.Join(home, ".local", "share", "akagent", "credentials") {
				return fakeDir(0o700), nil
			}
			if path == filepath.Join(home, ".local", "share", "akagent", "credentials", "git") {
				return fakeInfo{mode: 0o600}, nil
			}
			return fakeDir(0o700), nil
		},
	}
	got := c.Check(Entry{ID: "git", Source: "file:~/.local/share/akagent/credentials/git"})
	if got.Status != Ready {
		t.Fatalf("status = %q, want ready (%s), calls=%v", got.Status, got.Reason, calls)
	}
	if len(calls) < 1 || calls[0] != path {
		t.Fatalf("stat calls = %v, want first call %q", calls, path)
	}
}

func TestCheckFileRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	got := NewChecker().Check(Entry{ID: "git", Source: "file:" + link})
	if got.Status != Unsafe {
		t.Fatalf("status = %q, want unsafe (%s)", got.Status, got.Reason)
	}
}

func TestCheckFileParentInspectionFailureIsUnavailable(t *testing.T) {
	c := &Checker{
		LookupEnv: func(string) string { return "" },
		Stat: func(path string) (os.FileInfo, error) {
			if path == "/secrets/git" {
				return fakeInfo{mode: 0o600}, nil
			}
			return nil, os.ErrPermission
		},
	}
	got := c.Check(Entry{ID: "git", Source: "file:/secrets/git"})
	if got.Status != Unavailable {
		t.Fatalf("status = %q, want unavailable (%s)", got.Status, got.Reason)
	}
}

func TestCheckFileRejectsSymlinkParent(t *testing.T) {
	c := &Checker{
		LookupEnv: func(string) string { return "" },
		Stat: func(path string) (os.FileInfo, error) {
			if path == "/secrets/git" {
				return fakeInfo{mode: 0o600}, nil
			}
			if path == "/secrets" {
				return fakeInfo{mode: os.ModeSymlink | 0o777}, nil
			}
			return nil, os.ErrNotExist
		},
	}
	got := c.Check(Entry{ID: "git", Source: "file:/secrets/git"})
	if got.Status != Unsafe {
		t.Fatalf("status = %q, want unsafe (%s)", got.Status, got.Reason)
	}
}

func TestCheckFileRejectsHigherAncestorSymlink(t *testing.T) {
	c := &Checker{
		LookupEnv: func(string) string { return "" },
		Stat: func(path string) (os.FileInfo, error) {
			switch path {
			case "/root/redirect/secrets/git":
				return fakeInfo{mode: 0o600}, nil
			case "/root/redirect/secrets":
				return fakeDir(0o700), nil
			case "/root/redirect":
				return fakeInfo{mode: os.ModeSymlink | 0o777}, nil
			case "/root":
				return fakeDir(0o700), nil
			case "/":
				return fakeDir(0o700), nil
			default:
				return nil, os.ErrNotExist
			}
		},
	}
	got := c.Check(Entry{ID: "git", Source: "file:/root/redirect/secrets/git"})
	if got.Status != Unsafe {
		t.Fatalf("status = %q, want unsafe (%s)", got.Status, got.Reason)
	}
}
