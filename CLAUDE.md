# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
# Build
go build -o gclaw.exe ./cmd/gclaw/

# Run all tests
go test ./...

# Run a single package's tests
go test ./internal/cron/

# Run a single test
go test ./internal/cron/ -run TestParse

# Run with race detection
go test -race ./...
```

Module: `github.com/openclaw/gclaw` — Go 1.26. Key external dependencies: `gopkg.in/yaml.v3`, `charm.land/bubbletea/v2` (Bubble Tea v2 TUI framework), `charm.land/lipgloss/v2` (terminal styling).

**Cross-compile for Linux:**
```powershell
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o gclaw-linux ./cmd/gclaw/
```

## Architecture

### Three-Agent Isolation

The CLI (`cmd/gclaw/main.go`) creates **three independent `agent.Agent` instances** with separate message histories, models, and turn limits:

| Agent | MaxTurns | Purpose | Model |
|-------|----------|---------|-------|
| `ag` | 100 | REPL interactive loop (TUI) | `model.default` |
| `weixinAgent` | 20 | WeChat message handling | `model.default` |
| `cronAgent` | 10 | Cron job execution | `cron.model` (falls back to default) |

Each runs independently — WeChat and cron never block the REPL, and vice versa.

### TUI (`internal/tui/`)

Full Bubble Tea v2 TUI replacing the original `bufio.Scanner` REPL. Layout: banner → transcript (virtual scroll + scrollbar) → divider → queue preview → completions → composer → divider → status bar → hints.

AltScreen mode with `WindowSizeMsg`-based full redraw on text change to work around ultraviolet incremental renderer CJK wide-character ghosting bug.

**Components:**
- `App` — root `tea.Model`, dispatches key/mouse events, manages busy/idle state, completion state, attachment mode, queue, cancel function for interrupt, mouse selection state
- `Transcript` — virtual scrolling with right-side scrollbar, renders 4 message kinds (User, Assistant, ToolCall, Event), banner, thinking fold
- `Composer` — multi-line input with `[]rune` storage (correct multi-byte/Chinese support), attachments, multi-line cursor movement (Up/Down moves within text; history only at boundaries via `AtFirstLineStart`/`AtLastLineEnd`), Ctrl+V paste support
- `StatusBar` — left-aligned `● model │ ctx │ agents │ bg │ cron │ time` with live token/cron/agent data
- `ApprovalRequest` — tool approval popup (Y/N/A/Esc)
- `CompletionEngine` — slash command matching with `/` prefix, scrolling dropdown (max 8 visible, auto-scrolls to keep selection in view), Tab/↑↓/Esc
- `Selection` — mouse drag selection with inverse-video highlighting, right-click copy to clipboard, CJK-aware visual column calculation
- `History` — persistent command history (`~/.gclaw/history`), search, dedup, saved on exit
- `LogBuffer` — ring buffer with `slog.Handler` integration, `/logs [N]` command
- `RenderMarkdown` — code blocks with border + syntax highlighting, tables, headings, bold, italic, inline code, links

**Streaming:** `runAgentCmd` uses `Agent.RunStreaming` with `tea.Program.Send` callback to push `streamChunkMsg` for real-time text display with blinking cursor `▌`. `SetSend()` is called after `tea.NewProgram` creation to wire the callback. A cancellable `context.Context` is created in `submitInput` and stored in `App.cancelFn` for interrupt support.

**Themes:** `tokyo-night` (default), `catppuccin-mocha`, `light`, `terminal`. Configurable via `tui.theme` in config.yaml.

**Key bindings:** Enter=send, Ctrl+Enter/Ctrl+J=newline, Esc=clear/close completions/clear selection, Ctrl+C=interrupt (cancelFn + InterruptAndStop)/quit, Ctrl+L=clear transcript, ↑↓=move cursor within composer, history only at boundaries, PgUp/PgDn=scroll, Tab=apply completion, Ctrl+I=attach file, Ctrl+V=paste, Ctrl+O=toggle thinking/tool fold, mouse wheel=scroll, mouse drag=select text, right-click=copy selection to clipboard.

**Queue:** Slash commands typed while agent is busy are queued and auto-executed when agent finishes. Queue preview shows above composer.

**Thinking fold:** DeepSeek ReasoningContent renders as `▸ 💭 思考过程 · Ctrl+O 展开`, collapsed by default.

**Tool Call Tree:** Nested `┊` indentation with status icons (✓/✕/⠋), output folding with line count.

### Agent Loop (`internal/agent/agent.go`)

Core execution loop: receive message → check interrupts → auto-compact if needed → call LLM → if tool calls, execute them → feed results back → repeat until the model returns text-only or `MaxTurns` is exhausted.

Key methods:
- `Run(ctx, prompt)` — blocking, returns final text
- `RunStreaming(ctx, prompt, onText)` — streaming variant
- `Submit(ctx, message)` — concurrent-safe (mutex-protected busy flag), used by autonomous scheduler and cron
- `Interrupt(message)` — injects a message into a running agent loop between turns (non-blocking, buffered channel)
- `InterruptAndStop(message, cancel)` — inject + cancel context
- `Reset()` — clears message history, drains interrupt channel, resets context manager

**Interrupt System**: Each agent has a buffered channel (`interruptCh`, cap 8). Between every turn, `drainInterrupts()` merges pending interrupts into a single `[User Interrupt]` user message. The autonomous scheduler uses this to inject high-priority events (EventUser, EventWebhook) when the agent is busy, instead of skipping them.

### Model Layer (`internal/model/`)

`Model` interface: `ID()`, `MaxTokens()`, `Call()`, `Stream()`, `CountTokens()`. Four backend types:

- `claude` — Anthropic Claude API
- `openai` — Standard OpenAI API
- `openai-compatible` — DeepSeek, GLM, Moonshot, Qianfan (all share the OpenAI client with endpoint/base URL overrides)
- `ollama` — Local models, no API key needed

**`Message.ReasoningContent`** — DeepSeek-R1/V4 return thinking tokens that must be echoed back in the next request's assistant message. The field is in `model.Message` and `model.Response`, and the OpenAI client passes it through.

### Provider Factory (`internal/provider/factory.go`)

`DefaultFactory()` auto-detects providers from environment variables (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `DEEPSEEK_API_KEY`, `ZHIPU_API_KEY`, etc.). Config-file providers are then merged on top, allowing overrides. Each model name in `provider.models[]` is registered as a separate entry in the factory.

Model resolution order: configured `model.default` → `model.fallback[]` list → first remaining available provider.

### Credential Pool (`internal/provider/pool.go`)

`CredentialPool` manages round-robin rotation across multiple API keys for a single provider. When `ProviderConfig.Keys` has multiple entries, the factory creates a pool instead of using a single key.

- `Select()` — returns next healthy key via round-robin, skips exhausted keys in cooldown
- `MarkExhausted(key, err)` — marks a key as failed (e.g. 429 rate limit), sets cooldown timer (default 5m)
- `Recover()` — resets keys whose cooldown has expired
- `BuildWithPool(name)` — factory method that returns both the model and the pool for later exhaustion marking

### Tools (`internal/tool/`)

`Tool` interface: `Name()`, `Description()`, `InputSchema()`, `ConcurrencySafe()`, `RequiresApproval()`, `Execute()`.

Built-in tools: `ReadFile`, `WriteFile`, `Patch`, `Bash`, `Glob`, `Grep`, `Todo`, `Memory`, `Clarify`, `WebSearch`, `WebExtract`, `SessionSearch`, `Vision`, `ImageGen`, `TTS`, `Video`, `CodeExecution`, `SleepTool`, plus browser tools (`browser_navigate`, `browser_snapshot`, `browser_click`, `browser_type`, `browser_scroll`, `browser_press`, `browser_screenshot`), MCP tools (`mcp_list_servers`, `mcp_discover`, `mcp_call`), skill tools (`skill_create`, `skill_delete`, `skill_list`), and meta tools (`delegate_task`, `cron_list`, `cron_run`, `tasks_list`, `weixin_status`).

**Bash tool shell resolution** (Windows-aware): tries `powershell` → `sh` → `bash` → `cmd`. Output is sanitized for UTF-16 LE/BE → UTF-8 conversion (WSL's `bash.exe` outputs UTF-16). The tool description dynamically reports the active shell to the model.

**Tool Registry** (`internal/tool/interface.go`): `GlobalRegistry` holds all tools registered via `init()`. Key methods: `List()`, `AllTools()`, `Names()`, `Toolsets()`, `Get(name)`, `AvailableTools()`. Tools self-register via `init()` functions in each tool package.

### Permission System (`internal/perm/`)

Four modes: `default` (rule-based glob matching), `auto` (allow all), `strict` (ask rules require confirmation), `plan` (read-only). Rules match glob patterns like `Bash(git:*)` or `Bash(rm -rf *)`.

### Autonomous Mode (`internal/autonomous/`)

Three levels: `interactive` (wait for user), `semi` (plan-execute with confirmation gates), `full` (ticker-driven, self-directed with SleepTool).

The `Scheduler` runs an event loop consuming from `EventBus` (tick/file/webhook/cron/user/wake events). The `Sleeper` manages idle sleep; `SleepTool` is wired to the sleeper so the model can call it.

### Cron (`internal/cron/`)

`Executor` interface (`Submit`/`IsBusy`/`Reset`) is implemented by `agent.Agent`. The scheduler checks every second, skips jobs when the executor is busy, and calls `Reset()` before each execution for a clean context. Each `Job` has an `OnResult` callback used for WeChat push.

**No-Agent Script Mode**: Jobs with a `Script` field execute the script first. If the script output is empty or the last line contains `{"wakeAgent": false}`, the LLM agent is skipped (zero token cost). Otherwise, script output is injected into the agent prompt. `PauseJob(name)` and `ResumeJob(name)` allow runtime control.

### WeChat Channel (`internal/channel/weixin/`)

Communicates with `ilinkai.weixin.qq.com` via long-poll HTTP. Critical protocol detail: `from_user_id: ""` (empty string) AND `client_id` (UUID) are both required in SendMessage — the API returns HTTP 200 without them but the message never arrives. `ChatUserID` is persisted to `~/.gclaw/weixin/accounts/<bot-id>.json` and restored on restart for cron push targeting.

### Config (`internal/config/config.go`)

Priority (high to low): environment variables → `.gclaw/config.yaml` (found by walking up from cwd) → `~/.gclaw/config.yaml` → defaults. YAML values support `${ENV_VAR}` interpolation.

### Context Management (`internal/context/manager.go`)

Token estimation via char-count / 4. Auto-compaction at `compact_at` threshold.

**Two compaction strategies:**
1. **LLM Compression** (`compactor.go`) — when `context.compressor_enabled: true` and `context.compressor_model` is set, uses a secondary model to generate structured summaries with 8 sections (Active Task, Completed Actions, Active State, In Progress, Blocked, Key Decisions, Pending Items, Critical Context). Previous summaries are passed for iterative refinement. Anti-thrashing: skips if last 2 compressions saved < 10%.
2. **Truncation** (legacy) — keeps first message (anchor) + recent N messages, replaces middle with boundary markers. Used when no compressor is configured or when LLM compression fails.

The agent auto-compact checks before each model call. Messages are synced to the context manager via `syncToManager()`, compacted if `ShouldCompact()` returns true, then read back.

### Skill System (`internal/skill/`)

Skills are reusable procedural knowledge units stored as YAML frontmatter + Markdown (`SKILL.md`) files. Three sources with priority (low→high):

| Source | Location | Editable |
|--------|----------|----------|
| `project` | `.gclaw/skills/` (alongside config.yaml) | Shipped with repo, not by agent |
| `user` | `~/.gclaw/skills/user/` | By user |
| `agent` | `<skills.dir>/agent/` | By agent via `skill_create` tool |

`Manager.LoadAll(baseDir)` scans `baseDir/user` and `baseDir/agent`. `Manager.LoadProject(dir)` scans a flat directory of skill subdirectories and marks them source `project`. Project skills load first (lowest priority), so user/agent skills can override them by name.

`Parse(dir)` reads `SKILL.md`, splits frontmatter via `---` delimiters, infers source from path (looks for `user`/`agent` segment). Skills are injected into system prompt via `ForSystemPrompt()` which formats all loaded skill bodies under `## Active Skills`.

