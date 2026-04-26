# WeChat Channel 设计

## 概述

为 GClaw 接入微信通道，实现双向互通：用户可在微信中直接给 Agent 发任务，Agent 的回复和 cron 推送也能发回微信。

## 架构

```
┌──────────────────────────────────────────────────────────────┐
│                       GClaw Process                          │
│                                                              │
│  ┌──────────┐   ┌──────────────┐   ┌──────────────────────┐  │
│  │  REPL    │   │ Cron 调度器  │   │   自主调度器         │  │
│  │ (ag)     │   │ (cronAgent)  │   │   (ag)               │  │
│  └────┬─────┘   └──────┬───────┘   └──────────┬───────────┘  │
│       │                │                       │              │
│       │        ┌───────┴────────┐              │              │
│       │        │  OnResult      │              │              │
│       │        │  回调          │              │              │
│       │        └───────┬────────┘              │              │
│       │                │                       │              │
│       ▼                ▼                       ▼              │
│  ┌──────────────────────────────────────────────────────┐    │
│  │              三个独立 Agent 实例                      │    │
│  │  · ag (REPL)        — MaxTurns 100, 交互模式         │    │
│  │  · weixinAgent      — MaxTurns 20,  交互模式         │    │
│  │  · cronAgent        — MaxTurns 10,  交互模式         │    │
│  └──────────────────────────────────────────────────────┘    │
│       │                │                                     │
│       │         ┌──────┴──────┐                              │
│       │         │   Weixin    │                              │
│       │         │   Channel   │                              │
│       │         └──────┬──────┘                              │
│       │                │ HTTP long-poll                      │
│       │                ▼                                     │
│       │      ┌──────────────────┐                            │
│       │      │  ilinkai.weixin  │                            │
│       │      │  .qq.com         │                            │
│       │      └────────┬─────────┘                            │
│       │               ▼                                      │
│       │      ┌──────────┐                                    │
│       │      │ 微信用户  │                                    │
│       │      └──────────┘                                    │
└──────────────────────────────────────────────────────────────┘
```

**关键设计决策**：三个场景（REPL、微信、Cron）各用独立的 Agent 实例，互不阻塞。Cron 完成后通过 `OnResult` 回调推送微信（需 `notify_weixin: true` 配置）。

## 新增文件

```
internal/channel/
├── channel.go              # Channel 接口定义
└── weixin/
    ├── weixin.go           # Channel 实现（Start/Stop/Send/Status/Login/Logout）
    ├── client.go           # ilink HTTP 客户端（getUpdates/sendMessage/notifyStart/notifyStop）
    ├── login.go            # 扫码登录流程（getQRCode → poll → confirm）
    ├── types.go            # 协议类型 + AccountData（含 ChatUserID 持久化）
    └── store.go            # token/ChatUserID 持久化读写（JSON 文件存储）

internal/cron/
└── cron.go                 # Cron 调度器 + Job 定义（含 OnResult/NotifyWeixin）
```

## 修改文件

- `cmd/gclaw/main.go` — 三个独立 Agent 实例，微信通道初始化，cron 推送回调，`/weixin` REPL 命令
- `internal/agent/agent.go` — 新增 `Reset()` 方法（清空消息历史）
- `internal/config/config.go` — 新增 `CronConfig`（Model/Jobs）、`ChannelsConfig`（Weixin）、`CronJob.NotifyWeixin`
- `.gclaw/config.yaml` — 新增 `channels`、`cron` 配置段

## Channel 接口

```go
package channel

type Channel interface {
    ID() string
    Start(ctx context.Context, bus *autonomous.EventBus) error
    Stop() error
    Send(ctx context.Context, to, text string) error
    Status() Status
}

type Status struct {
    Connected bool
    AccountID string
    UserID    string
    LastMsgAt time.Time
    MsgCount  int64
}
```

### Weixin Channel 额外能力

- `SetAgent(ag)` — 注入独立 Agent 实例
- `Login() error` — 触发扫码登录
- `Logout() error` — 解绑并清空 token
- `LastUserID() string` — 返回最后发消息的微信用户 ID（用于 cron 推送）
- `IsConnected() bool` — 连接状态
- `HasStoredAccount() bool` — 是否有已保存的账号
- `Config.OnMessageHandled` — 消息处理完成回调（CLI 用于重新打印 `> ` 提示符）

## 配置

```yaml
channels:
  weixin:
    enabled: true          # 启动时加载微信通道
    verbose: false         # 详细调试日志

cron:
  enabled: true
  model: deepseek-v4-flash # cron 任务专用模型（可选，不设则用默认模型）
  jobs:
    - name: github-trending
      schedule: "0 9 * * *"
      prompt: "搜索 GitHub 热点..."
      enabled: true
      notify_weixin: true  # 执行完毕后推送到微信
```

运行时状态存储到 `~/.gclaw/weixin/`：

```
~/.gclaw/weixin/
├── accounts.json              # ["bot-id-1"]
└── accounts/
    └── <bot-id>.json          # { token, baseUrl, userId, chatUserId, savedAt }
```

### ChatUserID 持久化

收到微信消息时，`FromUserID`（如 `o9cq80x...@im.wechat`）写入 `AccountData.ChatUserID`，重启后自动恢复。Cron 推送通过 `LastUserID()` 获取这个持久化的 ID，无需每次启动都先发一条微信消息。

