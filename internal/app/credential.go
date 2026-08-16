package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/akofink/akagent-cli/internal/credential"
	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

type credentialRow struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Source string `json:"source"`
}

type credentialListView struct {
	Credentials []credentialRow `json:"credentials"`
	Warnings    []string        `json:"warnings"`
}

type credentialInspectView struct {
	Credential credentialDetail `json:"credential"`
}

type credentialDetail struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Source      string `json:"source"`
	Reference   string `json:"reference"`
	RequiredFor string `json:"required_for"`
}

type doctorCheck struct {
	Overall     string `json:"overall"`
	Ready       int    `json:"ready"`
	Missing     int    `json:"missing"`
	Unsafe      int    `json:"unsafe"`
	Unavailable int    `json:"unavailable"`
	Unsupported int    `json:"unsupported"`
}

type doctorView struct {
	Check       doctorCheck     `json:"check"`
	Credentials []credentialRow `json:"credentials"`
	Warnings    []string        `json:"warnings"`
	Errors      []string        `json:"errors"`
}

func credentialCommand(args []string, stdout io.Writer) int {
	if len(args) == 0 {
		return writeError(stdout, "usage", "Usage: akagent credential <list|inspect|doctor|clean>", false, "Run `akagent credential doctor`")
	}

	switch args[0] {
	case "clean":
		if len(args) < 2 {
			return writeError(stdout, "usage", "Usage: akagent credential clean <task-id> [--allow-credentials]", false, "Inspect the task before authorizing credential cleanup")
		}
		options, ok := parseCredentialCleanup(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent credential clean <task-id> [--allow-credentials]", false, "Inspect the task before authorizing credential cleanup")
		}
		state, err := store.Open()
		if err != nil {
			return lifecycleError(stdout, err)
		}
		manager := lifecycle.New(state)
		manifest, err := manager.CleanCredentials(args[1], options)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(args[1], manifest)})
	case "list":
		if len(args) != 1 {
			return writeError(stdout, "usage", "Usage: akagent credential list", false, "Run `akagent credential list`")
		}
		return credentialList(stdout)
	case "inspect":
		if len(args) != 2 {
			return writeError(stdout, "usage", "Usage: akagent credential inspect <id>", false, "Run `akagent credential inspect git-ssh`")
		}
		if strings.HasPrefix(args[1], "-") {
			return writeError(stdout, "usage", "Unknown flag: "+args[1], false, "Run `akagent credential inspect <id>`")
		}
		return credentialInspect(args[1], stdout)
	case "doctor":
		if len(args) != 1 {
			return writeError(stdout, "usage", "Usage: akagent credential doctor", false, "Run `akagent credential doctor`")
		}
		return credentialDoctor(stdout)
	}

	return writeError(stdout, "usage", fmt.Sprintf("Unknown credential command: %s", args[0]), false, "Run `akagent credential --help`")
}

func credentialList(stdout io.Writer) int {
	manifest, err := loadManifest()
	if err != nil {
		return credentialError(stdout, err)
	}

	rows := make([]credentialRow, 0, len(manifest.Entries))
	var warnings []string
	for _, check := range credential.Doctor(manifest, credential.NewChecker()) {
		rows = append(rows, credentialRow{
			ID:     check.Entry.ID,
			Status: string(check.Status),
			Source: check.Entry.Kind(),
		})
		if check.Status != credential.Ready && !check.Entry.Required() {
			warnings = append(warnings, fmt.Sprintf("optional credential %s is %s", check.Entry.ID, check.Status))
		}
	}
	return write(stdout, credentialListView{Credentials: rows, Warnings: warnings})
}

func credentialInspect(id string, stdout io.Writer) int {
	manifest, err := loadManifest()
	if err != nil {
		return credentialError(stdout, err)
	}

	checker := credential.NewChecker()
	for _, entry := range manifest.Entries {
		if entry.ID != id {
			continue
		}
		check := checker.Check(entry)
		view := credentialInspectView{Credential: credentialDetail{
			ID:          entry.ID,
			Type:        entry.Type,
			Status:      string(check.Status),
			Source:      entry.Kind(),
			Reference:   entry.Ref(),
			RequiredFor: entry.RequiredFor,
		}}
		return write(stdout, view)
	}
	return writeError(stdout, "not_found", fmt.Sprintf("Credential not found: %s", id), false, "Run `akagent credential list`")
}

func credentialDoctor(stdout io.Writer) int {
	manifest, err := loadManifest()
	if err != nil {
		return credentialError(stdout, err)
	}

	summary := doctorCheck{Overall: "ok"}
	rows := make([]credentialRow, 0, len(manifest.Entries))
	var warnings, errors []string
	for _, check := range credential.Doctor(manifest, credential.NewChecker()) {
		rows = append(rows, credentialRow{
			ID:     check.Entry.ID,
			Status: string(check.Status),
			Source: check.Entry.Kind(),
		})
		switch check.Status {
		case credential.Ready:
			summary.Ready++
			continue
		case credential.Missing:
			summary.Missing++
		case credential.Unsafe:
			summary.Unsafe++
		case credential.Unavailable:
			summary.Unavailable++
		case credential.Unsupported:
			summary.Unsupported++
		}

		message := fmt.Sprintf("%s credential %s is %s", scope(check.Entry), check.Entry.ID, check.Status)
		if check.Entry.Required() {
			summary.Overall = "failed"
			errors = append(errors, message)
		} else {
			if summary.Overall == "ok" {
				summary.Overall = "degraded"
			}
			warnings = append(warnings, message)
		}
	}

	if exitCode := write(stdout, doctorView{
		Check:       summary,
		Credentials: rows,
		Warnings:    warnings,
		Errors:      errors,
	}); exitCode != 0 {
		return exitCode
	}
	if summary.Overall == "failed" {
		return 1
	}
	return 0
}

func scope(entry credential.Entry) string {
	if entry.Required() {
		return fmt.Sprintf("required (%s)", entry.RequiredFor)
	}
	return "optional"
}

func loadManifest() (*credential.Manifest, error) {
	manifest, err := credential.Load(credential.ConfigPath())
	if err != nil {
		return nil, fmt.Errorf("credential manifest could not be loaded")
	}
	return manifest, nil
}

func credentialError(stdout io.Writer, err error) int {
	return writeError(stdout, "capability",
		"Credential manifest is invalid: "+credential.Redact(err.Error()),
		false, "Fix the configured credential manifest or run `akagent credential list`")
}
