# 内置工具集设计

参考 hermes-agent 的工具系统，为 gclaw 实现 12 类核心内置工具。

## 架构：分层工具集

在 `internal/tool/builtin/` 下按工具集分组，引入轻量级后端抽象层封装具体实现。

### 目录结构

```
internal/
  tool/
    builtin/
      file/           # read_file, write_file, patch
      search/         # glob, grep
      shell/          # bash
      time/           # sleep
      web/            # web_search, web_extract
      browser/        # browser_navigate, browser_snapshot, browser_click, ...
      vision/         # vision_analyze
      image/          # image_generate
      todo/           # todo
      memory/         # memory
      clarify/        # clarify
      codeexec/       # execute_code
      session/        # session_search
      tts/            # text_to_speech
      mcp/            # mcp_discover, mcp_call, mcp_list_servers
      meta/           # delegate_task, cron_*, weixin_status, tasks_list
      skill/          # skill_list, skill_create, skill_delete
  websearch/          # Web 搜索后端抽象
    backend.go
    tavily.go
    firecrawl.go
    exa.go
    google.go
  browser/            # 浏览器后端
    browser.go
    chromedp.go
  vision/             # 视觉分析后端
    backend.go
  imagegen/           # 图片生成后端
    backend.go
  tts/                # TTS 后端
    backend.go
```

### 后端抽象模式

所有多后端工具遵循统一模式：

```go
type Backend interface {
    Name() string
    Check() bool  // 检查环境变量/依赖是否可用
}

type Factory struct {
    backends []Backend
    default  string
}
```

配置通过 `config.yaml` 选择后端，或自动检测（按环境变量可用性）。

---

## 分批实现计划

### 第一批：核心开发工具

#### 1. Patch — 定向文件编辑

**Toolset:** `file`
**ConcurrencySafe:** false
**RequiresApproval:** true

参数：
```json
{
  "file_path": "string (required) — 文件绝对路径",
  "old_string": "string (required) — 要替换的原文本",
  "new_string": "string (required) — 替换后的文本",
  "replace_all": "boolean (optional) — 是否替换所有匹配，默认 false"
}
```

行为：
- 精确匹配 `old_string`
- 0 个匹配：报错 "not found"
- >1 个匹配且非 replace_all：报错 "multiple matches"
- 写入前检查文件 mtime 防止并发冲突
- 返回 unified diff 格式的变更预览
- 自动检测二进制文件并拒绝

#### 2. Todo — 任务追踪

**Toolset:** `todo`
**ConcurrencySafe:** true
**RequiresApproval:** false

参数：
```json
{
  "action": "string (required) — list | add | update | remove",
  "id": "string (optional) — 任务 ID（update/remove 时必填）",
  "subject": "string (optional) — 任务标题（add 时必填）",
  "description": "string (optional) — 任务描述",
  "status": "string (optional) — pending | in_progress | completed | cancelled"
}
```

行为：
- 纯内存，不持久化
- 状态：pending → in_progress → completed / cancelled
- 每次调用返回完整任务列表
- 自动生成递增 ID

#### 3. Memory — 跨会话持久记忆

**Toolset:** `memory`
**ConcurrencySafe:** true（文件锁保护）
**RequiresApproval:** false

参数：
```json
{
  "action": "string (required) — read | add | replace | remove",
  "key": "string (optional) — 记忆键名（add/replace/remove 时必填）",
  "content": "string (optional) — 记忆内容（add/replace 时必填）"
}
```

行为：
- 存储在 `~/.gclaw/memory/` 目录，每条记忆一个 `.md` 文件
- 会话开始时将记忆索引注入 system prompt
- 写入时不更新当前 system prompt（保护 prefix cache）
- 注入检测：过滤隐形字符和威胁模式（如 "ignore previous instructions"）
- `read`: 返回所有记忆的索引（文件名 + 第一行摘要）
- `add`: 创建新记忆文件
- `replace`: 通过 key 匹配替换内容
- `remove`: 通过 key 匹配删除文件

#### 4. Clarify — 向用户提问

**Toolset:** `clarify`
**ConcurrencySafe:** true
**RequiresApproval:** false

参数：
```json
{
  "question": "string (required) — 要问用户的问题",
  "options": "array (optional) — 选项列表，每个元素 {label, description}"
}
```

行为：
- Agent 调用时，系统通过 interrupt 机制向用户展示问题
- 选项式：用户从选项中选择
- 开放式：用户自由输入
- 回答通过 interrupt channel 注入 Agent 消息流
- 不影响 Agent 的 turn 计数

---

### 第二批：信息获取工具

#### 5. Web Search — 多后端 Web 搜索

**Toolset:** `web`
**ConcurrencySafe:** true
**RequiresApproval:** false

