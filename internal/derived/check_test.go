package derived

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

type fixtureRunner struct {
	path, branch, sha, origin, status string
	pulls, runs, statuses             string
	unavailable                       string
	calls                             *int
}

func (f fixtureRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	if f.calls != nil {
		*f.calls++
	}
	key := name + " " + strings.Join(args, " ")
	if f.unavailable != "" && strings.Contains(key, f.unavailable) {
		return nil, errors.New("private command failure")
	}
	if name == "gh" {
		path := args[len(args)-1]
		switch {
		case strings.Contains(path, "/pulls?"):
			return []byte(f.pulls), nil
		case strings.Contains(path, "/check-runs?"):
			return []byte(f.runs), nil
		case strings.HasSuffix(path, "/status"):
			return []byte(f.statuses), nil
		}
		return nil, errors.New("unknown api")
	}
	switch {
	case strings.HasSuffix(key, "rev-parse --show-toplevel"):
		return []byte(f.path + "\n"), nil
	case strings.HasSuffix(key, "config --get remote.origin.url"):
		return []byte(f.origin), nil
	case strings.HasSuffix(key, "worktree list --porcelain"):
		return []byte("worktree " + f.path + "\nHEAD " + f.sha + "\nbranch refs/heads/" + f.branch + "\n"), nil
	case strings.HasSuffix(key, "branch --show-current"):
		return []byte(f.branch + "\n"), nil
	case strings.Contains(key, "rev-parse --verify --quiet refs/heads/"):
		if strings.Contains(key, "refs/heads/"+f.branch+"^{commit}") {
			return []byte(f.sha + "\n"), nil
		}
		return nil, errors.New("missing ref")
	case strings.Contains(key, "status --porcelain=v1"):
		return []byte(f.status), nil
	}
	return nil, errors.New("unexpected git call")
}

var headSHA = strings.Repeat("a", 40)

func pullJSON(number int, state, merged, branch, sha string) string {
	mergedAt := "null"
	if merged != "" {
		mergedAt = `"` + merged + `"`
	}
	return `{"number":` + strconv.Itoa(number) + `,"state":"` + state + `","merged_at":` + mergedAt + `,"html_url":"https://github.com/akofink/demo/pull/` + strconv.Itoa(number) + `","head":{"ref":"` + branch + `","sha":"` + sha + `","repo":{"full_name":"akofink/demo"}}}`
}

func fixture(t *testing.T) (Snapshot, fixtureRunner) {
	t.Helper()
	dir := t.TempDir()
	resource := store.Resource{ID: "source", Repository: "demo", Branch: "agent/feature", WorktreePath: dir}
	runner := fixtureRunner{
		path: dir, branch: resource.Branch, sha: headSHA, origin: "https://github.com/akofink/demo.git",
		pulls:    "[" + pullJSON(7, "open", "", resource.Branch, headSHA) + "]",
		runs:     `{"total_count":1,"check_runs":[{"name":"test","status":"completed","conclusion":"success"}]}`,
		statuses: `{"state":"pending","statuses":[]}`,
	}
	snapshot := Snapshot{
		TaskID:         "work",
		Task:           store.Manifest{Lifecycle: "created", Condition: "active"},
		Resources:      []store.Resource{resource},
		RepositoryPath: func(string) string { return dir },
	}
	return snapshot, runner
}

func resourceFindings(t *testing.T, snapshot Snapshot, runner fixtureRunner) []Finding {
	t.Helper()
	report := Check(snapshot, runner, Options{})
	if len(report.Resources) != 1 || len(report.Resources[0].Findings) != 4 {
		t.Fatalf("unexpected report: %+v", report)
	}
	return report.Resources[0].Findings
}

func expect(t *testing.T, got Finding, state, code string) {
	t.Helper()
	if got.State != state || got.Code != code {
		t.Fatalf("expected %s %s, got %+v", state, code, got)
	}
}

func TestCheckAllCurrent(t *testing.T) {
	snapshot, runner := fixture(t)
	report := Check(snapshot, runner, Options{})
	for _, f := range report.Resources[0].Findings {
		if f.State != StateCurrent {
			t.Fatalf("expected current, got %+v", report)
		}
	}
	got := report.Resources[0].Findings
	if got[1].Detail != headSHA || got[2].Detail != "https://github.com/akofink/demo/pull/7" {
		t.Fatalf("details: %+v", got)
	}
	if report.Summary != (Summary{Current: 4}) || len(report.Task) != 0 {
		t.Fatalf("summary: %+v", report)
	}
}

