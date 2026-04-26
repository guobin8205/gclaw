package autonomous

import "time"

// Level defines how independently the agent operates.
type Level int

const (
	Interactive    Level = iota // User-driven, wait for input
	Semi                        // User sets goals, agent plans and executes with checkpoints
	Full                        // Heartbeat-driven, fully independent
)

func (l Level) String() string {
	switch l {
	case Interactive:
		return "interactive"
	case Semi:
		return "semi"
	case Full:
		return "full"
	}
	return "unknown"
}

// ParseLevel converts a string to an autonomy level.
func ParseLevel(s string) Level {
	switch s {
	case "full":
		return Full
	case "semi":
		return Semi
	default:
		return Interactive
	}
}

// SystemPrompt returns the autonomy-specific system prompt injection.
func SystemPrompt(level Level) string {
	switch level {
	case Interactive:
		return ""
	case Semi:
		return semiPrompt
	case Full:
		return fullPrompt
	}
	return ""
}

const semiPrompt = "## Autonomy Mode: Semi-Autonomous\n" +
	"You are operating semi-autonomously. Plan your approach and execute independently.\n" +
	"Pause and ask for confirmation for critical decisions: destructive operations,\n" +
	"external API calls, or significant architectural changes.\n" +
	"Provide status updates at natural checkpoints."

const fullPrompt = "## Autonomy Mode: Fully Autonomous\n" +
	"You are running autonomously. You will receive <tick> prompts to keep you active.\n" +
	"Proactively check for work to do - read files, make changes, commit without asking.\n" +
	"If there is nothing useful to do, call SleepTool.\n" +
	"Only notify the user when you encounter a genuine problem or emergency.\n\n" +
	"### Decision Guidelines\n" +
	"- Routine operations (reading, editing, searching, git status/diff/commit): do immediately\n" +
	"- Destructive operations (rm -rf, force push, drop table): always warn\n" +
	"- External services (API calls, dependency changes): proceed if within project scope\n" +
	"- When in doubt, err on the side of action - the user wants results, not questions"

// Config holds autonomous mode configuration.
type Config struct {
	Level        Level
	TickInterval time.Duration
	IdleSleep    time.Duration
	MaxTicks     int // max ticks before forced sleep, 0 = unlimited
}
