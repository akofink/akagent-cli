package derived

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

type gitOnlyRunner struct {
	t    *testing.T
	deny string
}

func (r gitOnlyRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name != "git" {
		r.t.Fatalf("non-PR contract contacted %s", name)
	}
	if r.deny != "" && strings.Contains(strings.Join(args, " "), r.deny) {
		return nil, errors.New("unavailable")
	}
	return (CommandRunner{}).Run(ctx, name, args...)
}

func cloneFixture(t *testing.T) (Snapshot, string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	root := t.TempDir()
	primary, clone, work := filepath.Join(root, "primary"), filepath.Join(root, "alternate"), filepath.Join(root, "work")
	git := func(path string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", path}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture git %v: %v %s", args, err, out)
		}
	}
	if err := os.Mkdir(primary, 0700); err != nil {
		t.Fatal(err)
	}
	git(primary, "init", "-b", "main")
	git(primary, "config", "user.name", "Fixture")
	git(primary, "config", "user.email", "fixture@example.invalid")
	git(primary, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "seed")
	git(primary, "clone", primary, clone)
	git(primary, "remote", "add", "origin", "https://github.com/example/demo.git")
	git(clone, "remote", "set-url", "origin", "git@github.com:example/demo.git")
	git(clone, "worktree", "add", "-b", "task", work)
	snapshot := Snapshot{TaskID: "fixture", Task: store.Manifest{Lifecycle: "created", Condition: "active"},
		Resources:      []store.Resource{{ID: "source", Repository: "demo", Branch: "task", WorktreePath: work, Metadata: map[string]string{"delivery_contract": "local_publication"}}},
		RepositoryPath: func(string) string { return primary }}
	return snapshot, clone, work
}

func TestOwningCloneAndLocalPublication(t *testing.T) {
	snap, _, work := cloneFixture(t)
	runner := gitOnlyRunner{t: t}
	findings := Check(snap, runner, Options{}).Resources[0].Findings
	expect(t, findings[0], StateCurrent, "registered")
	expect(t, findings[1], StateCurrent, "local_ref")
	expect(t, findings[2], StateCurrent, "published_cached")
	expect(t, findings[3], StateCurrent, "not_applicable")
	for _, ref := range []string{"refs/remotes/origin/absent", "refs/remotes/origin/main@{0}"} {
		snap.Resources[0].Metadata["publication_ref"] = ref
		expect(t, Check(snap, runner, Options{}).Resources[0].Findings[2], StateUnknown, "publication_unverified")
	}
	delete(snap.Resources[0].Metadata, "publication_ref")
	cmd := exec.Command("git", "-C", work, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "new")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v %s", err, out)
	}
	expect(t, Check(snap, runner, Options{}).Resources[0].Findings[2], StateCurrent, "unpublished")
	snap.Task.Lifecycle = "finished"
	expect(t, Check(snap, runner, Options{}).Resources[0].Findings[2], StateStale, "unpublished_after_finish")
	snap.Task.Lifecycle = "created"
	if err := os.WriteFile(filepath.Join(work, "dirty"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	expect(t, Check(snap, runner, Options{}).Resources[0].Findings[0], StateStale, "dirty")
}

func TestOwningCloneRefusesWrongIdentityAndRetainsUnknownMissing(t *testing.T) {
	snap, clone, work := cloneFixture(t)
	blocked := Check(snap, gitOnlyRunner{t: t, deny: "--git-common-dir"}, Options{}).Resources[0].Findings
	expect(t, blocked[0], StateMissing, "not_registered")
	expect(t, blocked[2], StateUnknown, "publication_unverified")
	blocked = Check(snap, gitOnlyRunner{t: t, deny: "merge-base"}, Options{}).Resources[0].Findings
	expect(t, blocked[2], StateUnknown, "publication_unverified")
	if out, err := exec.Command("git", "-C", clone, "remote", "set-url", "origin", "https://github.com/other/demo.git").CombinedOutput(); err != nil {
		t.Fatalf("remote: %v %s", err, out)
	}
	findings := Check(snap, gitOnlyRunner{t: t}, Options{}).Resources[0].Findings
	expect(t, findings[0], StateMissing, "not_registered")
	expect(t, findings[1], StateMissing, "local_ref_missing")
	snap.Resources[0].WorktreePath = filepath.Join(work, "absent")
	findings = Check(snap, gitOnlyRunner{t: t}, Options{}).Resources[0].Findings
	expect(t, findings[0], StateMissing, "not_registered")
}

func TestExplicitNonPRContractOnly(t *testing.T) {
	snap, runner := fixture(t)
	snap.Resources[0].Metadata = map[string]string{"delivery_contract": "local_only"}
	findings := Check(snap, gitOnlyRunner{t: t, deny: "anything"}, Options{}).Resources[0].Findings
	expect(t, findings[2], StateCurrent, "not_applicable")
	snap.Resources[0].ExternalURLs = []string{"https://github.com/example/demo/pull/1"}
	expect(t, Check(snap, gitOnlyRunner{t: t}, Options{}).Resources[0].Findings[2], StateStale, "contract_conflict")
	snap.Resources[0].ExternalURLs = nil
	snap.Resources[0].Metadata = map[string]string{"delivery_contract": "typo"}
	runner.pulls = "[]"
	expect(t, resourceFindings(t, snap, runner)[2], StateMissing, "not_found")
}