后端抽象层 (`internal/websearch/`)：

```go
type Backend interface {
    Name() string
    Check() bool
    Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error)
}

type SearchOptions struct {
    MaxResults int
    TimeRange  string // "day", "week", "month", "year"
    Region     string
}

type SearchResult struct {
    Title       string
    URL         string
    Description string
    RawContent  string
}
```

支持的后端：
- **Tavily** — `TAVILY_API_KEY`
- **Firecrawl** — `FIRECRAWL_API_KEY` + `FIRECRAWL_API_URL`
- **Exa** — `EXA_API_KEY`
- **Google Custom Search** — `GOOGLE_API_KEY` + `GOOGLE_CX`

配置：`websearch.backend` 或自动检测。

工具参数：
```json
{
  "query": "string (required) — 搜索查询",
  "max_results": "integer (optional, default 10)",
  "time_range": "string (optional) — day|week|month|year",
  "backend": "string (optional) — 指定后端"
}
```

#### 6. Web Extract — URL 内容提取

**Toolset:** `web`
**ConcurrencySafe:** true
**RequiresApproval:** false

参数：
```json
{
  "url": "string (required) — 要提取的 URL",
  "format": "string (optional) — markdown (默认) | text",
  "summarize": "boolean (optional, default true) — 是否对长内容 LLM 摘要"
}
```

行为：
- HTTP GET 获取页面
- HTML → Markdown 转换
- 大页面（>500K 字符）：分块 + LLM 摘要
- 超大页面（>2M 字符）：拒绝
- SSRF 防护：禁止内网 IP（127.0.0.1, 10.x, 172.16-31.x, 192.168.x, 0.0.0.0）
- 支持 PDF URL
- 缓存：相同 URL 30 分钟内复用

#### 7. Session Search — 搜索历史会话

**Toolset:** `session`
**ConcurrencySafe:** true
**RequiresApproval:** false

参数：
```json
{
  "query": "string (required) — 搜索查询",
  "limit": "integer (optional, default 5)",
  "time_range": "string (optional) — 时间范围过滤"
}
```

行为：
- 会话记录存储在 `~/.gclaw/sessions/`
- 每次会话保存为 JSON（messages 数组 + 元数据）
- 搜索：全文本匹配 + 时间范围过滤
- 返回匹配会话的摘要（时间、消息数、关键内容片段）

---

### 第三批：多模态工具

#### 8. Vision Analyze — 图片分析

**Toolset:** `vision`
**ConcurrencySafe:** true
**RequiresApproval:** false

后端抽象层 (`internal/vision/`)：

```go
type Backend interface {
    Name() string
    Check() bool
    Analyze(ctx context.Context, image ImageInput, prompt string) (string, error)
}

type ImageInput struct {
    URL       string
    FilePath  string
    Base64    string
    MediaType string
}
```

支持的后端：
- 复用现有 model 层（Claude/GPT-4V 等多模态模型）
- OpenRouter 路由
- 默认使用当前 Agent 的模型

参数：
```json
{
  "image": "string (required) — 图片 URL 或本地文件路径",
  "prompt": "string (required) — 分析指令"
}
```

#### 9. Image Generation — 图片生成

**Toolset:** `image`
**ConcurrencySafe:** true
**RequiresApproval:** false

后端抽象层 (`internal/imagegen/`)：

```go
type Backend interface {
    Name() string
    Check() bool
    Generate(ctx context.Context, prompt string, opts GenOptions) (*GenResult, error)
}

type GenOptions struct {
    Model string
    Size  string // "landscape", "square", "portrait"
}

type GenResult struct {
    URL       string
    LocalPath string
}
```

支持的后端：
- **FAL.ai** — `FAL_API_KEY`
- **OpenAI DALL-E** — `OPENAI_API_KEY`
- **Stability AI** — `STABILITY_API_KEY`

参数：
```json
{
  "prompt": "string (required) — 图片描述",
  "model": "string (optional) — 模型名称",
  "size": "string (optional) — landscape|square|portrait"
}
```

#### 10. TTS — 文字转语音

**Toolset:** `tts`
**ConcurrencySafe:** true
**RequiresApproval:** false

后端抽象层 (`internal/tts/`)：

```go
type Backend interface {
    Name() string
    Check() bool
    Speak(ctx context.Context, text string, opts TTSOptions) (*TTSResult, error)
}

type TTSOptions struct {
    Voice  string
    Speed  float64
    Format string // "mp3", "wav"
}

type TTSResult struct {
    FilePath string
}
```

支持的后端：
- **Edge TTS** — 免费，无需 API Key
- **OpenAI TTS** — `OPENAI_API_KEY`
- **ElevenLabs** — `ELEVENLABS_API_KEY`

