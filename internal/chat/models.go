package chat

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func ChatPrompt(role string, tty bool) string {
	if !tty {
		return role + "> "
	}

	switch role {
	case "You":
		return "\x1b[36mYou>\x1b[0m "
	case "Assistant":
		return "\x1b[35mAssistant>\x1b[0m "
	default:
		return role + "> "
	}
}

func ChatHeader(role string, tty bool) string {
	if !tty {
		return role + ">"
	}

	switch role {
	case "Assistant":
		return "\x1b[35mAssistant>\x1b[0m"
	case "You":
		return "\x1b[36mYou>\x1b[0m"
	default:
		return role + ">"
	}
}

func ChatModelSelector(model ModelInfo) string {
	if model.Owner == "" || model.Owner == "virtual" || strings.HasPrefix(model.ID, model.Owner+"/") {
		return model.ID
	}
	return model.Owner + "/" + model.ID
}

func PromptModelPicker(prompt string, models []ModelInfo, selectFromOptions func(string, []string) (string, error)) (string, error) {
	items := make([]string, 0, len(models))
	for _, model := range models {
		label := model.Selector
		if model.Name != "" && model.Name != model.ID {
			label = fmt.Sprintf("%s — %s", model.Selector, model.Name)
		}
		items = append(items, label)
	}

	selected, err := selectFromOptions(prompt, items)
	if err != nil {
		return "", err
	}
	for i, item := range items {
		if item == selected {
			return models[i].Selector, nil
		}
	}
	return "", nil
}

func FilterModels(models []ModelInfo, filter string) []ModelInfo {
	filter = strings.TrimSpace(strings.ToLower(filter))
	if filter == "" {
		return models
	}

	filtered := make([]ModelInfo, 0, len(models))
	for _, model := range models {
		if strings.Contains(strings.ToLower(model.ID), filter) || strings.Contains(strings.ToLower(model.Name), filter) || strings.Contains(strings.ToLower(model.Owner), filter) || strings.Contains(strings.ToLower(model.Selector), filter) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func ListModels(c Client) ([]ModelInfo, error) {
	statusData, err := c.Get("/api/admin/status")
	if err != nil {
		return nil, err
	}

	var statusResp map[string]any
	if err := json.Unmarshal(statusData, &statusResp); err != nil {
		return nil, err
	}

	type providerInfo struct {
		id   string
		name string
	}
	providers := make([]providerInfo, 0)
	if items, ok := statusResp["activeProviders"].([]any); ok {
		for _, item := range items {
			entry, _ := item.(map[string]any)
			id, _ := entry["id"].(string)
			name, _ := entry["name"].(string)
			if id == "" {
				continue
			}
			providers = append(providers, providerInfo{id: id, name: name})
		}
	}

	models := make([]ModelInfo, 0)
	for _, provider := range providers {
		data, err := c.Get("/api/admin/providers/" + provider.id + "/models")
		if err != nil {
			continue
		}

		var resp map[string]any
		if err := json.Unmarshal(data, &resp); err != nil {
			continue
		}
		items, _ := resp["models"].([]any)
		for _, item := range items {
			entry, _ := item.(map[string]any)
			enabled, _ := entry["enabled"].(bool)
			if !enabled {
				continue
			}
			id, _ := entry["id"].(string)
			name, _ := entry["name"].(string)
			selector := ChatModelSelector(ModelInfo{ID: id, Owner: provider.id, Name: name})
			models = append(models, ModelInfo{
				ID:           id,
				Owner:        provider.id,
				OwnerName:    provider.name,
				Name:         name,
				Selector:     selector,
				ProviderID:   provider.id,
				ProviderName: provider.name,
			})
		}
	}

	allModelsData, err := c.Get("/models")
	if err == nil {
		var allResp map[string]any
		if json.Unmarshal(allModelsData, &allResp) == nil {
			items, _ := allResp["data"].([]any)
			for _, item := range items {
				entry, _ := item.(map[string]any)
				owner, _ := entry["owned_by"].(string)
				if owner != "virtual" {
					continue
				}
				id, _ := entry["id"].(string)
				name, _ := entry["display_name"].(string)
				selector := ChatModelSelector(ModelInfo{ID: id, Owner: owner, Name: name})
				models = append(models, ModelInfo{
					ID:           id,
					Owner:        owner,
					OwnerName:    "Virtual",
					Name:         name,
					Selector:     selector,
					ProviderID:   owner,
					ProviderName: "Virtual",
				})
			}
		}
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].ProviderName != models[j].ProviderName {
			return models[i].ProviderName < models[j].ProviderName
		}
		return models[i].Name < models[j].Name
	})
	return models, nil
}
