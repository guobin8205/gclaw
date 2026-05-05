package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Manager discovers, loads, and formats skills for system prompt injection.
type Manager struct {
	mu     sync.RWMutex
	skills map[string]*Skill // keyed by Name
	dirs   []string
}

// NewManager creates a skill manager.
func NewManager() *Manager {
	return &Manager{
		skills: make(map[string]*Skill),
	}
}

// LoadAll scans baseDir/user and baseDir/agent for SKILL.md files.
// Supports both flat and one-level nested directory structures for categories.
func (m *Manager) LoadAll(baseDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dirs = []string{
		filepath.Join(baseDir, "user"),
		filepath.Join(baseDir, "agent"),
	}

	m.skills = make(map[string]*Skill)
	for _, d := range m.dirs {
		if _, err := os.Stat(d); os.IsNotExist(err) {
			continue
		}
		m.scanDir(d, d, "")
	}
	return nil
}

// LoadProject loads project-bundled skills from a project skills directory.
// Supports both flat and one-level nested directory structures for categories.
func (m *Manager) LoadProject(projectDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return nil
	}
	m.scanDir(projectDir, projectDir, "project")
	return nil
}

// scanDir recursively scans for SKILL.md files up to one level of nesting.
func (m *Manager) scanDir(dir, scanRoot, sourceOverride string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == ".archive" {
			continue
		}
		childPath := filepath.Join(dir, entry.Name())
		skillFile := filepath.Join(childPath, skillFileName)

		if _, err := os.Stat(skillFile); err == nil {
			// This directory contains a SKILL.md - it's a skill
			s, err := Parse(childPath)
			if err != nil {
				continue
			}
			s.Category = categoryFromPath(childPath, scanRoot)
			if sourceOverride != "" {
				s.Source = sourceOverride
			}
			m.skills[s.Name] = s
		} else {
			// No SKILL.md - treat as category directory, scan one level deeper
			subEntries, err := os.ReadDir(childPath)
			if err != nil {
				continue
			}
			for _, sub := range subEntries {
				if !sub.IsDir() {
					continue
				}
				subPath := filepath.Join(childPath, sub.Name())
				s, err := Parse(subPath)
				if err != nil {
					continue
				}
				s.Category = categoryFromPath(subPath, scanRoot)
				if sourceOverride != "" {
					s.Source = sourceOverride
				}
				m.skills[s.Name] = s
			}
		}
	}
}

// categoryFromPath derives the category from the skill directory relative to the scan root.
func categoryFromPath(skillDir, scanRoot string) string {
	rel, err := filepath.Rel(scanRoot, skillDir)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) <= 1 {
		return ""
	}
	return parts[0]
}

// ForSystemIndex returns a compact skill index for system prompt injection.
// Only names and descriptions are included, grouped by category.
func (m *Manager) ForSystemIndex() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.skills) == 0 {
		return ""
	}

	// Group skills by category
	categories := make(map[string][]*Skill)
	for _, s := range m.skills {
		cat := s.Category
		if cat == "" {
			cat = "[uncategorized]"
		}
		categories[cat] = append(categories[cat], s)
	}

	catNames := make([]string, 0, len(categories))
	for c := range categories {
		catNames = append(catNames, c)
	}
	sort.Strings(catNames)

	var sb strings.Builder
	sb.WriteString("\n## Skills\n")
	sb.WriteString("Load relevant skills with skill_view(name) before using them.\n\n")

	for _, cat := range catNames {
		sb.WriteString(fmt.Sprintf("  %s:\n", cat))
		skills := categories[cat]
		sort.Slice(skills, func(i, j int) bool {
			return skills[i].Name < skills[j].Name
		})
		for _, s := range skills {
			sb.WriteString(fmt.Sprintf("    - %s: %s\n", s.Name, s.Description))
		}
	}

	return sb.String()
}

// ForSystemPrompt formats all skill bodies for system prompt injection.
// Kept for backward compatibility; prefer ForSystemIndex() for large skill sets.
func (m *Manager) ForSystemPrompt() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n## Active Skills\n\n")
	for _, s := range m.skills {
		sb.WriteString(fmt.Sprintf("### %s\n", s.Name))
		sb.WriteString(expandVars(s.Body, s.Dir))
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// GetBody returns the expanded body of a skill by name, or empty string if not found.
func (m *Manager) GetBody(name string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.skills[name]
	if !ok {
		return ""
	}
	return expandVars(s.Body, s.Dir)
}

// expandVars replaces ${SKILL_DIR} in skill body.
func expandVars(body, dir string) string {
	body = strings.ReplaceAll(body, "${SKILL_DIR}", dir)
	return body
}

// List returns all loaded skills.
func (m *Manager) List() []Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Skill, 0, len(m.skills))
	for _, s := range m.skills {
		result = append(result, *s)
	}
	return result
}

