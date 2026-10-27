package render

import (
	_ "embed"
	"encoding/json"

	"slate/internal/api"
)

//go:embed assets/templates.json
var templatesJSON []byte

var templates = func() struct {
	System  []api.DashboardSystemTemplate `json:"system"`
	Starter api.DashboardTemplate         `json:"starter"`
} {
	var t struct {
		System  []api.DashboardSystemTemplate `json:"system"`
		Starter api.DashboardTemplate         `json:"starter"`
	}
	if err := json.Unmarshal(templatesJSON, &t); err != nil {
		panic(err)
	}
	return t
}()

// SystemTemplates are the built-in dashboard layouts for data pushed by external programs.
func SystemTemplates() []api.DashboardSystemTemplate { return templates.System }

// StarterTemplate is the default layout of a new custom dashboard.
func StarterTemplate() api.DashboardTemplate { return templates.Starter }

func SystemTemplate(id string) *api.DashboardSystemTemplate {
	for i := range templates.System {
		if templates.System[i].ID == id {
			return &templates.System[i]
		}
	}
	return nil
}

// TemplateFor resolves a dashboard template reference.
func TemplateFor(ref *api.DashboardTemplateRef) *api.DashboardTemplate {
	switch {
	case ref == nil:
		return nil
	case ref.Kind == "custom":
		return ref.Template
	case ref.Kind == "system":
		if t := SystemTemplate(ref.ID); t != nil {
			return &t.Template
		}
	}
	return nil
}