func TestCheckWorktree(t *testing.T) {
	snapshot, runner := fixture(t)
	runner.status = " M a.go\x00R  new.go\x00old.go\x00?? untracked\x00"
	got := resourceFindings(t, snapshot, runner)[0]
	expect(t, got, StateStale, "dirty")
	if got.Detail != "3 changed paths" {
		t.Fatalf("dirty detail: %+v", got)
	}
	runner.status = ""
	runner.branch = "other"
	expect(t, resourceFindings(t, snapshot, runner)[0], StateStale, "branch_mismatch")
	runner.branch = snapshot.Resources[0].Branch

	missing := snapshot
	missing.Resources = []store.Resource{snapshot.Resources[0]}
	missing.Resources[0].WorktreePath = filepath.Join(runner.path, "missing")
	expect(t, resourceFindings(t, missing, runner)[0], StateMissing, "not_registered")

	unbound := snapshot
	unbound.Resources = []store.Resource{snapshot.Resources[0]}
	unbound.Resources[0].WorktreePath = ""
	expect(t, resourceFindings(t, unbound, runner)[0], StateUnknown, "binding_missing")

	finished := snapshot
	finished.Task.Lifecycle = "finished"
	expect(t, resourceFindings(t, finished, runner)[0], StateStale, "retained_after_finish")

	runner.unavailable = "status --porcelain"
	got = resourceFindings(t, snapshot, runner)[0]
	expect(t, got, StateUnknown, "git_unavailable")
	if strings.Contains(got.Detail+got.Action, "private") {
		t.Fatalf("leaked command failure: %+v", got)
	}
}

func TestCheckBranchAndRepository(t *testing.T) {
	snapshot, runner := fixture(t)
	runner.branch = "renamed"
	got := resourceFindings(t, snapshot, runner)
	expect(t, got[1], StateMissing, "local_ref_missing")

	snapshot, runner = fixture(t)
	snapshot.RepositoryPath = func(string) string { return "" }
	snapshot.Resources[0].WorktreePath = ""
	got = resourceFindings(t, snapshot, runner)
	expect(t, got[0], StateUnknown, "repository_unavailable")
	expect(t, got[1], StateUnknown, "repository_unavailable")

	snapshot, runner = fixture(t)
	runner.unavailable = "rev-parse --show-toplevel"
	got = resourceFindings(t, snapshot, runner)
	expect(t, got[0], StateUnknown, "repository_unavailable")
	expect(t, got[2], StateUnknown, "unsupported_remote")
}

func TestCheckPullRequest(t *testing.T) {
	snapshot, runner := fixture(t)
	branch := snapshot.Resources[0].Branch

	runner.pulls = `[]`
	expect(t, resourceFindings(t, snapshot, runner)[2], StateMissing, "not_found")

	runner.pulls = "[" + pullJSON(7, "closed", "2026-09-26T00:00:00Z", branch, headSHA) + "]"
	expect(t, resourceFindings(t, snapshot, runner)[2], StateCurrent, "merged")

	runner.pulls = "[" + pullJSON(7, "closed", "", branch, headSHA) + "]"
	expect(t, resourceFindings(t, snapshot, runner)[2], StateStale, "closed")

	runner.pulls = "[" + pullJSON(7, "open", "", branch, headSHA) + "," + pullJSON(8, "closed", "", branch, headSHA) + "]"
	expect(t, resourceFindings(t, snapshot, runner)[2], StateUnknown, "ambiguous_pulls")

	linked := snapshot
	linked.Resources = []store.Resource{snapshot.Resources[0]}
	linked.Resources[0].ExternalURLs = []string{"https://github.com/akofink/demo/pull/8"}
	got := resourceFindings(t, linked, runner)[2]
	expect(t, got, StateStale, "closed")
	if !strings.HasSuffix(got.Detail, "/pull/8") {
		t.Fatalf("link did not disambiguate: %+v", got)
	}

	// A single PR on the branch wins over an unrelated recorded link.
	runner.pulls = "[" + pullJSON(7, "open", "", branch, headSHA) + "]"
	linked.Resources[0].ExternalURLs = []string{"https://github.com/other/repo/pull/3"}
	expect(t, resourceFindings(t, linked, runner)[2], StateCurrent, "open")

	runner.pulls = `[]`
	expect(t, resourceFindings(t, linked, runner)[2], StateStale, "link_mismatch")

	runner.pulls = "[" + pullJSON(7, "open", "", "someone-else", headSHA) + "]"
	expect(t, resourceFindings(t, snapshot, runner)[2], StateMissing, "not_found")

	runner.pulls = "[" + strings.Repeat(pullJSON(1, "closed", "", "x", headSHA)+",", 99) + pullJSON(1, "closed", "", "x", headSHA) + "]"
	expect(t, resourceFindings(t, snapshot, runner)[2], StateUnknown, "result_truncated")

	runner.unavailable = "/pulls?"
	expect(t, resourceFindings(t, snapshot, runner)[2], StateUnknown, "github_unavailable")

	snapshot, runner = fixture(t)
	runner.origin = "https://example.com/akofink/demo.git"
	expect(t, resourceFindings(t, snapshot, runner)[2], StateUnknown, "unsupported_remote")
}

