package skill

// Skill is a reusable procedural knowledge unit.
type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Source      string `yaml:"source"` // "user" | "agent" | "project"
	Category    string `yaml:"category"`
	Pinned      bool   `yaml:"pinned"` // pinned skills are never touched by curator
	Body        string // frontmatter 之后的所有内容
	Dir         string // 技能目录绝对路径
}
