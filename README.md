# gclaw

高性能自主 AI Agent CLI 工具。支持多模型后端，可在交互对话和全自主模式之间切换，内置文件读写、命令执行、代码搜索等工具。

## 快速开始

```bash
# 编译（需要 Go 1.26+）
go build -o gclaw.exe ./cmd/gclaw/

# 配置 API Key
mkdir .gclaw
cat > .gclaw/config.yaml << 'EOF'
version: 1
model:
  default: deepseek-v4-pro
  providers:
    deepseek:
      keys: [sk-your-api-key]
      models: [deepseek-v4-pro]
      base_url: https://api.deepseek.com
EOF

# 启动
./gclaw.exe
```

```text
gclaw dev — interactive mode | deepseek-v4-pro | type /help

> 读取 go.mod 文件的内容
> 执行 ls 命令查看当前目录
> 搜索所有包含 "Tool" 的 Go 文件
```

## 特性

- **多模型支持** — DeepSeek、GLM（智谱）、Claude、OpenAI、Ollama，自动回退
- **三级自主模式** — interactive（交互）/ semi（半自主）/ full（全自主心跳驱动）
- **微信通道** — 扫码登录，双向消息，cron 结果推送
- **定时任务** — 内置 cron 调度，支持独立模型，微信通知
- **内置工具** — ReadFile、WriteFile、Bash、Glob、Grep、SleepTool
- **权限系统** — 洋葱模型，glob 规则匹配，四种执行模式
- **上下文管理** — Token 估算，自动压缩，可配置阈值
- **任务管理** — 异步后台任务，并发控制，DAG 依赖

## 模型提供者

| 提供者 | 类型 | 需要 API Key |
|--------|------|:---:|
| DeepSeek | `openai-compatible` | 是 |
| GLM (智谱) | `openai-compatible` | 是 |
| Claude (Anthropic) | `claude` | 是 |
| OpenAI | `openai` | 是 |
| Moonshot (Kimi) | `openai-compatible` | 是 |
| 百度千帆 | `openai-compatible` | 是 |
| Ollama (本地) | `ollama` | 否 |

设置 `DEEPSEEK_API_KEY`、`ZHIPU_API_KEY`、`ANTHROPIC_API_KEY`、`OPENAI_API_KEY` 等环境变量即可自动注册对应提供者。

## 配置

```yaml
version: 1

model:
  default: deepseek-v4-pro
  fallback: [deepseek-v4-pro, glm-4.7, llama3.2:1b]
  providers:
    deepseek:
      keys: [sk-xxx]
      models: [deepseek-v4-pro, deepseek-v4-flash]
      base_url: https://api.deepseek.com
    ollama:
      base_url: http://localhost:11434
      models: [llama3.2:1b]

agent:
  autonomy: interactive   # interactive | semi | full
  tick_interval: 30s
  idle_sleep: 5m
  max_turns: 50

context:
  max_tokens: 128000
  compact_at: 0.85
  reserve_ratio: 0.15

permission:
  mode: default           # default | auto | strict | plan
  rules:
    - allow: "Bash(git:*)"
    - deny: "Bash(rm -rf *)"

channels:
  weixin:
    enabled: false        # 微信通道开关
    verbose: false        # 详细日志

cron:
  enabled: false          # 定时任务开关
  model: deepseek-v4-flash # 可选：定时任务专用模型
  jobs:
    - name: my-job
      schedule: "0 9 * * *"
      prompt: "生成今日摘要"
      enabled: true
      notify_weixin: true # 结果推送到微信

logging:
  level: info             # debug | info | warn | error
```

配置加载优先级：**环境变量** → **项目配置** (`.gclaw/config.yaml`) → **用户配置** (`~/.gclaw/config.yaml`) → **默认值**

## REPL 命令

| 命令 | 功能 |
|------|------|
| `/help` | 帮助信息 |
| `/stats` | 上下文和 token 用量 |
| `/autonomy` | 自主调度器状态 |
| `/cron` | 定时任务状态 |
| `/weixin` | 微信通道 login|logout|status |
| `/tasks` | 后台任务列表 |
| `/config` | 当前配置 |
| `/compact` | 手动压缩上下文 |
| `/clear` | 清空对话 |
| `/exit` | 退出 |

## 环境变量

| 变量 | 作用 |
|------|------|
| `GCLAW_MODEL` | 覆盖默认模型 |
| `GCLAW_AUTONOMY` | 覆盖自主级别 |
| `GCLAW_PERMISSION_MODE` | 覆盖权限模式 |
| `GCLAW_LOG_LEVEL` | 覆盖日志级别 |
| `DEEPSEEK_API_KEY` | DeepSeek API Key |
| `ZHIPU_API_KEY` | 智谱 API Key |
| `ANTHROPIC_API_KEY` | Claude API Key |
| `OPENAI_API_KEY` | OpenAI API Key |

## 项目结构

```
gclaw/
├── cmd/gclaw/           # 主入口 (REPL + 微信 + Cron)
├── internal/
│   ├── agent/           # Agent 核心循环
│   ├── autonomous/      # 自主模式调度器
│   ├── channel/         # 消息通道接口
│   │   └── weixin/      # 微信通道实现
│   ├── config/          # 配置加载与验证
│   ├── context/         # 上下文窗口管理
│   ├── cron/            # 定时任务调度器
│   ├── model/           # LLM 接口与实现
│   │   ├── claude/
│   │   ├── openai/
│   │   └── ollama/
│   ├── perm/            # 权限检查
│   ├── provider/        # 模型工厂
│   ├── task/            # 后台任务管理
│   └── tool/builtin/    # 内置工具
├── docs/                # 文档
└── plugins/             # 插件系统
```

## 文档

- [架构文档](docs/ARCHITECTURE.md)
- [用户使用手册](docs/USER_GUIDE.md)

## License

MIT