`DeleteSkill` only allows deleting `agent`-source skills. Tools: `skill_create`, `skill_delete`, `skill_list` (in `internal/tool/builtin/skill_tools/`).

**Builtin Skills** (`internal/skill/builtin.go`): 5 embedded skills shipped via `go:embed` from `internal/skill/builtin/`. `InstallBuiltin(destDir)` copies them to `<destDir>/user/` on first run (won't overwrite existing). Categories: `software-development/plan`, `software-development/systematic-debugging`, `software-development/test-driven-development`, `creative/architecture-diagram`, `devops/kanban-orchestrator`.

**Skill Metadata**: `Pinned` field (YAML tag `pinned: true`) protects skills from curator actions. `Source` field tracks origin (`user`/`agent`/`project`).

**Archive**: `ArchiveSkill(name)` moves `<agentDir>/<name>/` to `<skillDir>/.archive/<name>/` (non-destructive). `UnarchiveSkill(name)` restores. `scanDir` skips `.archive` directories.

### Curator (`internal/curator/`)

Automatic maintenance of agent-created skills. Two-phase operation:

1. **Auto-transitions** (pure time-based, no LLM): skills inactive > `stale_after_days` get `[stale]` prefix on description; skills inactive > `archive_after_days` are moved to `.archive/`; stale skills that become active again are reactivated (prefix removed). Pinned skills are never touched.
2. **LLM review** (via `delegate.AgentFactory`): spawns a sub-agent to scan agent skills, merge narrow skills into umbrella skills, and archive the absorbed ones. Agent uses `skill_list`, `skill_view`, `skill_create`, `skill_delete` tools.

`MaybeRun(ctx, idleDuration)` gates: paused check → interval check → min-idle check → execute. State persisted to `<skillDir>/.curator_state` (JSON: `last_run_at`, `run_count`, `paused`, `last_summary`).

Config (`internal/config/config.go` — `CuratorConfig`):
```yaml
curator:
  enabled: true
  interval_hours: 168      # 7 days
  min_idle_hours: 2
  stale_after_days: 30
  archive_after_days: 90
```

Wired via `scheduler.SetOnIdleHook(fn)` — curator's `MaybeRun` is called on each `EventTick` when idle. Only operates on `source=agent` skills; never touches user/project/builtin skills.

### Slash Commands

The REPL supports slash commands via `handleCommand` in `cmd/gclaw/main.go`. Commands receive a `cmdCtx` struct with all runtime dependencies (config, agent, providers, memory, skills, sessions, MCP, gateway, cron, etc.).

**Session**: `/clear`, `/compact`, `/interrupt <msg>`
**Info**: `/help`, `/version`, `/status`, `/stats`, `/config`, `/model [name]`, `/fallback [models...]`, `/tools [all]`, `/theme [name]`
**Subsystems**: `/skills`, `/memory [list|clear]`, `/sessions`, `/mcp`, `/cron [run|pause|resume] <name>`, `/tasks`
**Channels**: `/weixin login|logout|status`, `/gateway`
**Diagnostics**: `/doctor`, `/debug`, `/dump`, `/backup`, `/logs [N]`
**Scheduling**: `/autonomy`
**Curator**: `/curator status|run|pause|resume|restore <name>`
**Exit**: `/exit`

`/model <name>` calls `agent.SetModel(m)` to hot-swap the model at runtime. `/theme <name>` calls `app.SetTheme(name)` to hot-swap the TUI theme at runtime. `/status` shows a comprehensive panel aggregating all subsystem states. `/doctor` checks config, model connectivity, and disk space. `/logs [N]` shows the last N entries from the ring-buffer log (default 20).

### Checkpoint Manager (`internal/checkpoint/`)

Creates git shadow repos in `~/.gclaw/checkpoints/{dir_hash}/repo.git` using `GIT_DIR`/`GIT_WORK_TREE` isolation. Automatically snapshots before `WriteFile`, `Patch`, and destructive `Bash` commands. Per-turn dedup via `checkpointed` map cleared by `NewTurn()`.

- `EnsureCheckpoint(dir, reason)` — auto-snapshot if directory not yet checkpointed this turn
- `Restore(dir, hash)` — restore to a specific snapshot
- `List(dir)` — list snapshots
- `Diff(dir, hash)` — show diff between snapshot and current state

### Delegate (`internal/delegate/`)

`Dispatcher` supports single (`Delegate`) and batch (`BatchDelegate`) sub-agent execution. Batch mode uses `sync.WaitGroup` with semaphore-controlled concurrency. Results are returned as a JSON array indexed to input tasks. `AgentFactory` creates fresh agent instances; `MaxDepth` prevents recursive delegation.

### Backend Abstraction Pattern

Used for optional subsystems with multiple implementations:

| Package | Interface | Backends |
|---------|-----------|----------|
| `internal/websearch/` | `Backend` | Tavily, Exa |
| `internal/imagegen/` | `Backend` | FAL.ai, OpenAI DALL-E |
| `internal/tts/` | `Backend` | OpenAI TTS |
| `internal/browser/` | `Browser` | chromedp |
| `internal/mcp/` | `Manager`/`Client` | stdio, HTTP (JSON-RPC 2.0) |

Each has a `Factory` with `NewDefaultFactory(backend)` and `Available()` for auto-detection. Global references (e.g., `webtool.SearchFactory`, `visiontool.ModelRef`) are set in `main.go`.

### Agent Runtime Methods

- `SetModel(m)` — hot-swap the model at runtime (used by `/model` command)
- `Reset()` — clears message history, drains interrupts, resets context
- `Run(ctx, prompt)` / `RunStreaming(ctx, prompt, onText)` — execute agent loop
- `Submit(ctx, message)` — concurrent-safe submission
- `Interrupt(message)` / `InterruptAndStop(message, cancel)` — inject messages into running loop
- `IsBusy()` — check if agent is currently executing
- `Usage()` — returns `model.Usage` with input/output token counts