func TestCheckChecks(t *testing.T) {
	snapshot, runner := fixture(t)
	runner.runs = `{"total_count":2,"check_runs":[{"name":"test","status":"completed","conclusion":"failure"},{"name":"lint","status":"in_progress","conclusion":null}]}`
	got := resourceFindings(t, snapshot, runner)[3]
	expect(t, got, StateStale, "failed")
	if got.Detail != "test" {
		t.Fatalf("failed names: %+v", got)
	}
	runner.runs = `{"total_count":1,"check_runs":[{"name":"lint","status":"queued","conclusion":null}]}`
	runner.statuses = `{"state":"pending","statuses":[{"context":"deploy","state":"pending"}]}`
	got = resourceFindings(t, snapshot, runner)[3]
	expect(t, got, StateStale, "pending")
	if got.Detail != "deploy,lint" {
		t.Fatalf("pending names: %+v", got)
	}
	runner.runs = `{"total_count":0,"check_runs":[]}`
	runner.statuses = `{"state":"pending","statuses":[]}`
	expect(t, resourceFindings(t, snapshot, runner)[3], StateMissing, "none_reported")

	runner.runs = `{"total_count":5,"check_runs":[]}`
	expect(t, resourceFindings(t, snapshot, runner)[3], StateUnknown, "checks_truncated")

	snapshot, runner = fixture(t)
	runner.pulls = "[" + pullJSON(7, "open", "", snapshot.Resources[0].Branch, strings.Repeat("b", 40)) + "]"
	expect(t, resourceFindings(t, snapshot, runner)[3], StateStale, "head_mismatch")

	snapshot, runner = fixture(t)
	runner.pulls = "[" + pullJSON(7, "open", "", snapshot.Resources[0].Branch, "../../x") + "]"
	expect(t, resourceFindings(t, snapshot, runner)[3], StateUnknown, "head_missing")

	snapshot, runner = fixture(t)
	runner.unavailable = "check-runs"
	got = resourceFindings(t, snapshot, runner)[3]
	expect(t, got, StateUnknown, "checks_unavailable")
	if strings.Contains(got.Detail, "private") {
		t.Fatalf("leaked command failure: %+v", got)
	}
	runner.unavailable = "/status"
	expect(t, resourceFindings(t, snapshot, runner)[3], StateUnknown, "statuses_unavailable")
}

func TestRecordConsistency(t *testing.T) {
	snapshot, runner := fixture(t)
	snapshot.Resources = nil
	snapshot.Executions = []store.Execution{
		{ID: "z", Lifecycle: "created", Condition: "active", SessionReferences: []store.SessionReference{{Tool: "pi"}, {Tool: "claude"}, {Tool: "pi"}}},
		{ID: "a", Lifecycle: "finished"},
	}
	report := Check(snapshot, runner, Options{})
	expect(t, report.Executions[0], StateCurrent, "closed")
	expect(t, report.Executions[1], StateUnknown, "adapter_unavailable")
	if report.Executions[0].Surface != "execution:a" || report.Executions[1].Detail != "claude,pi" {
		t.Fatalf("order or tools: %+v", report.Executions)
	}

	snapshot.Task.Lifecycle = "finished"
	snapshot.Executions[0].Revision = 3
	report = Check(snapshot, runner, Options{})
	got := report.Executions[1]
	expect(t, got, StateStale, "task_terminal")
	if !strings.Contains(got.Action, "revision 3") || report.Summary.NeedsAction != 1 {
		t.Fatalf("terminal task: %+v", report)
	}

	for _, closed := range []store.Execution{
		{ID: "e", Lifecycle: "stopped"},
		{ID: "e", ExternalCompletion: &store.ExternalCompletion{}},
		{ID: "e", HandoffDisposition: &store.HandoffDisposition{}},
		{ID: "e", ArchiveState: "complete"},
	} {
		expect(t, checkExecution(closed, true), StateCurrent, "closed")
	}

	snapshot.Task.Lifecycle = "created"
	snapshot.Executions = []store.Execution{{ID: "a", Lifecycle: "finished"}}
	report = Check(snapshot, runner, Options{})
	if len(report.Task) != 1 {
		t.Fatalf("expected no open execution: %+v", report)
	}
	expect(t, report.Task[0], StateStale, "no_open_execution")
	snapshot.Task.Condition = "waiting"
	if report = Check(snapshot, runner, Options{}); len(report.Task) != 0 {
		t.Fatalf("waiting task needs no execution: %+v", report)
	}
}

