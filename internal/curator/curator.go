package curator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openclaw/gclaw/internal/delegate"
	"github.com/openclaw/gclaw/internal/skill"
)

const (
	defaultInterval    = 7 * 24 * time.Hour
	defaultMinIdle     = 2 * time.Hour
	defaultStaleAfter  = 30 * 24 * time.Hour
	defaultArchiveAfter = 90 * 24 * time.Hour
)

// Config holds curator configuration.
type Config struct {
	Enabled      bool
	Interval     time.Duration
	MinIdle      time.Duration
	StaleAfter   time.Duration
	ArchiveAfter time.Duration
}

// State tracks curator runs.
type State struct {
	LastRunAt   time.Time `json:"last_run_at"`
	RunCount    int       `json:"run_count"`
	Paused      bool      `json:"paused"`
	LastSummary string    `json:"last_summary"`
}

// Curator maintains agent-created skills automatically.
type Curator struct {
	cfg       Config
	skillMgr  *skill.Manager
	factory   delegate.AgentFactory
	stateFile string
}

// New creates a curator instance.
func New(cfg Config, skillMgr *skill.Manager, factory delegate.AgentFactory) *Curator {
	if cfg.Interval <= 0 {
		cfg.Interval = defaultInterval
	}
	if cfg.MinIdle <= 0 {
		cfg.MinIdle = defaultMinIdle
	}
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = defaultStaleAfter
	}
	if cfg.ArchiveAfter <= 0 {
		cfg.ArchiveAfter = defaultArchiveAfter
	}
	stateFile := ""
	if skillMgr != nil {
		stateFile = filepath.Join(skillMgr.SkillDir(), ".curator_state")
	}
	return &Curator{
		cfg:       cfg,
		skillMgr:  skillMgr,
		factory:   factory,
		stateFile: stateFile,
	}
}

// loadState reads the curator state file.
func (c *Curator) loadState() State {
	if c.stateFile == "" {
		return State{}
	}
	data, err := os.ReadFile(c.stateFile)
	if err != nil {
		return State{}
	}
	var s State
	if json.Unmarshal(data, &s) != nil {
		return State{}
	}
	return s
}

// saveState writes the curator state file.
func (c *Curator) saveState(s State) {
	if c.stateFile == "" {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(c.stateFile), 0755)
	os.WriteFile(c.stateFile, data, 0644)
}

// Status returns the current curator state.
func (c *Curator) Status() State {
	return c.loadState()
}

// Pause stops automatic curator runs.
func (c *Curator) Pause() {
	s := c.loadState()
	s.Paused = true
	c.saveState(s)
}

// Resume re-enables automatic curator runs.
func (c *Curator) Resume() {
	s := c.loadState()
	s.Paused = false
	c.saveState(s)
}

// MaybeRun checks if a curator pass should run and executes it.
func (c *Curator) MaybeRun(ctx context.Context, idleDuration time.Duration) {
	if !c.cfg.Enabled {
		return
	}
	state := c.loadState()
	if state.Paused {
		return
	}
	if !state.LastRunAt.IsZero() && time.Since(state.LastRunAt) < c.cfg.Interval {
		return
	}
	if idleDuration < c.cfg.MinIdle {
		return
	}
	c.run(ctx)
}

// RunNow forces a curator pass regardless of interval/idle gates.
func (c *Curator) RunNow(ctx context.Context) {
	c.run(ctx)
}

