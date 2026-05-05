package skill_tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/openclaw/gclaw/internal/skill"
	"github.com/openclaw/gclaw/internal/tool"
)

// ManagerRef holds a reference to the skill manager, set by main.
var ManagerRef *skill.Manager

// ---- Skill List ----

type skillListTool struct{}

func (t *skillListTool) Name() string    { return "skill_list" }
func (t *skillListTool) Toolset() string { return "skill" }
func (t *skillListTool) Description() string {
	return "List all available skills (both user-created and agent-created)."
}
func (t *skillListTool) Check() bool           { return ManagerRef != nil }
func (t *skillListTool) ConcurrencySafe() bool { return true }
func (t *skillListTool) RequiresApproval(map[string]any) bool { return false }

func (t *skillListTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"filter": {Type: "string", Description: "Optional filter: 'user', 'agent', or empty for all"},
		},
	}
}

func (t *skillListTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	skills := ManagerRef.List()
	filter, _ := params["filter"].(string)

	var sb strings.Builder
	count := 0
	for _, s := range skills {
		if filter != "" && s.Source != filter {
			continue
		}
		count++
		sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", s.Source, s.Name, s.Description))
	}
	if count == 0 {
		return tool.ToolResult{Content: "No skills found."}, nil
	}
	return tool.ToolResult{Content: fmt.Sprintf("%d skills:\n%s", count, sb.String())}, nil
}

func init() {
	tool.GlobalRegistry.Register(&skillListTool{})
}

// ---- Skill View ----

type skillViewTool struct{}

func (t *skillViewTool) Name() string    { return "skill_view" }
func (t *skillViewTool) Toolset() string { return "skill" }
func (t *skillViewTool) Description() string {
	return "Load and return the full content of a skill by name. Use this before following a skill's instructions."
}
func (t *skillViewTool) Check() bool           { return ManagerRef != nil }
func (t *skillViewTool) ConcurrencySafe() bool { return true }
func (t *skillViewTool) RequiresApproval(map[string]any) bool { return false }

func (t *skillViewTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"name": {Type: "string", Description: "Name of the skill to load"},
		},
		Required: []string{"name"},
	}
}

func (t *skillViewTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	name, _ := params["name"].(string)
	if name == "" {
		return tool.ToolResult{Content: "Skill name is required.", IsError: true}, nil
	}
	body := ManagerRef.GetBody(name)
	if body == "" {
		return tool.ToolResult{Content: fmt.Sprintf("Skill %q not found.", name), IsError: true}, nil
	}
	return tool.ToolResult{Content: body}, nil
}

func init() {
	tool.GlobalRegistry.Register(&skillViewTool{})
}

// ---- Skill Create ----

type skillCreateTool struct{}

func (t *skillCreateTool) Name() string    { return "skill_create" }
func (t *skillCreateTool) Toolset() string { return "skill" }
func (t *skillCreateTool) Description() string {
	return "Create a new agent skill. The skill body should contain instructions the agent should follow. After creating, the skill is immediately active."
}
func (t *skillCreateTool) Check() bool           { return ManagerRef != nil }
func (t *skillCreateTool) ConcurrencySafe() bool { return false }
func (t *skillCreateTool) RequiresApproval(map[string]any) bool { return true }

func (t *skillCreateTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"name":        {Type: "string", Description: "Unique skill name (slug format, e.g. 'my-skill')"},
			"description": {Type: "string", Description: "Brief description of what the skill does"},
			"body":        {Type: "string", Description: "Skill instructions in markdown format"},
		},
		Required: []string{"name", "body"},
	}
}

func (t *skillCreateTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	name, _ := params["name"].(string)
	body, _ := params["body"].(string)
	desc, _ := params["description"].(string)

	if err := ManagerRef.CreateSkill(name, body, desc); err != nil {
		return tool.ToolResult{Content: fmt.Sprintf("Failed to create skill: %v", err), IsError: true}, nil
	}
	return tool.ToolResult{Content: fmt.Sprintf("Skill %q created successfully.", name)}, nil
}

func init() {
	tool.GlobalRegistry.Register(&skillCreateTool{})
}

// ---- Skill Delete ----

type skillDeleteTool struct{}

func (t *skillDeleteTool) Name() string    { return "skill_delete" }
func (t *skillDeleteTool) Toolset() string { return "skill" }
func (t *skillDeleteTool) Description() string {
	return "Delete an agent-created skill by name. User-created skills cannot be deleted."
}
func (t *skillDeleteTool) Check() bool           { return ManagerRef != nil }
func (t *skillDeleteTool) ConcurrencySafe() bool { return false }
func (t *skillDeleteTool) RequiresApproval(map[string]any) bool { return true }

func (t *skillDeleteTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"name": {Type: "string", Description: "Name of the skill to delete"},
		},
		Required: []string{"name"},
	}
}

func (t *skillDeleteTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	name, _ := params["name"].(string)

	if err := ManagerRef.DeleteSkill(name); err != nil {
		return tool.ToolResult{Content: fmt.Sprintf("Failed to delete skill: %v", err), IsError: true}, nil
	}
	return tool.ToolResult{Content: fmt.Sprintf("Skill %q deleted.", name)}, nil
}

func init() {
	tool.GlobalRegistry.Register(&skillDeleteTool{})
}
