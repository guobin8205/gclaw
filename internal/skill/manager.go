package skill

import (
	"fmt"
	"os"
	"path/filepath"
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
		entries, err := os.ReadDir(d)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillDir := filepath.Join(d, entry.Name())
			s, err := Parse(skillDir)
			if err != nil {
				continue // skip broken skills
			}
			m.skills[s.Name] = s
		}
	}
	return nil
}

// ForSystemPrompt formats all skill bodies for system prompt injection.
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

// LoadProject loads project-bundled skills from a project skills directory.
// Skills are scanned from subdirectories and marked with source "project".
// Project skills have the lowest priority (loaded first, can be overridden by user/agent).
func (m *Manager) LoadProject(projectDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(projectDir, entry.Name())
		s, err := Parse(skillDir)
		if err != nil {
			continue
		}
		s.Source = "project"
		m.skills[s.Name] = s
	}
	return nil
}