func (c *Curator) run(ctx context.Context) {
	slog.Info("curator: starting pass")

	// Step 1: auto-transitions (pure time-based, no LLM)
	counts := c.autoTransitions()
	autoParts := []string{}
	if counts["archived"] > 0 {
		autoParts = append(autoParts, fmt.Sprintf("%d archived", counts["archived"]))
	}
	if counts["marked_stale"] > 0 {
		autoParts = append(autoParts, fmt.Sprintf("%d stale", counts["marked_stale"]))
	}
	if counts["reactivated"] > 0 {
		autoParts = append(autoParts, fmt.Sprintf("%d reactivated", counts["reactivated"]))
	}
	autoSummary := "no auto changes"
	if len(autoParts) > 0 {
		autoSummary = strings.Join(autoParts, ", ")
	}

	// Step 2: LLM review (merge/consolidate)
	llmSummary := "skipped"
	agentSkills := c.skillMgr.ListAgentSkills()
	if len(agentSkills) > 0 && c.factory != nil {
		result, err := c.llmReview(ctx, agentSkills)
		if err != nil {
			slog.Error("curator: LLM review failed", "error", err)
			llmSummary = fmt.Sprintf("error: %v", err)
		} else {
			llmSummary = result
		}
	}

	summary := fmt.Sprintf("auto: %s; llm: %s", autoSummary, llmSummary)
	slog.Info("curator: pass complete", "summary", summary)

	state := c.loadState()
	state.LastRunAt = time.Now()
	state.RunCount++
	state.LastSummary = summary
	c.saveState(state)
}

// autoTransitions applies time-based state transitions to agent skills.
func (c *Curator) autoTransitions() map[string]int {
	counts := map[string]int{"checked": 0, "archived": 0, "marked_stale": 0, "reactivated": 0}
	agentSkills := c.skillMgr.ListAgentSkills()

	for _, s := range agentSkills {
		counts["checked"]++
		if s.Pinned {
			continue
		}
		// Use directory mod time as last-activity approximation
		info, err := os.Stat(s.Dir)
		if err != nil {
			continue
		}
		lastActivity := info.ModTime()

		if time.Since(lastActivity) > c.cfg.ArchiveAfter {
			if err := c.skillMgr.ArchiveSkill(s.Name); err != nil {
				slog.Error("curator: failed to archive", "skill", s.Name, "error", err)
			} else {
				counts["archived"]++
				slog.Info("curator: archived", "skill", s.Name)
			}
			continue
		}

		isStale := strings.HasPrefix(s.Description, "[stale] ")
		if time.Since(lastActivity) > c.cfg.StaleAfter && !isStale {
			newDesc := "[stale] " + s.Description
			if err := c.skillMgr.SetDescription(s.Name, newDesc); err != nil {
				slog.Error("curator: failed to mark stale", "skill", s.Name, "error", err)
			} else {
				counts["marked_stale"]++
			}
		} else if time.Since(lastActivity) <= c.cfg.StaleAfter && isStale {
			newDesc := strings.TrimPrefix(s.Description, "[stale] ")
			if err := c.skillMgr.SetDescription(s.Name, newDesc); err != nil {
				slog.Error("curator: failed to reactivate", "skill", s.Name, "error", err)
			} else {
				counts["reactivated"]++
			}
		}
	}
	return counts
}

// llmReview runs a background agent to review and consolidate agent skills.
func (c *Curator) llmReview(ctx context.Context, agentSkills []skill.Skill) (string, error) {
	var candidateList strings.Builder
	candidateList.WriteString(fmt.Sprintf("Agent-created skills (%d):\n", len(agentSkills)))
	for _, s := range agentSkills {
		candidateList.WriteString(fmt.Sprintf("- %s: %s (pinned: %t)\n", s.Name, s.Description, s.Pinned))
	}

	prompt := `You are gclaw's skill curator. Review the agent-created skills below and consolidate them.

Rules:
1. Only touch agent-created skills (the list below is already filtered).
2. NEVER delete skills. Archive by calling skill_delete (moves to .archive/).
3. NEVER touch pinned skills.
4. Merge narrow skills into broad umbrella skills using skill_create.
5. After merging, archive the absorbed narrow skills using skill_delete.

How to consolidate:
- Identify clusters of skills sharing a domain/prefix.
- For each cluster, decide: merge into one umbrella (create new via skill_create) or keep as-is.
- After creating umbrella, archive old narrow skills via skill_delete.

Available tools: skill_list, skill_view, skill_create, skill_delete

` + candidateList.String()

	runner := c.factory()
	defer runner.Reset()

	reviewCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	output, err := runner.Run(reviewCtx, prompt)
	if err != nil {
		return "", err
	}

	// Truncate summary
	if len(output) > 500 {
		return output[:500] + "...", nil
	}
	return output, nil
}
