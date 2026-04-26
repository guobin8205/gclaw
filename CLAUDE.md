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

Core execution loop: receive message → call LLM → if tool calls, execute them → feed results back → repeat until the model returns text-only or `MaxTurns` is exhausted.

Key methods:
- `Run(ctx, prompt)` — blocking, returns final text
- `Submit(ctx, message)` — concurrent-safe (mutex-protected busy flag), used by autonomous scheduler and cron
- `Reset()` — clears message history; called before every cron job execution to avoid context accumulation

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

Token estimation via char-count / 4. Auto-compaction at `compact_at` threshold: keeps first message (anchor) + recent N messages, replaces middle with boundary markers.
