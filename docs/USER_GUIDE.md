# GClaw 用户使用手册

## 简介

GClaw 是一个命令行 AI Agent，能自主执行软件工程任务：读写代码、搜索文件、运行命令、管理项目。它支持多种 LLM 后端，可在交互对话和全自主模式之间切换。

## 快速开始

### 安装

```bash
# 从源码编译（需要 Go 1.26+）
git clone https://github.com/openclaw/gclaw.git
cd gclaw
go build -o gclaw.exe ./cmd/gclaw/
```

### 最小配置

在项目根目录创建 `.gclaw/config.yaml`：

```yaml
version: 1

model:
  default: deepseek-v4-pro
  providers:
    deepseek:
      keys:
        - sk-your-deepseek-api-key
      models:
        - deepseek-v4-pro
      base_url: https://api.deepseek.com
```

然后运行：

```bash
./gclaw.exe
```

### 验证安装

```bash
# 查看版本
./gclaw.exe version

# 查看配置（API Key 已脱敏）
./gclaw.exe config dump

# 查看帮助
./gclaw.exe help
```

## 配置详解

### 完整配置示例

```yaml
version: 1

model:
  default: deepseek-v4-pro       # 默认使用的模型名

  fallback:                      # 回退链，默认模型不可用时依次尝试
    - deepseek-v4-pro
    - glm-4.7
    - llama3.2:1b

  providers:
    deepseek:
      keys:
        - sk-your-deepseek-key    # 支持多个 key 轮询
        - sk-another-key
      models:
        - deepseek-v4-pro         # 每个模型名作为独立 provider 注册
        - deepseek-v4-flash
      base_url: https://api.deepseek.com

    zhipu:
      keys:
        - your-zhipu-api-key
      models:
        - glm-4.7
        - glm-4.7-flash
      base_url: https://open.bigmodel.cn/api/paas/v4
      endpoint: /chat/completions  # 自定义端点路径
      thinking: true               # 启用推理模式

    ollama:
      base_url: http://localhost:11434
      models:
        - llama3.2:1b
        - qwen2.5:7b

agent:
  autonomy: interactive           # 自主级别：interactive | semi | full
  tick_interval: 30s              # 自主模式心跳间隔
  idle_sleep: 5m                  # 空闲后休眠时长
  max_turns: 50                   # 单次响应的最大对话轮数

context:
  max_tokens: 128000              # 上下文窗口大小
  compact_at: 0.85                # 触发压缩的 token 使用率
  reserve_ratio: 0.15             # 为响应保留的 token 比例

permission:
  mode: default                   # 权限模式：default | auto | strict | plan
  rules:
    - allow: "Bash(git:*)"        # 允许所有 git 命令
    - deny: "Bash(rm -rf *)"      # 禁止危险删除
    - allow: "ReadFile"           # 总是允许读文件
    # - ask: "WriteFile"          # 写入前询问

channels:
  weixin:
    enabled: true                 # 启用微信通道
    verbose: false                # 详细调试日志

cron:
  enabled: true                   # 启用定时任务调度器
  model: deepseek-v4-flash        # 可选：定时任务专用模型（不设则用默认）
  jobs:
    - name: daily-check           # 任务名（唯一标识）
      schedule: "0 9 * * *"       # 5 字段 cron 表达式
      prompt: "生成今日工作计划"    # 触发时发给 Agent 的指令
      enabled: true               # 可单独启停每个任务
      notify_weixin: true         # 完成后推送到微信

    - name: pr-review
      schedule: "*/30 * * * *"    # 每 30 分钟
      prompt: "检查分支上的 TODO 和 FIXME"
      enabled: true
      notify_weixin: false

logging:
  level: info                     # debug | info | warn | error
```

### 环境变量

