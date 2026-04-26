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
│   ├── model/           # LLM 模型接口与实现
│   │   ├── claude/      # Anthropic Claude 客户端
│   │   ├── openai/      # OpenAI 及兼容 API 客户端
│   │   ├── ollama/      # 本地 Ollama 客户端
│   │   ├── interface.go # 统一 Model 接口定义
│   │   ├── router.go    # 多模型路由与回退
│   │   └── mock.go      # 测试用 Mock 提供者
│   ├── perm/            # 权限检查器
│   ├── provider/        # 模型工厂 (注册与构建)
│   ├── task/            # 后台任务管理
│   ├── tool/            # 工具接口与注册表
│   │   └── builtin/     # 内置工具实现
│   └── remote/          # 远程桥接模式
├── pkg/
│   └── proto/           # gRPC/Protobuf 定义
├── plugins/
│   └── mcp-gateway/     # MCP 协议网关插件
├── docs/                # 文档
└── .gclaw/
    └── config.yaml      # 项目级配置
```

## 核心架构

### 数据流

```
                    ┌──────────────────────────────┐
                    │          main.go              │
                    │  三个独立 Agent 实例           │
                    │                              │
 用户输入 (stdin)    │  ag (REPL)      MaxTurns 100 │
────────────▶       │  weixinAgent     MaxTurns  20 │
                    │  cronAgent       MaxTurns  10 │
 微信消息            │                              │
────────────▶       │  + Cron 调度器                │
                    │  + 微信 Channel               │
 Cron 触发           │                              │
────────────▶       └──────────┬───────────────────┘
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
    │ ├ ReadFile   │        │   │ Claude/Ollama   │
    │ ├ WriteFile  │        │   └─────────────────┘
    │ ├ Bash       │        │
    │ ├ Glob       │        │
    │ └ Grep       │        │
    └──────────────┘        │
                            │
  Cron 结果 ──▶ OnResult ──▶ weixinCh.Send() ──▶ 微信推送
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

### 3. 工具系统 (`internal/tool/`)

#### 工具接口

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() Schema      // JSON Schema 参数定义
    ConcurrencySafe() bool    // 是否并发安全
    RequiresApproval(params) bool // 是否需要权限确认
    Execute(ctx, params) (ToolResult, error)
}
```

#### 内置工具

| 工具 | 用途 | 需要审批 |
|------|------|---------|
| `ReadFile` | 读取文件内容 | 否 |
| `WriteFile` | 创建/覆写文件 | 是 |
| `Bash` | 执行 Shell 命令 | 部分（安全命令自动通过） |
| `Glob` | 文件名模式匹配 | 否 |
| `Grep` | 正则搜索文件内容 | 否 |
| `SleepTool` | 自主模式休眠 | 否 |

#### Bash 安全命令白名单

`git status`, `git diff`, `git log`, `ls`, `cat`, `echo`, `pwd`, `whoami`, `which` 自动放行，其余需要审批。

### 4. 权限系统 (`internal/perm/perm.go`)

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

### 5. 自主模式 (`internal/autonomous/`)

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

### 6. 上下文管理 (`internal/context/manager.go`)

- Token 估算：基于字符数 / 4 的粗略估算
- 压缩触发：当 token 使用率达到 `compact_at` 阈值时触发
- 压缩策略：保留第一条消息（上下文锚点）和最近 N 条消息，中间替换为标记

### 7. 任务管理 (`internal/task/`)

异步后台任务系统：
- 任务类型：`local_bash`、`local_agent`、`remote_agent`、`cron_task`、`monitor_task`
- 声明周期：`pending` → `running` → `completed` / `failed` / `killed`
- 并发控制：信号量限制最大并发数
- 依赖管理：任务 DAG（blockedBy / blocks）

### 10. 配置系统 (`internal/config/config.go`)

配置优先级（从低到高）：`默认值` → `用户配置` → `项目配置` → `环境变量`

配置段：
- `model` — 模型提供者、默认模型、回退链
- `agent` — 自主级别、心跳间隔、最大轮数
- `context` — Token 窗口、压缩阈值
- `permission` — 权限模式、规则列表
- `channels` — 微信通道（enabled, verbose）
- `cron` — 定时任务（全局开关、模型、任务列表含 notify_weixin）
- `plugins` — MCP 服务器、启用列表
- `logging` — 日志级别、审计、OTEL

配置发现：
- 项目配置：从当前目录向上查找 `.gclaw/config.yaml`
- 用户配置：`~/.gclaw/config.yaml`
- 环境变量：`GCLAW_MODEL`、`GCLAW_AUTONOMY`、`GCLAW_PERMISSION_MODE`、`GCLAW_LOG_LEVEL`

支持 `${ENV_VAR}` 语法在 YAML 中引用环境变量。

### 8. 消息通道 (`internal/channel/`)

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
- **发消息**：`POST /cgi-bin/bot/send_message`，需 `from_user_id: ""` + `client_id`（UUID）两字段
- **ChatUserID 持久化**：最后发消息的用户 ID 写入 `AccountData.ChatUserID`，重启后恢复，供 cron 推送使用
- **独立 Agent**：持有专属 `weixinAgent` 实例，与 REPL 互不阻塞

消息处理流程：`收消息 → agent.Submit() → Agent 循环 → 回复 → OnMessageHandled 回调`

### 9. 定时任务 (`internal/cron/`)

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

Cron Agent 可配置独立模型（`cron.model`），不占用主对话的消息历史。

## 关键设计决策

1. **多 Agent 实例隔离** — REPL、微信、Cron 各用独立 Agent 实例，互不阻塞，消息历史隔离
2. **Cron 模型独立** — 通过 `cron.model` 可为定时任务指定轻量模型（如 `deepseek-v4-flash`），降低 API 成本
3. **Agent.Reset()** — Cron 执行前清空消息历史，避免多轮累积导致的 token 爆炸和上下文污染

4. **OpenAI 兼容适配** — DeepSeek、GLM 等国内模型通过统一 `openai-compatible` 类型接入，降低维护成本
5. **reasoning_content 透传** — DeepSeek V4 的推理内容需原样回传，Message 结构中专门保留了 `ReasoningContent` 字段
6. **双模式 Agent** — 同一 Agent 实例支持 interactive 和 autonomous 两种模式，通过 `Submit()` 的并发锁保护
7. **Mock 优先的开发流程** — 所有提供者工厂默认注册 Mock，确保无 API Key 时也能本地开发测试
8. **Shell 自适应** — Bash 工具自动检测可用 Shell（powershell > sh > bash > cmd），并转换 UTF-16 输出
