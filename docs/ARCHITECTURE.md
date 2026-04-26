# GClaw 项目架构文档

## 项目概述

GClaw 是一个用 Go 编写的高性能自主 AI Agent CLI 工具。它支持多种 LLM 提供商（DeepSeek、GLM、Claude、OpenAI、Ollama），提供交互式对话和自主运行两种模式，内置文件读写、命令执行、代码搜索等工具。

## 目录结构

```
gclaw/
├── cmd/
│   ├── gclaw/           # 主入口 (REPL + 微信 + Cron)
│   └── providertest/    # 提供商连通性测试
├── internal/
│   ├── agent/           # 核心 Agent 循环（多实例隔离）
│   ├── autonomous/      # 自主模式调度器
│   ├── channel/         # 消息通道接口
│   │   └── weixin/      # 微信通道实现
│   ├── config/          # 配置加载与验证
│   ├── context/         # 上下文窗口管理
│   ├── cron/            # 定时任务调度器
│   ├── delegate/        # 子代理委派 (Dispatcher)
│   ├── gateway/         # 统一路由层 (SessionSource)
│   │   └── adapter/     # REPL/微信适配器
│   ├── memory/          # 记忆系统 (Provider 接口)
│   ├── model/           # LLM 模型接口与实现
│   │   ├── claude/      # Anthropic Claude 客户端
│   │   ├── openai/      # OpenAI 及兼容 API 客户端
│   │   ├── ollama/      # 本地 Ollama 客户端
│   │   ├── interface.go # 统一 Model 接口定义
│   │   ├── router.go    # 多模型路由与回退
│   │   └── mock.go      # 测试用 Mock 提供者
│   ├── perm/            # 权限检查器
│   ├── provider/        # 模型工厂 (注册与构建) + 凭证池
│   ├── session/         # SQLite + FTS5 会话持久化
│   ├── skill/           # Skill 文件系统 (YAML frontmatter)
│   ├── task/            # 后台任务管理
│   ├── tool/            # 工具接口与注册表
│   │   └── builtin/     # 内置工具（自注册）
│   │       ├── shell/       # Bash (toolset: shell)
│   │       ├── file_read/   # ReadFile (toolset: read)
│   │       ├── file_write/  # WriteFile (toolset: write)
│   │       ├── search/      # Glob + Grep (toolset: search)
│   │       ├── timetool/    # SleepTool (toolset: time)
│   │       ├── meta/        # 元工具: delegate_task, cron_*, weixin_status, tasks_list
│   │       ├── skill_tools/ # skill_create, skill_delete, skill_list
│   │       └── github_trending/ # GitHub 热点搜索
│   └── remote/          # 远程桥接模式
├── pkg/
│   └── proto/           # gRPC/Protobuf 定义
├── plugins/
│   └── mcp-gateway/     # MCP 协议网关插件
├── docs/                # 文档
└── .gclaw/
    ├── config.yaml      # 项目级配置
    └── skills/          # 项目内置 skills（示例）
```

## 核心架构

### 数据流