| 变量 | 作用 | 示例 |
|------|------|------|
| `GCLAW_MODEL` | 覆盖默认模型 | `export GCLAW_MODEL=glm-4.7` |
| `GCLAW_AUTONOMY` | 覆盖自主级别 | `export GCLAW_AUTONOMY=full` |
| `GCLAW_PERMISSION_MODE` | 覆盖权限模式 | `export GCLAW_PERMISSION_MODE=auto` |
| `GCLAW_LOG_LEVEL` | 覆盖日志级别 | `export GCLAW_LOG_LEVEL=debug` |
| `DEEPSEEK_API_KEY` | DeepSeek API Key | `export DEEPSEEK_API_KEY=sk-xxx` |
| `OPENAI_API_KEY` | OpenAI API Key | `export OPENAI_API_KEY=sk-xxx` |
| `ANTHROPIC_API_KEY` | Claude API Key | `export ANTHROPIC_API_KEY=sk-xxx` |
| `ZHIPU_API_KEY` | 智谱 API Key | `export ZHIPU_API_KEY=xxx` |

YAML 中也可使用 `${VAR}` 语法引用环境变量，例如：

```yaml
providers:
  deepseek:
    keys:
      - ${DEEPSEEK_API_KEY}
```

### 配置优先级

```
环境变量 (最高)
  └─ 项目配置 (.gclaw/config.yaml)
      └─ 用户配置 (~/.gclaw/config.yaml)
          └─ 默认值 (最低)
```

项目配置从当前目录向上查找 `.gclaw/config.yaml`，找到即用。

## 使用方式

### 交互模式（默认）

```bash
./gclaw.exe
```

进入 REPL 后直接输入消息对话：

```
gclaw dev — interactive mode | deepseek-v4-pro | type /help

> 读取 go.mod 文件的内容
> 执行 ls -la 查看当前目录
> 搜索所有包含 "tool" 的 Go 文件
```

#### REPL 命令

| 命令 | 功能 |
|------|------|
| `/help` | 显示帮助 |
| `/stats` | 查看上下文和 token 用量 |
| `/autonomy` | 查看自主调度器状态 |
| `/config` | 显示当前配置 |
| `/weixin` | 微信通道 login|logout|status |
| `/cron` | 查看定时任务状态 |
| `/tasks` | 列出后台任务 |
| `/compact` | 手动压缩上下文 |
| `/clear` | 清空对话历史 |
| `/exit` | 退出 |

### 半自主模式 (semi)

```yaml
agent:
  autonomy: semi
```

Agent 在收到目标后会自主规划和执行，但在关键决策点（破坏性操作、外部 API 调用、架构变更）暂停请求确认。

### 全自主模式 (full)

```yaml
agent:
  autonomy: full
```

Agent 通过心跳驱动完全独立运行：
- 每个 tick 检查是否有待处理的工作
- 空闲一段时候后调用 SleepTool 休眠
- 仅在遇到真正问题或紧急情况时通知用户

在全自主模式下，用户输入的消息会作为事件注入到 Agent 的事件总线中。

## 可用工具

GClaw 通过工具调用能力与环境交互：

### ReadFile
```
参数：file_path (必填), offset (选填), limit (选填)
说明：读取文件内容
```

### WriteFile
```
参数：file_path (必填), content (必填)
说明：创建或覆写文件（需要权限确认）
```

### Bash
```
参数：command (必填), description (选填), timeout (选填)
说明：执行 Shell 命令
安全命令自动放行：git status, git diff, git log, ls, cat, echo, pwd, whoami, which
```

### Glob
```
参数：pattern (必填), path (选填)
说明：按 glob 模式查找文件，如 **/*.go, src/**/*.ts
```

### Grep
```
参数：pattern (必填), path (选填), include (选填)
说明：正则搜索文件内容，可用 include 过滤文件类型
```

## 定时任务

GClaw 内置 cron 调度器，可定期向 Agent 发送指令，执行自动化巡检、定时报告等任务。

### 配置

```yaml
cron:
  enabled: true                   # 全局开关
  jobs:
    - name: daily-check           # 任务名（唯一标识）
      schedule: "0 9 * * *"       # 5 字段 cron 表达式
      prompt: "生成今日工作计划"    # 触发时发给 Agent 的指令
      enabled: true               # 可单独启停
```

