package skill

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const skillFileName = "SKILL.md"

// Parse reads a SKILL.md file and returns the Skill.
func Parse(dir string) (*Skill, error) {
	path := filepath.Join(dir, skillFileName)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fm, body, err := splitFrontmatter(f)
	if err != nil {
		return nil, err
	}

	s := &Skill{
		Dir:  dir,
		Body: strings.TrimSpace(body),
	}
	if fm != "" {
		if err := yaml.Unmarshal([]byte(fm), s); err != nil {
			return nil, err
		}
	}
	if s.Name == "" {
		s.Name = filepath.Base(dir)
	}
	if s.Source == "" {
		// Infer source from path: .../user/... → "user", .../agent/... → "agent"
		parts := strings.Split(filepath.ToSlash(dir), "/")
		for i, p := range parts {
			if (p == "user" || p == "agent") && i == len(parts)-2 {
				s.Source = p
				break
			}
		}
		if s.Source == "" {
			s.Source = "user"
		}
	}
	return s, nil
}

// splitFrontmatter splits YAML frontmatter from markdown body.
// Frontmatter is delimited by --- on the first line and --- to close.
func splitFrontmatter(f *os.File) (frontmatter, body string, err error) {
	scanner := bufio.NewScanner(f)

	// Check if first line is "---"
	if !scanner.Scan() {
		return "", "", nil
	}
	firstLine := strings.TrimSpace(scanner.Text())
	if firstLine != "---" {
		// No frontmatter, first line is body
		body = firstLine + "\n"
		for scanner.Scan() {
			body += scanner.Text() + "\n"
		}
		return "", body, nil
	}

	// Read frontmatter lines until closing "---"
	var fmLines []string
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		fmLines = append(fmLines, trimmed)
	}

	// Remaining lines are body
	for scanner.Scan() {
		body += scanner.Text() + "\n"
	}

	return strings.Join(fmLines, "\n"), body, nil
}
