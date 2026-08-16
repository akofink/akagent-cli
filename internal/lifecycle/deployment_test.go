package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akofink/akagent-cli/internal/credential"
)

func TestDeploymentReadinessIsCheckedAtLaunchWithoutPersistingValues(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "deploy-ready", Title: "Deployment"}); err != nil {
		t.Fatal(err)
	}
	value := filepath.Join(t.TempDir(), "runtime-value")
	manager.Credentials = func() (*credential.Manifest, error) {
		return &credential.Manifest{Version: 1, Entries: []credential.Entry{{ID: "deploy-token", Type: "api_token", Source: "env:DEPLOY_TOKEN"}}}, nil
	}
	manager.Checker = &credential.Checker{LookupEnv: func(string) string { return "" }}
	execution, _, err := manager.CreateDeployment("deploy-ready", ExecutionRequest{
		ID:           "attempt-one",
		Command:      "/bin/true",
		Requirements: []string{"deploy-token"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if execution.Requirements != "deploy-token" {
		t.Fatalf("requirements = %q, want metadata-only credential ID", execution.Requirements)
	}
	if _, err := manager.LaunchExecutionRecord("deploy-ready", execution.ID); err == nil || !strings.Contains(err.Error(), "deploy-token") || strings.Contains(err.Error(), value) {
		t.Fatalf("launch error = %v, want redaction-safe readiness failure", err)
	}
	stored, err := manager.InspectExecution("deploy-ready", execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Lifecycle != "created" || stored.Condition != "blocked" || stored.Reason != "deployment credential readiness failed" {
		t.Fatalf("stored deployment readiness = %#v, want blocked and retryable metadata", stored)
	}
}

func TestRunDeploymentInjectsReadyEnvironmentAndRecordsGenericResult(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "deploy-run", Title: "Deployment"}); err != nil {
		t.Fatal(err)
	}
	value := filepath.Join(t.TempDir(), "runtime-value")
	t.Setenv("DEPLOY_TOKEN", value)
	manager.Credentials = func() (*credential.Manifest, error) {
		return &credential.Manifest{Version: 1, Entries: []credential.Entry{{ID: "deploy-token", Type: "api_token", Source: "env:DEPLOY_TOKEN"}}}, nil
	}
	script := filepath.Join(t.TempDir(), "deployment-check")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntest -n \"$DEPLOY_TOKEN\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	execution, _, err := manager.CreateDeployment("deploy-run", ExecutionRequest{ID: "attempt-one", Command: script, Requirements: []string{"deploy-token"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.RunDeployment("deploy-run", execution.ID); err != nil {
		t.Fatal(err)
	}
	completed, err := manager.InspectExecution("deploy-run", execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Lifecycle != "finished" || completed.Condition != "succeeded" || completed.Result != "deployment succeeded" {
		t.Fatalf("completed deployment = %#v, want generic success result", completed)
	}
	if strings.Contains(completed.Result, value) {
		t.Fatal("deployment result leaked the credential value")
	}
}

func TestRunDeploymentRecordsFailureWithoutCommandError(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "deploy-fail", Title: "Deployment"}); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "deployment-fail")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	execution, _, err := manager.CreateDeployment("deploy-fail", ExecutionRequest{ID: "attempt-one", Command: script})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.RunDeployment("deploy-fail", execution.ID); err == nil || err.Error() != "deployment command failed" {
		t.Fatalf("RunDeployment() error = %v, want generic command failure", err)
	}
	completed, err := manager.InspectExecution("deploy-fail", execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Lifecycle != "finished" || completed.Condition != "failed" || completed.Result != "deployment failed" {
		t.Fatalf("failed deployment = %#v, want generic failure result", completed)
	}
	if _, err := manager.Store.ReadExecutionEvents("deploy-fail", "attempt-one"); err != nil {
		t.Fatal(err)
	}
}
