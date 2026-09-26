package app

import (
	"io"
	"strings"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
	"github.com/google/uuid"
)

func taskResourceCommand(args []string, stdout io.Writer) int {
	if len(args) == 0 {
		return writeError(stdout, "usage", "Usage: akagent task resource <create|list|inspect|update|archive>", false, "Run `akagent task resource list <task-id>`")
	}
	if args[0] == "clean" {
		return removedCommandError(stdout, "task resource clean", "External tools own worktree and credential cleanup; retain cleanup debt in the record")
	}
	state, err := store.Open()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	manager := lifecycle.New(state)
	switch args[0] {
	case "create", "add":
		if len(args) < 2 {
			return writeError(stdout, "usage", "Usage: akagent task resource create <task-id> --repository <name> [--resource-id <id>] [--branch <branch>] [--base <revision>] [--worktree <path>] [--metadata <key=value>] [--external-url <https-url>]", false, "Create the task first, then add a Git resource")
		}
		request, ok := parseResourceCreate(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task resource create <task-id> --repository <name> [--resource-id <id>] [--branch <branch>] [--base <revision>] [--worktree <path>] [--metadata <key=value>] [--external-url <https-url>]", false, "Provide a repository and immutable Git inputs")
		}
		if request.ID == "" {
			id, idErr := uuid.NewV7()
			if idErr != nil {
				return writeError(stdout, "internal", "Failed to generate a resource ID", false, "Retry resource creation")
			}
			request.ID = id.String()
		}
		resource, _, err := manager.CreateResourceRecord(args[1], request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, resourceDetailView{Resource: viewResource(resource)})
	case "list":
		if len(args) != 2 {
			return writeError(stdout, "usage", "Usage: akagent task resource list <task-id>", false, "Run `akagent task list`")
		}
		resources, err := manager.ListResources(args[1])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		items := make([]resourceListItem, 0, len(resources))
		for _, resource := range resources {
			items = append(items, viewResourceList(resource))
		}
		return write(stdout, resourceListView{Resources: items, Total: len(items)})
	case "inspect":
		if len(args) < 2 || len(args) > 3 {
			return writeError(stdout, "usage", "Usage: akagent task resource inspect <task-id> [<resource-id>]", false, "Run `akagent task resource list <task-id>`")
		}
		resourceID := ""
		if len(args) == 3 {
			resourceID = args[2]
		}
		resource, err := manager.InspectResource(args[1], resourceID)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, resourceDetailView{Resource: viewResource(resource)})
	case "update":
		if len(args) < 3 {
			return writeError(stdout, "usage", "Usage: akagent task resource update <task-id> <resource-id> [--metadata <key=value>] [--external-url <https-url>]", false, "Record non-secret delivery metadata for the resource")
		}
		request, ok := parseResourceUpdate(args[3:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task resource update <task-id> <resource-id> [--metadata <key=value>] [--external-url <https-url>]", false, "Record non-secret delivery metadata for the resource")
		}
		resource, err := manager.UpdateResource(args[1], args[2], request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, resourceDetailView{Resource: viewResource(resource)})
	case "archive":
		if len(args) != 3 {
			return writeError(stdout, "usage", "Usage: akagent task resource archive <task-id> <resource-id>", false, "Inspect the resource before archiving it")
		}
		archived, err := manager.ArchiveRecord(args[1], args[2], "")
		if err != nil {
			return lifecycleError(stdout, err)
		}
		resource, ok := archived.(store.Resource)
		if !ok {
			return writeError(stdout, "internal", "Resource archive returned an invalid record", false, "Retry the archive")
		}
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, resourceDetailView{Resource: viewResource(resource)})
	default:
		return writeError(stdout, "usage", "Usage: akagent task resource <create|list|inspect|update|archive>", false, "Run `akagent task resource list <task-id>`")
	}
}

func parseResourceCreate(args []string) (lifecycle.ResourceRequest, bool) {
	var request lifecycle.ResourceRequest
	metadata, urls, ok := parseResourceMetadata(args, &request.ID, &request.Repository, &request.Branch, &request.BaseRevision, &request.WorktreePath, &request.Head)
	request.Metadata, request.ExternalURLs = metadata, urls
	return request, ok && request.Repository != ""
}

func parseResourceUpdate(args []string) (lifecycle.ResourceUpdateRequest, bool) {
	var request lifecycle.ResourceUpdateRequest
	metadata, urls, ok := parseResourceMetadata(args, nil, nil, nil, nil, nil, nil)
	request.Metadata, request.ExternalURLs = metadata, urls
	return request, ok && (len(metadata) > 0 || len(urls) > 0)
}

func parseResourceMetadata(args []string, id, repository, branch, base, worktree, head *string) (map[string]string, []string, bool) {
	var metadata map[string]string
	var urls []string
	for len(args) > 0 {
		if len(args) < 2 {
			return nil, nil, false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		switch flag {
		case "--resource-id", "--id":
			if id == nil {
				return nil, nil, false
			}
			*id = value
		case "--repository":
			if repository == nil {
				return nil, nil, false
			}
			*repository = value
		case "--branch":
			if branch == nil {
				return nil, nil, false
			}
			*branch = value
		case "--base":
			if base == nil {
				return nil, nil, false
			}
			*base = value
		case "--worktree":
			if worktree == nil {
				return nil, nil, false
			}
			*worktree = value
		case "--head":
			if head == nil {
				return nil, nil, false
			}
			*head = value
		case "--metadata":
			key, metadataValue, found := strings.Cut(value, "=")
			if !found || key == "" || metadataValue == "" {
				return nil, nil, false
			}
			if metadata == nil {
				metadata = map[string]string{}
			}
			metadata[key] = metadataValue
		case "--external-url", "--external-reference", "--url":
			if value == "" {
				return nil, nil, false
			}
			urls = append(urls, value)
		default:
			return nil, nil, false
		}
	}
	return metadata, urls, true
}