参数：
```json
{
  "text": "string (required) — 要转换的文本",
  "voice": "string (optional) — 音色名称",
  "backend": "string (optional) — 指定后端"
}
```

---

### 第四批：高级工具

#### 11. Browser Automation — 浏览器自动化

**Toolset:** `browser`
**ConcurrencySafe:** false
**RequiresApproval:** true

后端：chromedp（Go 的 Chrome DevTools Protocol 库）。

抽象层 (`internal/browser/`)：

```go
type Browser interface {
    Navigate(ctx context.Context, url string) error
    Snapshot(ctx context.Context) (string, error)
    Click(ctx context.Context, ref string) error
    Type(ctx context.Context, ref string, text string) error
    Scroll(ctx context.Context, direction string, amount int) error
    Press(ctx context.Context, key string) error
    Screenshot(ctx context.Context) ([]byte, error)
    Close() error
}
```

工具集（7 个原子操作工具）：

| 工具 | 参数 | 说明 |
|------|------|------|
| `browser_navigate` | `url` | 打开 URL |
| `browser_snapshot` | 无 | 获取 accessibility tree 文本快照 |
| `browser_click` | `ref` | 点击元素（@e1, @e2 选择器） |
| `browser_type` | `ref`, `text` | 在元素中输入文本 |
| `browser_scroll` | `direction`, `amount` | 滚动页面 |
| `browser_press` | `key` | 按键操作 |
| `browser_screenshot` | 无 | 截图 |

关键设计：
- headless Chromium
- 使用 accessibility tree 文本表示（节省 token）
- 元素通过 `@ref` 选择器标识
- 每个 Agent 实例共享一个 Browser 实例

#### 12. Code Execution — 代码执行

**Toolset:** `codeexec`
**ConcurrencySafe:** false
**RequiresApproval:** true

参数：
```json
{
  "code": "string (required) — 要执行的代码",
  "language": "string (required) — go | python | javascript | shell",
  "timeout": "integer (optional, default 30000) — 超时毫秒数"
}
```

行为：
- 创建临时文件，写入代码
- 调用对应运行时执行（`go run`, `python`, `node`, `sh`）
- 捕获 stdout + stderr
- 返回输出 + 退出码
- 工作目录隔离（临时目录）

#### 13. MCP — Model Context Protocol 集成

**Toolset:** `mcp`
**ConcurrencySafe:** false
**RequiresApproval:** true

后端 (`internal/mcp/`)：

```go
type Client struct {
    config MCPServerConfig
    tools  []MCPTool
}

type MCPServerConfig struct {
    Command string            // stdio 模式
    URL     string            // HTTP 模式
    Env     map[string]string
}

type MCPTool struct {
    Name        string
    Description string
    InputSchema map[string]any
}
```

支持两种传输：Stdio、HTTP/StreamableHTTP。

工具拆分为 3 个操作：
- `mcp_list_servers` — 列出已配置的服务器
- `mcp_discover` — 发现服务器的所有工具
- `mcp_call` — 调用特定工具

行为：
- 启动时根据配置连接 MCP 服务器
- 动态注册发现的工具到 Registry（带 `mcp_` 前缀）
- 支持 reconnection 和超时
- 环境变量过滤（安全性）

---

## 配置扩展

`config.yaml` 新增配置段：

```yaml
websearch:
  backend: tavily          # tavily | firecrawl | exa | google | auto
  max_results: 10

vision:
  backend: default         # default (当前模型) | openrouter | custom

imagegen:
  backend: fal             # fal | openai | stability
  model: flux-2-klein
  default_size: square

tts:
  backend: edge            # edge | openai | elevenlabs
  voice: ""

browser:
  headless: true
  timeout: 30s

mcp:
  servers:
    - name: example
      command: "npx"
      args: ["-y", "@example/mcp-server"]
      env:
        API_KEY: "${EXAMPLE_API_KEY}"
```

## 安全考虑

- **SSRF 防护：** Web Extract 禁止内网 IP
- **注入检测：** Memory 工具过滤威胁模式
- **文件安全：** Patch 检测 mtime 冲突，拒绝二进制文件
- **命令隔离：** Code Execution 在临时目录中运行
- **环境变量过滤：** MCP 不暴露敏感环境变量
- **Approval 机制：** 危险操作（Patch、Browser、Code Exec、MCP）需要用户确认

## 现有工具迁移

第一批实现时需要将现有工具从扁平结构迁移到分组目录：
- `file_read/` → `file/read.go`
- `file_write/` → `file/write.go`
- `search/` → `search/` (已分组)
- `shell/` → `shell/` (已分组)
- `skill_tools/` → `skill/`
- `meta/` → `meta/` (已分组)

迁移原则：仅移动文件位置，不改变功能逻辑。`init()` 注册模式不变，`main.go` 的 blank import 路径更新即可。