## 协议

后端：`https://ilinkai.weixin.qq.com`

### 扫码登录

1. `GET /ilink/bot/get_bot_qrcode?bot_type=3` → 返回 `qrcode_img_content`
2. 终端渲染二维码，用户手机微信扫码
3. `GET /ilink/bot/get_qrcode_status?qrcode=xxx` 轮询（每秒）
4. 状态变为 `confirmed` → 返回 `bot_token`、`ilink_bot_id`、`ilink_user_id`
5. 支持 IDC 重定向（`scaned_but_redirect`）

### 收消息（长轮询）

```
POST /cgi-bin/bot/get_updates
Header: Authorization: Bearer <bot_token>
Body:   { "get_updates_buf": "" }

Response:
{
  "msgs": [{
    "from_user_id": "o9cq80x...@im.wechat",
    "message_id": 123,
    "message_type": 1,
    "item_list": [
      { "type": 1, "text_item": { "text": "你好" } }
    ],
    "context_token": "xxx"
  }],
  "get_updates_buf": "xxx",
  "longpolling_timeout_ms": 30000
}
```

收到消息后立即发起下一次长轮询。

### 发消息

```
POST /cgi-bin/bot/send_message
Header: Authorization: Bearer <bot_token>
Body: {
  "msg": {
    "from_user_id": "",           // 空字符串（关键：不设会发送失败）
    "to_user_id": "o9cq80x...@im.wechat",
    "client_id": "<random-uuid>", // 必须（不设会导致消息无法送达）
    "message_type": 3,
    "message_state": 1,
    "item_list": [{ "type": 1, "text_item": { "text": "回复" } }],
    "context_token": "xxx"
  },
  "base_info": { "channel_version": "v1.0.0" }
}
```

**关键修复**：`from_user_id: ""` 和 `client_id` 两字段缺一不可，虽然 API 返回 HTTP 200，但缺少任一字段消息将无法送达客户端。这是与 npm 版本协议的差异点。

## 消息路由

```
微信用户发 "你好"
  → long-poll 收到消息 {from, text, contextToken}
  → channel 保存 from 到 lastFromUser + 持久化到 ChatUserID
  → weixinAgent.Submit(ctx, text)
  → Agent 独立处理（不影响 REPL 或 Cron）
  → Channel.sendMessage(to, response, contextToken)
  → weixinCh.LastUserID() 持久化（cron 可用）
  → OnMessageHandled() → CLI 重新打印 "> " 提示符
```

## Cron 推送流程

```
Cron 触发 github-trending
  → cronAgent.Reset()   // 清空历史，避免上下文累积
  → cronAgent.Submit(ctx, prompt)
  → Agent 执行、返回结果
  → Job.OnResult(name, prompt, response)
    → 检查 notify_weixin 配置
    → 检查 weixinCh.IsConnected()
    → weixinCh.LastUserID() 获取目标用户
    → weixinCh.Send(ctx, to, msg)
  → 微信用户收到推送
```

## REPL 命令

| 命令 | 功能 |
|------|------|
| `/weixin login` | 触发扫码登录（已有 token 也可重新绑定） |
| `/weixin logout` | 清除 token，断开连接 |
| `/weixin status` | 显示连接状态、账号、用户 ID、最后收消息时间、消息数 |
| `/cron` | 查看定时任务状态（含 notify_weixin 字段） |

## Agent 实例隔离

| Agent | MaxTurns | 用途 | 模型 |
|-------|----------|------|------|
| `ag` | 100 | REPL 交互 | 默认模型 |
| `weixinAgent` | 20 | 微信消息处理 | 默认模型 |
| `cronAgent` | 10 | 定时任务 | `cron.model` 指定，未设则用默认 |

每个 Agent 有独立的消息历史，`cronAgent` 每次执行前调用 `Reset()` 清空历史，避免多次执行的上下文累积导致 token 爆炸。

## 启动流程

```
./gclaw.exe
  → 加载配置
  → 初始化三个 Agent 实例
  → 如果 channels.weixin.enabled:
    → 初始化 WeixinChannel
    → 注入 weixinAgent
    → 恢复 lastFromUser（从 AccountData.ChatUserID）
    → channel.Start():
      ├── 有保存的 token → notify_start → 启动长轮询（后台）
      └── 无 token → 等待 `/weixin login` 命令
  → 如果 cron.enabled:
    → 初始化 CronScheduler
    → 注入 cronAgent（可选 cron.model 指定模型）
    → 注册 Job，绑定 OnResult 回调（含微信推送）
    → 启动调度器
  → 进入 REPL（Channel 和 Cron 在后台运行）
```

## 技术选型

| 决策 | 选择 | 原因 |
|------|------|------|
| 协议实现 | Go 原生 HTTP | 协议简单，无需 Node.js |
| Agent 模型 | 多实例隔离 | 三种场景互不阻塞，历史独立 |
| 存储 | JSON 文件 | 与项目现有风格一致 |
| 并发模型 | goroutine + context | 长轮询天然适合 goroutine |
| 消息送达修复 | from_user_id="" + client_id | 对比 npm 包协议后发现的缺失字段 |
| ChatUserID 持久化 | AccountData JSON | 重启后无需重新发消息即可做 cron 推送 |
