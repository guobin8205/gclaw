---
name: github-trending
description: 追踪 GitHub 热门项目，支持新星项目和活跃热门两种策略，按领域分类总结
---

## 指令

当用户要求搜索 GitHub 热门/趋势项目时，执行以下操作：

**硬性规则**: 必须使用 `gh api` 命令，禁止使用 Invoke-WebRequest、curl 或其他 HTTP 工具。Bash 工具是非交互式的，命令不能有任何交互式提示。只执行一条命令，执行完后直接输出中文总结。

### 搜索策略

根据用户意图选择（默认策略A）：

**策略A — 新星项目**: 最近 N 天新创建、按 stars 排序（发现新兴项目）
**策略B — 活跃热门**: 最近 N 天有更新、stars>100、按 stars 排序（发现持续热门项目）

用户未指定天数时默认 N=7。

### 命令（根据 Bash 工具描述中的 Shell 类型选择对应模板，原样复制执行）

**PowerShell — 策略A**（默认）:
```
$since=(Get-Date).AddDays(-7).ToString('yyyy-MM-dd'); gh api "search/repositories?q=created:>=$since&sort=stars&order=desc&per_page=10" --jq '.items[]|[.full_name,.stargazers_count,.topics]'
```

**PowerShell — 策略B**:
```
$since=(Get-Date).AddDays(-7).ToString('yyyy-MM-dd'); gh api "search/repositories?q=pushed:>=$since+stars:>100&sort=stars&order=desc&per_page=10" --jq '.items[]|[.full_name,.stargazers_count,.topics]'
```

**Bash — 策略A**（默认）:
```
SINCE=$(date -d '7 days ago' +%Y-%m-%d); gh api "search/repositories?q=created:>=$SINCE&sort=stars&order=desc&per_page=10" --jq '.items[]|[.full_name,.stargazers_count,.topics]'
```

**Bash — 策略B**:
```
SINCE=$(date -d '7 days ago' +%Y-%m-%d); gh api "search/repositories?q=pushed:>=$SINCE+stars:>100&sort=stars&order=desc&per_page=10" --jq '.items[]|[.full_name,.stargazers_count,.topics]'
```

如用户指定天数，替换命令中的 `7`（或 `7 days ago`）为对应数字。

**注意**: 输出为 JSON 数组，每行一个项目：`["owner/repo", 1234, ["topic1","topic2"]]`

### 输出格式

根据输出中 topics 字段自动归类：

```
## GitHub 热点速递（近N天 · 策略说明）

### AI / 大模型
- **owner/repo** (⭐ stars)

### 游戏开发
- **owner/repo** (⭐ stars)

### 开发者工具
- **owner/repo** (⭐ stars)

### 开源框架 / 基础设施
- **owner/repo** (⭐ stars)

### 其他热门
- **owner/repo** (⭐ stars)
```

### 分类规则

根据 topics 字段匹配（忽略大小写）：
- 含 `llm,ai,ml,deep-learning,chatgpt,gpt,transformer,pytorch,nlp` → AI / 大模型
- 含 `game,game-development,godot,unity,unreal,game-engine` → 游戏开发
- 含 `cli,tool,devtools,developer-tools,vscode,ide,editor` → 开发者工具
- 含 `framework,library,infrastructure,api,rust-lang,go-lang,web` → 开源框架 / 基础设施
- 无法匹配 → 其他热门

没有匹配某个分类时显示"暂无"。不要重复调用工具，执行完命令后直接输出总结。