```
                    ┌──────────────────────────────────────┐
                    │            main.go                    │
                    │  三个独立 Agent 实例                   │
                    │                                      │
 用户输入 (stdin)    │  ag (REPL)          MaxTurns 100     │
────────────▶       │  weixinAgent         MaxTurns  20     │
                    │  cronAgent           MaxTurns  10     │
 微信消息            │                                      │
────────────▶       │  + Cron 调度器                        │
                    │  + Gateway 统一路由                    │
 Cron 触发           │  + Skill Manager → 注入 system prompt│
────────────▶       │  + Memory Manager → Prefetch/SyncTurn│
                    │  + Session Store (SQLite)             │
                    │  + Dispatcher (子代理池)               │
                    └──────────┬───────────────────────────┘
                               │
                               │ Submit(prompt)
                               ▼
                    ┌──────────────────┐
                    │   agent.Agent    │
                    │   (Agent 循环)    │
                    │   每轮:           │
                    │   1. 调 LLM       │
                    │   2. 执行工具      │
                    │   3. 回传结果      │
                    │   直到无工具调用   │
                    └──────┬───────┘
                           │
              ┌────────────┼────────────┐
              │ Tool Calls  │            │ API Call
              ▼             │            ▼
    ┌──────────────┐        │   ┌─────────────────┐
    │ tool.Registry│        │   │ DeepSeek/GLM/   │
    │ (自注册)      │        │   │ Claude/Ollama   │
    │ ├ shell/*    │        │   └─────────────────┘
    │ ├ read/*     │        │
    │ ├ write/*    │        │
    │ ├ search/*   │        │
    │ ├ meta/*     │        │
    │ └ ...        │        │
    └──────────────┘        │
                            │
  Cron 结果 ──▶ OnResult ──▶ weixinCh.Send(context_token) ──▶ 微信推送
```

### 1. Agent 循环 (`internal/agent/agent.go`)

Agent 是核心执行引擎，循环执行以下步骤：

1. 接收用户消息，追加到消息历史
2. 调用 LLM 模型，传入系统提示词、消息历史和可用工具
3. 如果模型返回文本（无工具调用），循环结束，返回文本
4. 如果模型返回工具调用，执行工具，将结果追加到历史，回到步骤 2
5. 在 `max_turns` 回合内未完成则报错

关键方法：
- `Run(ctx, prompt)` — 非流式执行
- `RunStreaming(ctx, prompt, onText)` — 流式执行（逐 token 回调）
- `Submit(ctx, message)` — 并发安全的提交（用于自主模式）
- `Interrupt(message)` — 向运行中的 Agent 注入中断消息（非阻塞）
- `InterruptAndStop(message, cancel)` — 注入中断并取消 context
- `Reset()` — 清空消息历史，排空中断缓冲，重置上下文管理器

#### 中断系统

Agent 内部维护一个 `chan string`（容量 8）作为中断缓冲。每轮 turn 开头检查：

1. `drainInterrupts()` — 非阻塞排空 channel，合并为一条 `[User Interrupt]` 消息
2. 合并消息追加到消息历史，Agent 在下一轮 LLM 调用时可见
3. 如果缓冲区满，`Interrupt()` 非阻塞丢弃溢出（不死锁）

自主调度器 (`autonomous/scheduler.go`) 的 `handleEvent()` 中：当 Agent 忙时，`EventUser` 和 `EventWebhook` 事件通过 `Interrupt()` 注入，而非直接跳过。

#### 上下文自动压缩

如果 Agent 配置了 `ContextMgr`（`context.Manager`），每轮 turn 还会：
1. `syncToManager()` — 同步消息到上下文管理器
2. 检查 `ShouldCompact()` — 当 token 使用率达到 `compact_at` 阈值
3. 触发 `Compact()` — LLM 智能压缩（优先）或截断（降级）
4. 压缩后从管理器读回消息，替换 Agent 本地历史

### 2. 模型层 (`internal/model/`)

统一接口 `Model` 定义所有 LLM 提供者必须实现的方法：

```go
type Model interface {
    ID() string
    MaxTokens() int
    SupportsThinking() bool
    SupportsStreaming() bool
    SupportsVision() bool
    Stream(ctx, params) (<-chan StreamEvent, error)
    Call(ctx, params) (*Response, error)
    CountTokens(messages) int
}
```

#### 提供者适配

| 提供者 | 类型 | 说明 |
|--------|------|------|
| Claude | `claude` | Anthropic Claude API |
| OpenAI | `openai` | 标准 OpenAI API |
| DeepSeek | `openai-compatible` | 兼容 OpenAI 格式，支持 reasoning_content |
| GLM (智谱) | `openai-compatible` | 兼容 OpenAI 格式，自定义 endpoint |
| Ollama | `ollama` | 本地模型（无需 API Key） |
| Mock | `mock` | 开发测试用，确定性输出 |

