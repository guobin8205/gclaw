package tool

import (
	"context"

	"github.com/openclaw/gclaw/internal/model"
)

// Schema represents the JSON Schema for tool parameters.
type Schema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

// Property describes a single parameter in the tool schema.
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ToolResult is the result of executing a tool.
type ToolResult struct {
	Content string
	IsError bool
}

// Tool is the unified interface all tools must implement.
type Tool interface {
	Name() string
	Toolset() string
	Description() string
	InputSchema() Schema
	Check() bool // availability gate; return false to hide from LLM
	ConcurrencySafe() bool
	RequiresApproval(params map[string]any) bool

	Execute(ctx context.Context, params map[string]any) (ToolResult, error)
}

// Registry manages tool registration and lookup.
type Registry struct {
	tools map[string]Tool
}

// GlobalRegistry is the package-level registry for init()-based self-registration.
var GlobalRegistry = NewRegistry()

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// Get looks up a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools as model.ToolDef for the API (unfiltered).
func (r *Registry) List() []model.ToolDef {
	var defs []model.ToolDef
	for _, t := range r.tools {
		schema := make(map[string]any)
		schema["type"] = t.InputSchema().Type
		if len(t.InputSchema().Properties) > 0 {
			props := make(map[string]any)
			for k, v := range t.InputSchema().Properties {
				props[k] = map[string]any{
					"type":        v.Type,
					"description": v.Description,
				}
			}
			schema["properties"] = props
		}
		if len(t.InputSchema().Required) > 0 {
			schema["required"] = t.InputSchema().Required
		}

		defs = append(defs, model.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: schema,
		})
	}
	return defs
}

// AvailableTools returns only tools whose Check() passes.
func (r *Registry) AvailableTools() []model.ToolDef {
	return r.listFiltered(nil)
}

func (r *Registry) listFiltered(toolset *string) []model.ToolDef {
	var defs []model.ToolDef
	for _, t := range r.tools {
		if toolset != nil && t.Toolset() != *toolset {
			continue
		}
		if !t.Check() {
			continue
		}
		schema := make(map[string]any)
		schema["type"] = t.InputSchema().Type
		if len(t.InputSchema().Properties) > 0 {
			props := make(map[string]any)
			for k, v := range t.InputSchema().Properties {
				props[k] = map[string]any{
					"type":        v.Type,
					"description": v.Description,
				}
			}
			schema["properties"] = props
		}
		if len(t.InputSchema().Required) > 0 {
			schema["required"] = t.InputSchema().Required
		}

		defs = append(defs, model.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: schema,
		})
	}
	return defs
}

// ListByToolset returns tools in a specific toolset whose Check() passes.
func (r *Registry) ListByToolset(toolset string) []model.ToolDef {
	return r.listFiltered(&toolset)
}

// Toolsets returns all distinct toolset names.
func (r *Registry) Toolsets() []string {
	seen := make(map[string]bool)
	for _, t := range r.tools {
		if t.Check() {
			seen[t.Toolset()] = true
		}
	}
	var names []string
	for n := range seen {
		names = append(names, n)
	}
	return names
}

// Names returns all registered tool names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	return names
}
