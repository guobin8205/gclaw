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

Module: `github.com/openclaw/gclaw` — Go 1.26. Only external dependency is `gopkg.in/yaml.v3`.

## Architecture

### Three-Agent Isolation

The CLI (`cmd/gclaw/main.go`) creates **three independent `agent.Agent` instances** with separate message histories, models, and turn limits:

| Agent | MaxTurns | Purpose | Model |
|-------|----------|---------|-------|
| `ag` | 100 | REPL interactive loop | `model.default` |
| `weixinAgent` | 20 | WeChat message handling | `model.default` |
| `cronAgent` | 10 | Cron job execution | `cron.model` (falls back to default) |

Each runs independently — WeChat and cron never block the REPL, and vice versa.

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

Built-in tools: `ReadFile`, `WriteFile`, `Bash`, `Glob`, `Grep`, `SleepTool` (registered only in semi/full autonomous mode).

**Bash tool shell resolution** (Windows-aware): tries `powershell` → `sh` → `bash` → `cmd`. Output is sanitized for UTF-16 LE/BE → UTF-8 conversion (WSL's `bash.exe` outputs UTF-16). The tool description dynamically reports the active shell to the model.

### Permission System (`internal/perm/`)

Four modes: `default` (rule-based glob matching), `auto` (allow all), `strict` (ask rules require confirmation), `plan` (read-only). Rules match glob patterns like `Bash(git:*)` or `Bash(rm -rf *)`.

### Autonomous Mode (`internal/autonomous/`)

Three levels: `interactive` (wait for user), `semi` (plan-execute with confirmation gates), `full` (ticker-driven, self-directed with SleepTool).

The `Scheduler` runs an event loop consuming from `EventBus` (tick/file/webhook/cron/user/wake events). The `Sleeper` manages idle sleep; `SleepTool` is wired to the sleeper so the model can call it.

### Cron (`internal/cron/`)

`Executor` interface (`Submit`/`IsBusy`/`Reset`) is implemented by `agent.Agent`. The scheduler checks every second, skips jobs when the executor is busy, and calls `Reset()` before each execution for a clean context. Each `Job` has an `OnResult` callback used for WeChat push.

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