在 REPL 中输入 `/cron` 查看所有作业状态：

```
--- Cron Jobs (2) ---
  daily-check:
    schedule: 0 9 * * *
    next_run: 09:00:00
    run_count: 12
  pr-review:
    schedule: */30 * * * *
    next_run: 14:30:00
    run_count: 47
```

### Cron 表达式

表达式由 5 个字段组成，空格分隔：

| 字段 | 范围 | 说明 |
|------|------|------|
| 分钟 | 0-59 | |
| 小时 | 0-23 | |
| 日 | 1-31 | |
| 月 | 1-12 | |
| 周 | 0-6 | 0=周日 |

支持语法：

| 语法 | 示例 | 说明 |
|------|------|------|
| `*` | `* * * * *` | 每分钟 |
| `*/N` | `*/5 * * * *` | 每 5 分钟 |
| 精确值 | `0 9 * * *` | 每天 9:00 |
| 逗号列表 | `0,30 9,17 * * *` | 9:00 和 9:30、17:00 和 17:30 |
| 范围 | `0 9 * * 1-5` | 工作日 9:00 |
| 复合 | `*/15 9-17 * * 1-5` | 工作日 9-17 点，每 15 分钟 |

### 常见场景

```yaml
# 每日站会前检查
- name: morning-scan
  schedule: "55 8 * * 1-5"
  prompt: "检查昨晚是否有新错误日志，汇总最近的代码变更"

# 每 30 分钟检查 PR 状态
- name: pr-watch
  schedule: "*/30 * * * *"
  prompt: "检查当前分支的 TODO 和 FIXME 并报告进度"

# 每小时心跳确认
- name: heartbeat
  schedule: "0 * * * *"
  prompt: "确认系统状态正常，回复 OK"
```

### 注意事项

- 调度器每秒检查一次，到点即触发（忽略秒级精度）
- 如果 Agent 正忙（上一次调用未结束），该次触发会被跳过
- 每次作业执行超时 5 分钟
- `run_count` 是从启动开始的累计执行次数，重启后归零

## 微信通道

GClaw 内置微信通道，扫码登录后可在微信中直接给 Agent 发任务，Agent 回复和 cron 推送也能发回微信。

### 配置

```yaml
channels:
  weixin:
    enabled: true       # 启动时加载微信通道
    verbose: false      # 详细调试日志
```

### 使用

在 REPL 中通过 `/weixin` 命令管理微信连接：

```
> /weixin login     # 触发扫码登录，终端显示二维码
> /weixin status    # 查看连接状态
> /weixin logout    # 断开连接并清除 token
```

#### 登录流程

1. 输入 `/weixin login`
2. 终端渲染二维码
3. 手机微信扫码确认
4. 连接成功，进入长轮询模式

### 消息处理

微信消息到达后，由独立的 `weixinAgent` 实例处理，与 REPL 互不阻塞：

```
微信用户发消息 → 长轮询收到 → agent.Submit() → Agent 处理 → 回复发回微信
```

- `weixinAgent` 有独立的消息历史，MaxTurns 20
- 最后发消息的用户 ID 会持久化保存，重启后自动恢复
- REPL 会话不受微信消息影响

### Cron 推送

定时任务可配置 `notify_weixin: true`，执行完毕后自动将结果推送到微信：

```yaml
cron:
  jobs:
    - name: github-trending
      schedule: "0 9 * * *"
      prompt: "搜索 GitHub 热点..."
      notify_weixin: true   # 完成后推送微信
```

推送流程：Cron 触发 → cronAgent 执行 → 结果通过微信通道发送给最近交互的用户。

### 数据存储

微信登录状态存储在 `~/.gclaw/weixin/`：

```
~/.gclaw/weixin/
├── accounts.json              # 账号列表
└── accounts/
    └── <bot-id>.json          # token、用户 ID 等
```

