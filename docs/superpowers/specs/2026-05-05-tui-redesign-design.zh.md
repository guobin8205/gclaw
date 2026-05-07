# gclaw TUI 重设计 — 设计规范

日期：2026-05-05

## 概述

将当前的 `bufio.Scanner` REPL 替换为完整的 Bubble Tea TUI，添加交互式输入（补全、历史记录、多行编辑）、工具调用可视化、流式输出、图像/文件支持和日志环形缓冲区。

框架：**Bubble Tea + Lip Gloss + Bubbles**（Charm 技术栈）。

## 布局

```
┌─ 对话记录区（自适应，虚拟滚动）──────────────────────────┐
│  横幅（版本 | 模型 | 模式）                              │
│  消息列表（用户 / 助手 / 工具 / 事件）                   │
│  滚动条（1列，右侧边缘）                                 │
├─ 分隔线 ────────────────────────────────────────────────┤
│  状态栏（左对齐）                                        │
│  队列预览（忙碌时显示）                                  │
│  补全下拉框（浮动，/ 触发）                              │
│  多行输入缓冲区                                          │
│  附件标签（图像 / 文件）                                 │
│  帮助提示                                                │
└─────────────────────────────────────────────────────────┘
```

## 组件

### 对话记录区（Transcript）

虚拟滚动：仅渲染可见行。自动滚动到底部；用户向上滚动时暂停，新用户消息时恢复。

消息类型：

| 类型 | 前缀 | 样式 |
|------|------|------|
| 用户 | `❯` | 绿色加粗 |
| 助手 | 缩进 | 正常文本，Markdown 渲染 |
| 工具调用 | `⚙` / `$` / `⚡` / `★` | 树形结构（见工具调用树） |
| 事件 | 虚线边框 | 系统通知（微信、定时任务、检查点） |

### 工具调用树

嵌套缩进，使用 `┊` 竖线。每一级增加 16px 缩进 + 1px 左边框。

```
⚡ delegate_task 配置微信公众号接入
┊ ★ skill: weixin_setup
┊ ┊ ⚙ grep weixin.*setup 0.4s
┊ ┊ $ grep -n "weixin" config.go 0.2s
┊ ┊ ⚙ read client.go 0.8s
┊ ⚙ write config.yaml ... ⠋
```

图标约定：

- `⚙` — 文件/搜索工具（read、grep、glob、web_search、web_extract）
- `$` — Bash 命令
- `⚡` — delegate_task（子代理）
- `★` — 技能调用
- `🖼` — 图像/视觉
- `✓` — 成功
- `✕` — 失败
- `⠋` — 进行中（旋转动画）

工具输出默认折叠。Ctrl+O 或点击展开。折叠时显示行数：`▸ 输出 (47行) · Ctrl+O 展开`。

### 编辑器（Composer）

多行输入缓冲区：

- **Enter** — 发送消息
- **Ctrl+Enter** — 插入新行
- **Backspace** 在行首 — 与上一行合并
- **Delete** — 删除光标后字符
- **← → Home End** — 跨行光标移动
- **Ctrl+V** — 粘贴文本；如果剪贴板包含图像，保存到 `~/.gclaw/tmp/clipboard-{timestamp}.png` 并附加
- **Ctrl+I** — 打开文件选择器附加文件
- **Tab** — 应用当前补全
- **↑ ↓** — 循环历史记录（无补全时）或循环补全项（补全激活时）
- **Ctrl+C** — 中断代理（空输入）或清除输入（有输入）
- **Esc** — 取消输入 / 关闭补全下拉框
- **Ctrl+L** — 清屏

### 补全

以 `/` 前缀触发。防抖 60ms。在输入行上方显示下拉框：

```
/help — 显示可用命令        ← 高亮（激活）
/logs — 查看最近日志 [N]
/model — 切换模型
/status /tools /skills ...
```

Tab 应用激活项。↑↓ 循环。Esc 关闭。最多显示 16 项。

### 状态栏

左对齐，单行：

```
● deepseek-v4-flash │ ctx: 1.2k/200k │ agents: 2 │ bg: 1 │ cron: active │ 5:36:20
```

字段：

- 模型名称带彩色圆点（绿色=就绪，黄色=忙碌，红色=错误）
- 上下文使用量（输入/总令牌数）
- 活跃子代理数量（为 0 时隐藏）
- 后台任务数量（为 0 时隐藏）
- 定时任务状态
- 会话运行时间

### 队列预览

当代理忙碌时，在输入上方显示排队消息：

```
⏳ 2 条排队  /model gpt-4o · /status
```

上下箭头编辑排队消息。Esc 取消编辑。

### 图像与文件附件

附件流程：

1. **Ctrl+V 图像剪贴板** → 保存到 `~/.gclaw/tmp/clipboard-{ts}.png` → 显示标签 `🖼 clipboard-image.png ✕`
2. **Ctrl+I 或拖拽文件** → `.png/.jpg/.jpeg` 作为图像附加（发送给视觉工具），`.txt/.log/.md` 将文本注入提示词
3. 标签显示在输入行下方，带 ✕ 可移除
4. 发送时，图像自动传递给视觉工具