#### 模型路由 (`router.go`)

`Router` 支持多模型回退和速率限制：
- 按默认模型 → 回退列表 → 随机可用模型的顺序尝试
- 基于令牌桶的每模型速率限制
- 调用成本追踪（按 $3/$15 每百万 token 估算）

#### 模型工厂 (`internal/provider/factory.go`)

`DefaultFactory()` 自动检测环境变量（`ANTHROPIC_API_KEY`、`OPENAI_API_KEY`、`DEEPSEEK_API_KEY`、`ZHIPU_API_KEY` 等）注册提供商。配置文件中的 provider 合并覆盖，每个模型名在 `models[]` 中独立注册。

`BuildWithPool(name)` 返回 `(Model, *CredentialPool, error)`，调用方可持有 pool 引用在 429 错误时调用 `MarkExhausted()`。单 Key 配置不创建池。

#### 凭证池 (`internal/provider/pool.go`)

多 API Key 轮转，解决单 Key 限速导致 Agent 瘫痪：

```go
type CredentialPool struct {
    creds    []PooledCredential  // 所有凭证
    index    int                 // round-robin 游标
    cooldown time.Duration       // 冷却时间（默认 5 分钟）
}
```

- `Select()` — round-robin 选健康 Key，跳过冷却中的 Key
- `MarkExhausted(key, err)` — 标记 exhausted，设冷却截止时间
- 冷却到期自动恢复为可用

### 3. 工具系统 (`internal/tool/`)

#### 工具接口

```go
type Tool interface {
    Name() string
    Toolset() string              // 工具集分组
    Description() string
    InputSchema() Schema          // JSON Schema 参数定义
    Check() bool                  // 可用性检测（false 则不暴露给模型）
    ConcurrencySafe() bool        // 是否并发安全
    RequiresApproval(params) bool // 是否需要权限确认
    Execute(ctx, params) (ToolResult, error)
}
```

#### 自注册模式

工具通过 `init()` 函数自注册到全局 Registry：

```go
// internal/tool/builtin/shell/tool.go
func init() {
    tool.GlobalRegistry.Register(&BashTool{})
}
```

`main.go` 通过 blank import 触发注册：

```go
import (
    _ "github.com/openclaw/gclaw/internal/tool/builtin/shell"
    _ "github.com/openclaw/gclaw/internal/tool/builtin/file_read"
    // ...
)
```

#### 工具集分组

| 工具集 | 工具 | 用途 |
|--------|------|------|
| `shell` | Bash | 执行 Shell 命令（需审批） |
| `read` | ReadFile | 读取文件内容 |
| `write` | WriteFile | 创建/覆写文件（需审批） |
| `search` | Glob, Grep | 文件搜索 |
| `time` | SleepTool | 自主模式休眠（需 Sleeper） |
| `meta` | delegate_task, cron_list, cron_run, weixin_status, tasks_list | 元工具 |
| `skill` | skill_list, skill_create, skill_delete | Skill 管理 |

`Check()` 方法控制工具是否暴露：例如 SleepTool 仅在自主模式下有 Sleeper 时返回 true。

#### Registry 方法

```go
tool.GlobalRegistry.AvailableTools()   // 只返回 Check()==true 的工具
tool.GlobalRegistry.ListByToolset("meta")  // 按工具集筛选
tool.GlobalRegistry.Toolsets()         // 列出所有工具集
```

### 4. Skill 系统 (`internal/skill/`)

Skill 是可复用的程序性知识单元，以 YAML frontmatter + Markdown 格式存储。三种来源，优先级从低到高：

```
~/.gclaw/skills/           # 用户 skills 目录
├── user/                  # 用户手写
│   └── my-skill/
│       └── SKILL.md
└── agent/                 # Agent 自创建
    └── some-skill/
        └── SKILL.md

.gclaw/skills/             # 项目内置 skills（随代码分发，优先级最低）
└── github-trending/
    └── SKILL.md
```