func TestDeliveredUnfinished(t *testing.T) {
	snapshot, runner := fixture(t)
	runner.pulls = "[" + pullJSON(7, "closed", "2026-09-26T00:00:00Z", snapshot.Resources[0].Branch, headSHA) + "]"
	report := Check(snapshot, runner, Options{})
	if len(report.Task) != 1 {
		t.Fatalf("expected delivery finding: %+v", report)
	}
	expect(t, report.Task[0], StateStale, "delivered_unfinished")

	second := snapshot.Resources[0]
	second.ID = "docs"
	second.Branch = "agent/docs"
	snapshot.Resources = append(snapshot.Resources, second)
	if report = Check(snapshot, runner, Options{}); len(report.Task) != 0 {
		t.Fatalf("one unmerged resource must block the finding: %+v", report)
	}
}

func TestOfflineMakesNoCalls(t *testing.T) {
	snapshot, runner := fixture(t)
	calls := 0
	runner.calls = &calls
	report := Check(snapshot, runner, Options{Offline: true})
	if calls != 0 {
		t.Fatalf("offline made %d calls", calls)
	}
	for _, f := range report.Resources[0].Findings {
		expect(t, f, StateUnknown, "offline")
	}
}

func TestCheckStoreFiltersAndSkipsTerminalAdapters(t *testing.T) {
	open, runner := fixture(t)
	open.Executions = []store.Execution{{ID: "live", Lifecycle: "created", Condition: "active"}}
	done := open
	done.TaskID = "done"
	done.Task = store.Manifest{Lifecycle: "finished", Condition: "none"}
	done.Executions = []store.Execution{{ID: "leftover", Lifecycle: "created", Condition: "active"}}
	clean := done
	clean.TaskID = "clean"
	clean.Executions = []store.Execution{{ID: "closed", Lifecycle: "finished"}}
	broken := Snapshot{TaskID: "broken", Unreadable: true}

	calls := 0
	runner.calls = &calls
	report := CheckStore([]Snapshot{open, done, clean, broken}, runner, Options{})
	if report.Checked != 4 || len(report.Tasks) != 2 {
		t.Fatalf("filtered tasks: %+v", report)
	}
	if report.Tasks[0].TaskID != "broken" || report.Tasks[1].TaskID != "done" {
		t.Fatalf("order: %+v", report.Tasks)
	}
	expect(t, report.Tasks[0].Task[0], StateStale, "record_unreadable")
	if len(report.Tasks[1].Resources) != 0 || len(report.Tasks[1].Executions) != 1 {
		t.Fatalf("terminal task findings: %+v", report.Tasks[1])
	}
	expect(t, report.Tasks[1].Executions[0], StateStale, "task_terminal")
	if report.Summary.NeedsAction != 2 || report.Summary.Current != 5 || report.Summary.Unknown != 1 {
		t.Fatalf("summary: %+v", report.Summary)
	}
	if calls == 0 {
		t.Fatalf("open task skipped live adapters")
	}
}

func TestGitHubRemoteAndURLValidation(t *testing.T) {
	for _, raw := range []string{"https://github.com/x/y.git", "git@github.com:x/y.git", "ssh://git@github.com/x/y.git", "https://github.com/x/y"} {
		owner, repo, ok := githubRemote(raw)
		if !ok || owner != "x" || repo != "y" {
			t.Fatalf("remote %q: %s %s %v", raw, owner, repo, ok)
		}
	}
	for _, raw := range []string{"https://github.com.evil/x/y", "https://user:token@github.com/x/y", "https://github.com/x/y/extra", "http://github.com/x/y", "git@github.com:../y", "https://github.com/x/y?z=1"} {
		if _, _, ok := githubRemote(raw); ok {
			t.Fatalf("accepted remote %q", raw)
		}
	}
	if n, ok := githubPRURL("https://github.com/x/y/pull/12", "x", "y"); !ok || n != 12 {
		t.Fatalf("pr url: %d %v", n, ok)
	}
	for _, raw := range []string{"https://github.com/x/z/pull/12", "https://github.com/x/y/pull/0", "https://github.com/x/y/issues/12", "https://github.com/x/y/pull/12#top"} {
		if _, ok := githubPRURL(raw, "x", "y"); ok {
			t.Fatalf("accepted pr url %q", raw)
		}
	}
}

func TestStatusEntries(t *testing.T) {
	for input, want := range map[string]int{"": 0, " M a\x00": 1, "R  b\x00a\x00?? c\x00": 2, "C  b\x00a\x00": 1} {
		if got := statusEntries([]byte(input)); got != want {
			t.Fatalf("%q: got %d want %d", input, got, want)
		}
	}
}
