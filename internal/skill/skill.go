package skill

// Skill is a reusable procedural knowledge unit.
type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Source      string // "user" | "agent"
	Body        string // frontmatter 之后的所有内容
	Dir         string // 技能目录绝对路径
}