加载顺序：project → user → agent（后加载覆盖同名的先加载），即 agent > user > project。

#### SKILL.md 格式

```markdown
---
name: my-skill
description: 技能描述
---

## 指令

当用户要求...时，执行以下操作：
...
```

#### 注入机制

`Manager.LoadAll()` 扫描目录 → 解析 frontmatter → `ForSystemPrompt()` 拼装注入 Agent 的 system prompt。

注入范围：
- 主 REPL Agent（`ag`）
- Cron Agent（`cronAgent`）

Agent 可通过 `skill_create`/`skill_delete`/`skill_list` 工具动态管理 skills。

### 5. 记忆系统 (`internal/memory/`)

#### Provider 接口

```go
type Provider interface {
    Available() bool
    Initialize(sessionID string) error
    Prefetch(ctx, query) (string, error)      // 搜索相关记忆
    SyncTurn(ctx, userMsg, assistantMsg) error // 写入记忆
    SystemPromptBlock() string
    Shutdown() error
}
```

#### 数据流

```
用户消息 → Prefetch(消息) → 搜索 memory → 注入 system prompt
         → LLM 处理
         → SyncTurn(用户消息, 助手回复) → 写 memory
```

#### File Provider

本地文件实现，复用 Claude Code memory 格式（`MEMORY.md` 索引 + 独立 `.md`），存储于 `~/.gclaw/memory/`。

`Manager` 支持多 Provider fan-out，单个失败不影响其他。

### 6. 会话持久化 (`internal/session/`)

基于 `modernc.org/sqlite`（纯 Go，无 CGO）的 SQLite + FTS5 全文搜索：

```sql
CREATE TABLE sessions (id TEXT PRIMARY KEY, agent_type TEXT, ...);
CREATE TABLE messages (id INTEGER PRIMARY KEY, session_id TEXT, content TEXT, ...);
CREATE VIRTUAL TABLE messages_fts USING fts5(content, content='messages');
```

`Store` 接口支持 `Search(query, limit)` 全文搜索历史对话。

### 7. Gateway (`internal/gateway/`)

统一路由层，抽象平台差异：

```go
type SessionSource struct {
    Platform string // "repl" | "weixin" | ...
    ChatID   string
    ChatName string
    UserID   string
    ThreadID string
}
```

目前接入 REPL 和微信两个平台，接口预留扩展。

### 8. 子代理委派 (`internal/delegate/`)

`Dispatcher` 管理子代理 goroutine 池：

```go
type Dispatcher struct {
    factory      AgentFactory       // 创建子代理
    sem          chan struct{}       // 并发信号量
    maxDepth     int                // 最大嵌套深度（默认2）
    blockedTools map[string]bool    // 子代理禁用工具
}
```

- `delegate_task` 元工具让主 Agent 能委派子任务
- 子代理禁用 `delegate_task`（防止递归）
- `AgentRunner` 接口：`Run(ctx, prompt) (string, error)` + `Reset()`

### 9. 权限系统 (`internal/perm/perm.go`)

采用洋葱模型，4 种模式：

| 模式 | 行为 |
|------|------|
| `default` | 根据规则列表进行 glob 匹配（允许列表 + 禁止列表） |
| `auto` | 全部自动批准 |
| `strict` | 所有 `ask` 规则变为需要确认 |
| `plan` | 仅允许读操作 |

规则支持通配符模式：
- `Bash(git:*)` — 匹配所有 git 命令
- `Bash(rm -rf *)` — 禁止危险删除
- `**` — 匹配所有工具

### 10. 自主模式 (`internal/autonomous/`)

三种运行级别：

| 级别 | 行为 |
|------|------|
| `interactive` | 等待用户输入，逐条响应 |
| `semi` | 用户设定目标，Agent 计划并执行，关键决策需确认 |
| `full` | 心跳驱动，完全自主执行，仅遇到问题时通知用户 |

