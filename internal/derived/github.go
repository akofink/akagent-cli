package derived

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/akofink/akagent-cli/internal/store"
)

type githubResult struct{ PR, Checks Finding }

type pull struct {
	Number   int     `json:"number"`
	State    string  `json:"state"`
	MergedAt *string `json:"merged_at"`
	HTMLURL  string  `json:"html_url"`
	Head     struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
}

type checkRuns struct {
	TotalCount int `json:"total_count"`
	CheckRuns  []struct {
		Name       string  `json:"name"`
		Status     string  `json:"status"`
		Conclusion *string `json:"conclusion"`
	} `json:"check_runs"`
}

type combinedStatus struct {
	State    string `json:"state"`
	Statuses []struct {
		Context string `json:"context"`
		State   string `json:"state"`
	} `json:"statuses"`
}

func checkGitHub(ctx context.Context, runner Runner, resource store.Resource, git gitResult) githubResult {
	result := githubResult{PR: finding("pull_request", StateUnknown, "github_unavailable", "Verify the pull request through the forge"), Checks: finding("checks", StateUnknown, "pr_unverified", "Verify the PR head and checks")}
	owner, repo, ok := githubRemote(git.Remote)
	if !ok {
		result.PR.Code = "unsupported_remote"
		return result
	}
	if resource.Branch == "" {
		result.PR.Code = "binding_missing"
		return result
	}
	endpoint := "/repos/" + owner + "/" + repo + "/pulls?state=all&head=" + url.QueryEscape(owner+":"+resource.Branch) + "&per_page=100"
	var pulls []pull
	if !apiJSON(ctx, runner, endpoint, &pulls) {
		return result
	}
	if len(pulls) == 100 {
		result.PR.Code = "result_truncated"
		return result
	}
	matches := make([]pull, 0)
	for _, pr := range pulls {
		if pr.Head.Ref == resource.Branch && pr.Head.Repo.FullName == owner+"/"+repo {
			matches = append(matches, pr)
		}
	}
	// A recorded PR URL is a hint, not proof: it only chooses among several PRs on the bound branch.
	linked := map[int]bool{}
	pullLinks := 0
	for _, raw := range resource.ExternalURLs {
		if number, valid := githubPRURL(raw, owner, repo); valid {
			linked[number] = true
			pullLinks++
		} else if strings.Contains(raw, "/pull/") {
			pullLinks++
		}
	}
	if len(matches) > 1 {
		chosen := matches[:0]
		for _, pr := range matches {
			if linked[pr.Number] {
				chosen = append(chosen, pr)
			}
		}
		matches = chosen
		if len(matches) != 1 {
			result.PR.Code = "ambiguous_pulls"
			return result
		}
	}
	if len(matches) == 0 {
		if pullLinks > 0 {
			result.PR = finding("pull_request", StateStale, "link_mismatch", "Verify the recorded PR URL against the bound repository and branch")
		} else {
			result.PR = finding("pull_request", StateMissing, "not_found", "Publish a PR only when the branch is ready")
		}
		return result
	}
	pr := matches[0]
	result.PR = finding("pull_request", StateCurrent, "open", "No PR action needed")
	result.PR.Detail = pr.HTMLURL
	if pr.MergedAt != nil {
		result.PR.Code = "merged"
	} else if pr.State != "open" {
		result.PR = finding("pull_request", StateStale, "closed", "Verify whether the closed PR completes the task")
		result.PR.Detail = pr.HTMLURL
	}
	if !commitID(pr.Head.SHA) {
		result.Checks.Code = "head_missing"
		return result
	}
	if git.Head != "" && git.Head != pr.Head.SHA {
		result.Checks = finding("checks", StateStale, "head_mismatch", "Compare the local and published heads before trusting checks")
		return result
	}
	base := "/repos/" + owner + "/" + repo + "/commits/" + pr.Head.SHA
	var runs checkRuns
	if !apiJSON(ctx, runner, base+"/check-runs?per_page=100", &runs) {
		result.Checks.Code = "checks_unavailable"
		return result
	}
	if runs.TotalCount > len(runs.CheckRuns) {
		result.Checks.Code = "checks_truncated"
		return result
	}
	var status combinedStatus
	if !apiJSON(ctx, runner, "/repos/"+owner+"/"+repo+"/commits/"+pr.Head.SHA+"/status", &status) {
		result.Checks.Code = "statuses_unavailable"
		return result
	}
	if len(runs.CheckRuns) == 0 && len(status.Statuses) == 0 {
		result.Checks = finding("checks", StateMissing, "none_reported", "Verify the repository's required check on this head")
		return result
	}
	var pending, failed []string
	for _, check := range runs.CheckRuns {
		switch {
		case check.Status != "completed" || check.Conclusion == nil:
			pending = append(pending, check.Name)
		case *check.Conclusion != "success" && *check.Conclusion != "neutral" && *check.Conclusion != "skipped":
			failed = append(failed, check.Name)
		}
	}
	for _, entry := range status.Statuses {
		switch entry.State {
		case "success":
		case "pending":
			pending = append(pending, entry.Context)
		default:
			failed = append(failed, entry.Context)
		}
	}
	if len(failed) > 0 {
		result.Checks = finding("checks", StateStale, "failed", "Investigate failed checks on this head")
		result.Checks.Detail = checkNames(failed)
		return result
	}
	if len(pending) > 0 {
		result.Checks = finding("checks", StateStale, "pending", "Wait for checks on this head")
		result.Checks.Detail = checkNames(pending)
		return result
	}
	result.Checks = finding("checks", StateCurrent, "reported_success", "Verify the named required check and delivery contract")
	return result
}

func checkNames(names []string) string {
	sort.Strings(names)
	unique := names[:0]
	for i, name := range names {
		if i == 0 || name != names[i-1] {
			unique = append(unique, name)
		}
	}
	return strings.Join(unique, ",")
}

func apiJSON(ctx context.Context, runner Runner, path string, target any) bool {
	data, err := runner.Run(ctx, "gh", "api", "--method", "GET", path)
	return err == nil && json.Unmarshal(data, target) == nil
}

func githubRemote(remote string) (string, string, bool) {
	var path string
	if strings.HasPrefix(remote, "git@github.com:") {
		path = strings.TrimPrefix(remote, "git@github.com:")
	} else {
		u, err := url.Parse(remote)
		if err != nil || u.Hostname() != "github.com" || (u.Scheme != "https" && u.Scheme != "ssh") || u.User == nil && u.Scheme == "ssh" || u.RawQuery != "" || u.Fragment != "" || u.User != nil && u.Scheme == "https" {
			return "", "", false
		}
		path = strings.TrimPrefix(u.Path, "/")
	}
	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || !safeSlug(parts[0]) || !safeSlug(parts[1]) {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func safeSlug(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, ch := range value {
		if ch < 'a' || ch > 'z' {
			if ch < 'A' || ch > 'Z' {
				if ch < '0' || ch > '9' {
					if ch != '-' && ch != '_' && ch != '.' {
						return false
					}
				}
			}
		}
	}
	return true
}

func githubPRURL(raw, owner, repo string) (int, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return 0, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != owner || parts[1] != repo || parts[2] != "pull" {
		return 0, false
	}
	n, err := strconv.Atoi(parts[3])
	return n, err == nil && n > 0
}

func commitID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}
