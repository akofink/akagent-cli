package lifecycle

import (
	"fmt"
	"strings"

	"github.com/akofink/akagent-cli/internal/store"
)

// CreateResource records caller-declared repository and worktree facts.
func (m *Manager) CreateResource(taskID string, request ResourceRequest) (store.Resource, bool, error) {
	manifest, err := m.Inspect(taskID)
	if err != nil {
		return store.Resource{}, false, err
	}
	if manifest.Provenance == store.ProvenanceExternal {
		return store.Resource{}, false, externalRecordOperationError("task", taskID)
	}
	return m.CreateResourceRecord(taskID, request)
}

func (m *Manager) ListResources(taskID string) ([]store.Resource, error) {
	manifest, err := m.Inspect(taskID)
	if err != nil {
		return nil, err
	}
	if manifest.ResourceIDs == "" {
		if manifest.Repository == "" {
			return []store.Resource{}, nil
		}
		resource, _, err := m.CreateResourceRecord(taskID, ResourceRequest{ID: "legacy", Repository: manifest.Repository, Branch: manifest.Branch, BaseRevision: manifest.BaseRevision, WorktreePath: manifest.WorktreePath})
		if err != nil {
			return nil, err
		}
		return []store.Resource{resource}, nil
	}
	ids := unique(splitIDs(manifest.ResourceIDs))
	resources := make([]store.Resource, 0, len(ids))
	for _, id := range ids {
		resource, err := m.Store.ReadResource(taskID, id)
		if err != nil {
			return nil, err
		}
		resources = append(resources, resource)
	}
	return resources, nil
}

func (m *Manager) InspectResource(taskID, resourceID string) (store.Resource, error) {
	resources, err := m.ListResources(taskID)
	if err != nil {
		return store.Resource{}, err
	}
	if resourceID == "" {
		if len(resources) != 1 {
			return store.Resource{}, validationError("resource ID is required when a task has multiple resources")
		}
		return resources[0], nil
	}
	for _, resource := range resources {
		if resource.ID == resourceID {
			return resource, nil
		}
	}
	return store.Resource{}, &store.Error{Kind: store.KindNotFound, Message: fmt.Sprintf("resource %s not found", resourceID)}
}

func (m *Manager) UpdateResource(taskID, resourceID string, request ResourceUpdateRequest) (store.Resource, error) {
	if resourceID == "" {
		return store.Resource{}, validationError("resource ID is required")
	}
	changed := false
	resource, err := m.Store.UpdateResource(taskID, resourceID, func(resource *store.Resource) error {
		if len(request.Metadata) > 0 {
			if resource.Metadata == nil {
				resource.Metadata = map[string]string{}
			}
			for key, value := range request.Metadata {
				if resource.Metadata[key] != value {
					changed = true
				}
				resource.Metadata[key] = value
			}
		}
		for _, url := range uniqueStrings(request.ExternalURLs) {
			if !containsString(resource.ExternalURLs, url) {
				resource.ExternalURLs = append(resource.ExternalURLs, url)
				changed = true
			}
		}
		return nil
	})
	if err != nil {
		return store.Resource{}, err
	}
	if changed {
		if _, err := m.Store.AppendResourceEvent(taskID, resourceID, store.Event{Operation: "metadata", Outcome: "updated"}); err != nil {
			return store.Resource{}, err
		}
	}
	return resource, nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func splitIDs(value string) []string {
	var ids []string
	for _, id := range strings.Split(value, ",") {
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