核心组件：
- **EventBus** — 发布/订阅事件系统
- **Ticker** — 定时心跳触发器
- **Sleeper** — Agent 空闲休眠管理器
- **Scheduler** — 协调 Ticker、Sleeper、EventBus 和 Agent 的主调度器

事件类型：`tick`（心跳）、`file`（文件变更）、`webhook`（外部回调）、`cron`（定时任务）、`user`（用户输入）、`wake`（休眠唤醒）

### 11. 上下文管理 (`internal/context/manager.go`)

- Token 估算：基于字符数 / 4 的粗略估算
- 压缩触发：当 token 使用率达到 `compact_at` 阈值时触发
- **双模式压缩**：
  - **LLM 智能压缩**（优先）— 用辅助模型生成结构化摘要，替代中间消息
  - **截断降级** — 无压缩模型或压缩失败时，保留首尾，中间替换为边界标记

#### LLM 智能压缩 (`internal/context/compactor.go`)

`Compactor` 接口定义压缩协议：

```go
type Compactor interface {
    Compact(messages []model.Message, previousSummary string) (*CompressionResult, error)
}
```

`LLMCompactor` 实现：
1. 分割消息：保护前 N 条（keepFirst=1）+ 中间（待压缩）+ 保护后 N 条（keepRecent=10）
2. 构建摘要 prompt（含前次摘要，支持迭代精炼）
3. 调用压缩模型（通常是便宜的 flash 模型）
4. 摘要替换中间消息，输出 8 个结构化段落：
   - Active Task / Completed Actions / Active State / In Progress
   - Blocked / Key Decisions / Pending Items / Critical Context

反震荡保护：最近 2 次压缩节省 token < 10% 则跳过本次压缩。

降级路径：压缩模型调用失败时自动回退到截断模式。

### 12. 任务管理 (`internal/task/`)

异步后台任务系统：
- 任务类型：`local_bash`、`local_agent`、`remote_agent`、`cron_task`、`monitor_task`
- 生命周期：`pending` → `running` → `completed` / `failed` / `killed`
- 并发控制：信号量限制最大并发数
- 依赖管理：任务 DAG（blockedBy / blocks）

### 13. 配置系统 (`internal/config/config.go`)

配置优先级（从低到高）：`默认值` → `用户配置` → `项目配置` → `环境变量`

配置段：
- `model` — 模型提供者、默认模型、回退链
- `agent` — 自主级别、心跳间隔、最大轮数
- `context` — Token 窗口、压缩阈值
- `permission` — 权限模式、规则列表
- `tools` — 禁用工具/工具集
- `skills` — Skill 文件系统
- `memory` — 记忆 Provider
- `session` — SQLite 会话持久化
- `gateway` — 统一路由层
- `delegate` — 子代理委派
- `channels` — 微信通道
- `cron` — 定时任务
- `plugins` — MCP 服务器
- `logging` — 日志级别、审计、OTEL

配置发现：
- 项目配置：从当前目录向上查找 `.gclaw/config.yaml`
- 用户配置：`~/.gclaw/config.yaml`
- 环境变量：`GCLAW_MODEL`、`GCLAW_AUTONOMY`、`GCLAW_PERMISSION_MODE`、`GCLAW_LOG_LEVEL`

`merge()` 函数负责合并所有配置段（包括 Skills、Memory、Session、Gateway、Delegate）。

支持 `${ENV_VAR}` 语法在 YAML 中引用环境变量。

### 14. 消息通道 (`internal/channel/`)

`Channel` 接口定义了消息通道的统一抽象：

```go
type Channel interface {
    ID() string
    Start(ctx, bus) error
    Stop() error
    Send(ctx, to, text string) error
    Status() Status
}
```

#### 微信通道 (`internal/channel/weixin/`)

通过 ilink 协议接入微信机器人，实现双向消息：