### 注意事项

- 微信通道需要网络能访问 `ilinkai.weixin.qq.com`
- 长轮询超时 30 秒，收到消息后立即发起下一次轮询
- 如果 Agent 正忙（上一次调用未结束），新消息会排队等待
- 退出 GClaw 时微信通道自动断开

## 模型

### 添加新模型

在配置的 `providers` 中添加条目：

```yaml
model:
  providers:
    moonshot:
      keys:
        - ${MOONSHOT_API_KEY}
      models:
        - moonshot-v1-8k
      base_url: https://api.moonshot.cn
```

`gclaw` 会自动检测以下环境变量中的 API Key：
- `ANTHROPIC_API_KEY` / `CLAUDE_API_KEY` → Claude
- `OPENAI_API_KEY` → OpenAI
- `DEEPSEEK_API_KEY` → DeepSeek
- `ZHIPU_API_KEY` → 智谱 GLM
- `MOONSHOT_API_KEY` → Moonshot Kimi
- `QIANFAN_API_KEY` / `BAIDU_API_KEY` → 百度千帆

设置了环境变量即自动注册对应的 provider。

### 本地模型 (Ollama)

```yaml
providers:
  ollama:
    base_url: http://localhost:11434
    models:
      - llama3.2:1b
```

无需 API Key，指向本地 Ollama 服务地址即可。

## 权限管理

### 规则语法

规则按顺序匹配，先匹配先生效：

```
- allow: "ReadFile"          # 精确匹配工具名
- deny: "Bash(rm -rf *)"     # 拒绝特定命令模式
- allow: "Bash(git:*)"       # 允许某类命令
- allow: "**"                # 允许一切（谨慎使用）
```

### 四种模式

| 模式 | 适用场景 |
|------|----------|
| `default` | 日常使用，按规则列表匹配 |
| `auto` | 信任的自动化环境，全部放行 |
| `strict` | 安全敏感环境，ask 规则需要确认 |
| `plan` | 评审模式，仅允许读操作 |

## 上下文管理

长对话中，令牌使用达到 `compact_at` 阈值（默认 85%）时自动触发压缩：

1. 保留第一条消息（系统上下文锚点）
2. 保留最近的对话轮次
3. 中间消息替换为边界标记

手动压缩：对话中输入 `/compact`

## 日志与调试

```yaml
logging:
  level: debug    # 查看完整 agent 循环日志
```

Debug 模式输出日志示例：
```
2026/04/26 13:12:18 INFO gclaw starting version=dev autonomy=interactive
2026/04/26 13:12:18 DEBUG running agent input="读取 go.mod 文件"
2026/04/26 13:12:18 DEBUG agent turn turn=1 messages=1
2026/04/26 13:12:21 DEBUG agent turn turn=2 messages=3
```

## 常见问题

### Q: 启动时报 "no model providers configured"

检查：
1. `.gclaw/config.yaml` 中是否配置了 `model.providers`
2. 是否设置了对应的环境变量 API Key
3. 运行 `./gclaw.exe config dump` 确认配置加载情况

### Q: 回复是乱码

Windows 终端编码问题，建议使用 Windows Terminal 或配置 UTF-8 编码：
```powershell
[System.Console]::OutputEncoding = [System.Text.Encoding]::UTF8
```

### Q: 工具调用失败

检查日志级别为 `debug` 查看详细错误信息。常见原因：
- API Key 无效或过期
- 网络无法访问 `base_url`
- 模型不支持工具调用（function calling）

### Q: 如何彻底重置

```bash
# 清空项目配置
rm -rf .gclaw

# 重新开始
./gclaw.exe
```

## 文件清单

| 文件 | 说明 |
|------|------|
| `.gclaw/config.yaml` | 项目级配置（可提交到版本控制） |
| `~/.gclaw/config.yaml` | 用户级配置（API Key 等敏感信息） |
| `gclaw.exe` | 编译产物 |