// Get returns a skill by name.
func (m *Manager) Get(name string) (*Skill, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.skills[name]
	return s, ok
}

// CreateSkill writes a new SKILL.md to the agent directory.
func (m *Manager) CreateSkill(name, body, description string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.dirs) < 2 {
		return fmt.Errorf("skill manager not loaded (call LoadAll first)")
	}

	agentDir := m.dirs[1] // agent dir
	skillDir := filepath.Join(agentDir, name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return err
	}

	fm := fmt.Sprintf("name: %s\ndescription: %s\nsource: agent\n", name, description)
	content := fmt.Sprintf("---\n%s---\n\n%s", fm, body)
	path := filepath.Join(skillDir, skillFileName)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}

	m.skills[name] = &Skill{
		Name:        name,
		Description: description,
		Source:      "agent",
		Body:        body,
		Dir:         skillDir,
	}
	return nil
}

// DeleteSkill removes a skill by name (agent-created only).
func (m *Manager) DeleteSkill(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.skills[name]
	if !ok {
		return fmt.Errorf("skill %q not found", name)
	}
	if s.Source != "agent" {
		return fmt.Errorf("cannot delete %s skill %q", s.Source, name)
	}
	if err := os.RemoveAll(s.Dir); err != nil {
		return err
	}
	delete(m.skills, name)
	return nil
}

// ListAgentSkills returns only agent-source skills.
func (m *Manager) ListAgentSkills() []Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []Skill
	for _, s := range m.skills {
		if s.Source == "agent" {
			result = append(result, *s)
		}
	}
	return result
}

// SkillDir returns the base skill directory.
func (m *Manager) SkillDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.dirs) > 0 {
		return filepath.Dir(m.dirs[0])
	}
	return ""
}

// AgentDir returns the agent skills directory.
func (m *Manager) AgentDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.dirs) >= 2 {
		return m.dirs[1]
	}
	return ""
}

// ArchiveSkill moves a skill to .archive/ (agent-source only).
func (m *Manager) ArchiveSkill(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.skills[name]
	if !ok {
		return fmt.Errorf("skill %q not found", name)
	}
	if s.Source != "agent" {
		return fmt.Errorf("cannot archive %s skill %q", s.Source, name)
	}
	baseDir := ""
	if len(m.dirs) > 0 {
		baseDir = filepath.Dir(m.dirs[0])
	}
	archiveDir := filepath.Join(baseDir, ".archive")
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return err
	}
	dest := filepath.Join(archiveDir, name)
	if err := os.Rename(s.Dir, dest); err != nil {
		return err
	}
	delete(m.skills, name)
	return nil
}

// UnarchiveSkill restores a skill from .archive/ back to agent dir.
func (m *Manager) UnarchiveSkill(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	baseDir := ""
	if len(m.dirs) > 0 {
		baseDir = filepath.Dir(m.dirs[0])
	}
	archivedPath := filepath.Join(baseDir, ".archive", name)
	if _, err := os.Stat(archivedPath); os.IsNotExist(err) {
		return fmt.Errorf("archived skill %q not found", name)
	}
	agentDir := ""
	if len(m.dirs) >= 2 {
		agentDir = m.dirs[1]
	}
	if agentDir == "" {
		return fmt.Errorf("agent skill dir not available")
	}
	dest := filepath.Join(agentDir, name)
	if err := os.Rename(archivedPath, dest); err != nil {
		return err
	}
	s, err := Parse(dest)
	if err != nil {
		return err
	}
	s.Source = "agent"
	m.skills[s.Name] = s
	return nil
}

// SetPinned toggles the pinned flag on a skill.
func (m *Manager) SetPinned(name string, pinned bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.skills[name]
	if !ok {
		return fmt.Errorf("skill %q not found", name)
	}
	s.Pinned = pinned
	return m.rewriteSkillFile(s)
}

// SetDescription updates a skill description.
func (m *Manager) SetDescription(name, description string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.skills[name]
	if !ok {
		return fmt.Errorf("skill %q not found", name)
	}
	s.Description = description
	return m.rewriteSkillFile(s)
}

// rewriteSkillFile rewrites SKILL.md frontmatter + body.
func (m *Manager) rewriteSkillFile(s *Skill) error {
	lines := []string{
		"name: " + s.Name,
		"description: " + s.Description,
		"source: " + s.Source,
		fmt.Sprintf("pinned: %t", s.Pinned),
	}
	if s.Category != "" {
		lines = append(lines, "category: "+s.Category)
	}
	fm := strings.Join(lines, string(byte(10))) + string(byte(10))
	content := string([]byte("---")) + string(byte(10)) + fm + string([]byte("---")) + string(byte(10)) + string(byte(10)) + s.Body
	path := filepath.Join(s.Dir, skillFileName)
	return os.WriteFile(path, []byte(content), 0644)
}