- **扫码登录**：`/ilink/bot/get_bot_qrcode` → 轮询确认 → 保存 token
- **长轮询收消息**：`POST /cgi-bin/bot/get_updates`，30s 超时，收到后立即发起下一次
- **发消息**：`POST /cgi-bin/bot/send_message`，需 `from_user_id: ""` + `client_id`（UUID）
- **context_token 复用**：存储最后一次用户消息的 context_token，主动推送时复用（空 token API 返回 `ret:-2`）
- **错误码检查**：解析响应 JSON 的 `ret` 字段，`ret != 0` 时返回 error
- **ChatUserID 持久化**：最后发消息的用户 ID 写入 `AccountData.ChatUserID`，重启后恢复
- **独立 Agent**：持有专属 `weixinAgent` 实例，与 REPL 互不阻塞

消息处理流程：`收消息 → agent.Submit() → Agent 循环 → 回复 → OnMessageHandled 回调`

### 15. 定时任务 (`internal/cron/`)

Cron 调度器每秒检查一次，到点触发 Job：

```go
type Job struct {
    Name          string    // 唯一标识
    Schedule      string    // 5 字段 cron 表达式
    Prompt        string    // 发给 Agent 的指令
    Enabled       bool
    NotifyWeixin  bool      // 完成后是否推送到微信
    OnResult      func(name, prompt, response string)
}

type Executor interface {
    Submit(ctx, prompt) (string, error)
    IsBusy() bool
    Reset()  // 清空 Agent 消息历史
}
```

执行流程：
1. 检查 `NextRun` 是否到期
2. 检查 Executor 是否忙碌（忙则跳过本次）
3. `executor.Reset()` 清空 Agent 历史
4. `executor.Submit(ctx, prompt)` 执行
5. 执行完毕后调用 `Job.OnResult`（如微信推送）

`RunNow()` 方法供 `cron_run` 元工具调用，立即执行指定 Job。

Cron Agent 的 system prompt 包含所有 skill 指令，prompt 可直接引用 skill。

Cron Agent 可配置独立模型（`cron.model`），不占用主对话的消息历史。

## 关键设计决策

1. **多 Agent 实例隔离** — REPL、微信、Cron 各用独立 Agent 实例，互不阻塞，消息历史隔离
2. **Cron 模型独立** — 通过 `cron.model` 可为定时任务指定轻量模型（如 `deepseek-v4-flash`），降低 API 成本
3. **Agent.Reset()** — Cron 执行前清空消息历史，避免多轮累积导致的 token 爆炸和上下文污染
4. **工具自注册** — 通过 `init()` + blank import 实现工具自注册，添加新工具无需改 main.go
5. **Skill 注入 Cron** — Cron Agent 的 system prompt 包含所有 skill，prompt 可直接引用 skill 指令
6. **context_token 复用** — 微信主动推送需要有效 context_token，从最近用户消息中获取并复用
7. **OpenAI 兼容适配** — DeepSeek、GLM 等国内模型通过统一 `openai-compatible` 类型接入，降低维护成本
8. **reasoning_content 透传** — DeepSeek V4 的推理内容需原样回传，Message 结构中专门保留了 `ReasoningContent` 字段
9. **双模式 Agent** — 同一 Agent 实例支持 interactive 和 autonomous 两种模式，通过 `Submit()` 的并发锁保护
10. **纯 Go SQLite** — 使用 `modernc.org/sqlite`，无 CGO 依赖，Windows 交叉编译零配置
11. **Shell 自适应** — Bash 工具自动检测可用 Shell（powershell > sh > bash > cmd），并转换 UTF-16 输出
12. **多凭证轮转** — 同一 provider 多 API Key，round-robin + 429 自动冷却切换，单 Key 不创建池
13. **中断注入** — 缓冲 channel 容量 8，非阻塞发送，drain-merge 合并多条中断为一则消息
14. **LLM 智能压缩** — 辅助模型生成 8 段结构化摘要替代中间消息，失败自动降级截断，反震荡防浪费