在对话记录中，图像附件渲染为卡片：

```
🖼 clipboard-image.png
[图像预览: 800×600]
```

## 流式输出与思考

- 助手回复以流式方式输入对话记录
- DeepSeek ReasoningContent 渲染为折叠块：

```
▸ 💭 思考过程 (2.1s · Ctrl+O 展开)
```

- **Ctrl+O** 切换当前思考块的折叠/展开
- 思考过程默认折叠；回复正文始终可见

## 工具审批

需要审批的工具在编辑器区域显示弹窗：

```
⚠ 需要审批
$ rm -rf /tmp/test-output
[Y 允许] [N 拒绝] [A 总是允许] [Esc 取消]
```

按键：Y（允许一次）、N（拒绝）、A（总是允许此模式）、Esc（取消）。左对齐，在解决前替换输入区域。

## Markdown 渲染

助手消息渲染 Markdown：
- 代码块：语言标签 + 语法高亮（Go、Python、JSON、YAML 等）
- 表格：对齐列
- 粗体/斜体/链接
- 通过 Lip Gloss 样式文本实现

## 异步事件通知

系统事件在对话记录中以虚线边框渲染：

```
--- 📬 weixin  张三: 今天的报告写完了吗？ ---
--- ⏰ cron  test-noagent 已完成 ✓ ---
--- 💾 checkpoint  WriteFile 前已保存快照 ---
```

子代理数量和后台任务数量显示在状态栏：`agents: 2 │ bg: 1`（为 0 时隐藏）。

## 历史记录

- 持久化文件：`~/.gclaw/history`
- 格式：每行一条，时间戳 + 内容
- 最大条目数：可配置，默认 10000
- ↑↓ 导航历史记录；前缀匹配过滤
- 循环历史记录时保留草稿

## 日志系统

- 所有 `slog` 输出重定向到内存环形缓冲区（默认 200 行）
- `/logs [N]` — 在对话记录中显示最近 N 行
- `/logs -f` — 跟随模式（流式新日志），Ctrl+C 退出
- 按级别着色：INFO=蓝色、WARN=黄色、ERROR=红色、DEBUG=灰色

## 主题

四套内置主题，通过 `tui.theme` 或 `/theme` 命令配置：

| 主题 | 背景 | 使用场景 |
|------|------|----------|
| `tokyo-night`（默认） | `#1a1b26` | 深色终端 |
| `catppuccin-mocha` | `#1e1e2e` | 暖色深色终端 |
| `light` | `#fafafa` | 浅色终端 |
| `terminal` | ANSI 默认值 | 跟随终端调色板 |

每主题颜色：

**Tokyo Night：**
- 文本：`#c0caf5`
- 强调：`#7aa2f7`
- 绿色：`#9ece6a`
- 橙色：`#ff9e64`
- 紫色：`#bb9af7`
- 黄色：`#e0af68`
- 红色：`#f7768e`
- 柔和：`#565f89`
- 边框：`#3b3d57`

**Catppuccin Mocha：**
- 文本：`#cdd6f4`
- 强调：`#89b4fa`
- 绿色：`#a6e3a1`
- 橙色：`#fab387`
- 紫色：`#cba6f7`
- 黄色：`#f9e2af`
- 红色：`#f38ba8`
- 柔和：`#6c7086`
- 边框：`#313244`

**Light：**
- 文本：`#333333`
- 强调：`#0066cc`
- 绿色：`#008800`
- 橙色：`#cc6600`
- 红色：`#cc0000`
- 柔和：`#888888`

**Terminal：**
- 所有颜色使用标准 ANSI 代码（0-15）

## 虚拟滚动

- 维护所有消息 ID 及其渲染高度的切片
- 滚动时，计算可见窗口并仅渲染这些行
- 顶部/底部间隔填充剩余空间
- 滚动条滑块反映视口位置
- 终端大小变化时重新渲染

## 配置

`.gclaw/config.yaml` 中的新配置字段：

```yaml
tui:
  theme: tokyo-night        # tokyo-night | light | terminal
  history:
    max_entries: 10000
  log:
    buffer_size: 200
  completion:
    debounce_ms: 60
    max_visible: 16
```

## 文件结构

新包：

```
internal/tui/
├── app.go              # Bubble Tea Model + 主 tea.Model 接口
├── transcript.go       # 虚拟滚动对话记录视图
├── composer.go         # 带补全的多行输入
├── statusbar.go        # 左对齐状态栏
├── completion.go       # 斜杠命令补全引擎
├── history.go          # 持久化历史记录管理器
├── logbuffer.go        # slog 捕获环形缓冲区
├── theme.go            # 主题定义和样式
├── markdown.go         # Markdown 转 Lip Gloss 样式文本
├── approval.go         # 工具审批弹窗
├── message.go          # 消息类型和渲染
└── keymap.go           # 按键绑定
```

集成点：`cmd/gclaw/main.go` 将 `bufio.Scanner` 循环替换为 `tea.NewProgram(app).Run()`。
