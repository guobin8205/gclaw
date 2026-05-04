package main

import (
	"bufio"
	stdctx "context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openclaw/gclaw/internal/agent"
	"github.com/openclaw/gclaw/internal/autonomous"
	"github.com/openclaw/gclaw/internal/channel/weixin"
	"github.com/openclaw/gclaw/internal/config"
	"github.com/openclaw/gclaw/internal/context"
	"github.com/openclaw/gclaw/internal/cron"
	"github.com/openclaw/gclaw/internal/delegate"
	"github.com/openclaw/gclaw/internal/gateway"
	gw_adapter "github.com/openclaw/gclaw/internal/gateway/adapter"
	"github.com/openclaw/gclaw/internal/memory"
	"github.com/openclaw/gclaw/internal/model"
	"github.com/openclaw/gclaw/internal/session"
	"github.com/openclaw/gclaw/internal/perm"
	"github.com/openclaw/gclaw/internal/provider"
	"github.com/openclaw/gclaw/internal/skill"
	"github.com/openclaw/gclaw/internal/task"
	"github.com/openclaw/gclaw/internal/tool"
	"github.com/openclaw/gclaw/internal/websearch"
	timetool "github.com/openclaw/gclaw/internal/tool/builtin/timetool"
	memtool "github.com/openclaw/gclaw/internal/tool/builtin/memory"
	clarifypkg "github.com/openclaw/gclaw/internal/tool/builtin/clarify"
	webtool "github.com/openclaw/gclaw/internal/tool/builtin/web"
	sessiontool "github.com/openclaw/gclaw/internal/tool/builtin/session"
	visiontool "github.com/openclaw/gclaw/internal/tool/builtin/vision"
	imagetool "github.com/openclaw/gclaw/internal/tool/builtin/image"
	ttstool "github.com/openclaw/gclaw/internal/tool/builtin/tts"
	"github.com/openclaw/gclaw/internal/imagegen"
	ttsbackend "github.com/openclaw/gclaw/internal/tts"
	browsertool "github.com/openclaw/gclaw/internal/tool/builtin/browser"
	mcptool "github.com/openclaw/gclaw/internal/tool/builtin/mcp"
	"github.com/openclaw/gclaw/internal/browser"
	"github.com/openclaw/gclaw/internal/mcp"

	// Blank imports trigger tool self-registration via init().
	_ "github.com/openclaw/gclaw/internal/tool/builtin/file"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/clarify"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/memory"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/search"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/session"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/shell"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/todo"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/web"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/vision"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/image"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/tts"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/codeexec"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/browser"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/mcp"
	"github.com/openclaw/gclaw/internal/tool/builtin/skill_tools"
	"github.com/openclaw/gclaw/internal/tool/builtin/meta"
)

// Version is set at build time.
var Version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("gclaw version %s\n", Version)
			return
		case "help", "--help", "-h":
			printHelp()
			return
		case "config":
			runConfig(os.Args[2:])
			return
		case "executor":
			// Reserved for remote bridge mode
			fmt.Println("executor mode not yet implemented")
			return
		}
	}

	runREPL()
}

func printHelp() {
	fmt.Println(`gclaw - High-performance autonomous agent

Usage:
  gclaw [command]

Commands:
  (no args)     Start interactive REPL mode
  config dump   Dump current config (with redaction)
  config path   Show config file paths
  executor      Start in remote executor mode (bridge)
  version       Show version
  help          Show this help

Configuration files:
  Project: .gclaw/config.yaml (auto-discovered by walking up)
  User:    ~/.gclaw/config.yaml

Environment variables:
  GCLAW_MODEL             Override default model
  GCLAW_AUTONOMY          Override autonomy level
  GCLAW_PERMISSION_MODE   Override permission mode
  GCLAW_LOG_LEVEL         Override log level`)
}

func runConfig(args []string) {
	userPath, _ := config.UserConfigPath()
	cwd, _ := os.Getwd()
	projectPath := config.FindProjectConfig(cwd)

	switch {
	case len(args) > 0 && args[0] == "path":
		fmt.Println("Project config:", projectPath)
		fmt.Println("User config:", userPath)
	case len(args) > 0 && args[0] == "dump":
		fmt.Println("Config paths:")
		fmt.Println("  Project:", projectPath)
		fmt.Println("  User:", userPath)
		fmt.Println("\nLoading config...")
		cfg, err := config.Load(projectPath, userPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		cfg.Model.Providers = config.MaskKeys(cfg.Model.Providers)
		fmt.Printf("\n%+#v\n", cfg)
	default:
		fmt.Println("Usage: gclaw config [dump|path]")
	}
}

func runREPL() {
	cwd, _ := os.Getwd()
	userPath, _ := config.UserConfigPath()
	projectPath := config.FindProjectConfig(cwd)

	cfg, err := config.Load(projectPath, userPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	setupLogging(cfg.Logging.Level)
	slog.Info("gclaw starting", "version", Version, "autonomy", cfg.Agent.Autonomy)

	// Initialize components
	autonomyLevel := parseAutonomy(cfg.Agent.Autonomy)
	toolRegistry := tool.GlobalRegistry
	providerFactory := setupProviders(cfg)
	modelProvider := resolveModel(providerFactory, cfg)
	permChecker := setupPermissions(cfg)
	ctxManager := setupContext(cfg, providerFactory)
	taskMgr := task.NewManager(10)

	// Initialize memory tool
	if cfg.Memory.Enabled {
		memDir := cfg.Memory.Dir
		if memDir == "" {
			memDir = config.ExpandPath("~/.gclaw/memory")
		}
		os.MkdirAll(memDir, 0755)
		memtool.Dir = memDir
	}

	// Initialize skill system
	var skillMgr *skill.Manager
	if cfg.Skills.Enabled {
		skillMgr = skill.NewManager()
		skillDir := cfg.Skills.Dir
		if skillDir == "" {
			skillDir = config.ExpandPath("~/.gclaw/skills")
		}

		// Load project-bundled skills first (lowest priority)
		projectSkillsDir := cfg.Skills.ProjectDir
		if projectSkillsDir == "" && projectPath != "" {
			projectSkillsDir = filepath.Join(filepath.Dir(projectPath), "skills")
		}
		if projectSkillsDir != "" {
			if err := skillMgr.LoadProject(projectSkillsDir); err != nil {
				slog.Warn("skill: failed to load project skills", "dir", projectSkillsDir, "error", err)
			}
		}

		if err := skillMgr.LoadAll(skillDir); err != nil {
			slog.Warn("skill: failed to load skills", "error", err)
		} else {
			slog.Info("skill: loaded", "count", len(skillMgr.List()))
		}
		skill_tools.ManagerRef = skillMgr
	}

	// Initialize memory system
	var memoryMgr *memory.Manager
	if cfg.Memory.Enabled {
		memDir := cfg.Memory.Dir
		if memDir == "" {
			memDir = config.ExpandPath("~/.gclaw/memory")
		}
		fp := memory.NewFileProvider(memDir)
		memoryMgr = memory.NewManager(fp)
		if err := memoryMgr.Initialize("default"); err != nil {
			slog.Warn("memory: failed to initialize", "error", err)
		}
	}

	systemPrompt := defaultSystemPrompt() + "\n" + autonomous.SystemPrompt(autonomous.ParseLevel(cfg.Agent.Autonomy))
	if skillMgr != nil {
		systemPrompt += skillMgr.ForSystemPrompt()
	}
	if memoryMgr != nil {
		systemPrompt += memoryMgr.SystemPromptBlock()
	}

	// Initialize session store
	var currentSessionID string
	var sessionStore session.Store
	if cfg.Session.Enabled {
		dbPath := cfg.Session.DBPath
		if dbPath == "" {
			dbPath = config.ExpandPath("~/.gclaw/sessions.db")
		}
		maxSessions := cfg.Session.MaxSessions
		if maxSessions <= 0 {
			maxSessions = 100
		}
		store, err := session.NewStore(dbPath, maxSessions)
		if err != nil {
			slog.Warn("session: failed to open store", "error", err)
		} else {
			sessionStore = store
			currentSessionID, _ = store.CreateSession("repl")
			defer store.Close()
		}
	}

	// Configure main REPL agent
	ag := agent.New(agent.Config{
		Model:        modelProvider,
		Tools:        toolRegistry,
		SystemPrompt: systemPrompt,
		MaxTurns:     100,
		Autonomy:     autonomyLevel,
		Permissions:  permChecker,
		ContextMgr:   ctxManager,
	})

	fmt.Printf("gclaw %s — %s mode | %s | type /help\n\n", Version, cfg.Agent.Autonomy, cfg.Model.Default)

	// Setup gateway
	var gw *gateway.Gateway
	if cfg.Gateway.Enabled {
		gw = gateway.New(nil)
		// REPL is always a platform
		if cfg.Gateway.Platforms["repl"].Enabled || len(cfg.Gateway.Platforms) == 0 {
			gw.Register("repl", gw_adapter.NewREPL())
		}
	}

	// Setup weixin channel
	var weixinCh *weixin.Channel
	if cfg.Channels.Weixin.Enabled {
		fmt.Print("正在初始化微信通道...")
		var err error
		weixinCh, err = weixin.New(weixin.Config{
			Verbose: cfg.Channels.Weixin.Verbose,
			OnMessageHandled: func() {
				fmt.Print("> ")
			},
		})
		if err != nil {
			fmt.Printf(" 失败: %v\n", err)
		} else {
			// Separate agent instance for WeChat with its own conversation history.
			weixinAgent := agent.New(agent.Config{
				Model:        modelProvider,
				Tools:        toolRegistry,
				SystemPrompt: defaultSystemPrompt(),
				MaxTurns:     20,
				Autonomy:     agent.Interactive,
				Permissions:  permChecker,
			})
			weixinCh.SetAgent(weixinAgent)
			if weixinCh.HasStoredAccount() {
				fmt.Println(" 发现已绑定账号")
			}
			if gw != nil {
				gw.Register("weixin", weixinCh)
				slog.Info("gateway: weixin platform registered")
			}
			go func() {
				bus := autonomous.NewEventBus()
				if err := weixinCh.Start(stdctx.Background(), bus); err != nil {
					fmt.Fprintf(os.Stderr, "\n微信通道启动失败: %v\n", err)
				}
			}()
			time.Sleep(200 * time.Millisecond)
		}
	}

	// Setup cron scheduler
	var cronSched *cron.Scheduler
	if cfg.Cron.Enabled && len(cfg.Cron.Jobs) > 0 {
		cronModel := modelProvider
		if cfg.Cron.Model != "" {
			if m, err := providerFactory.Build(cfg.Cron.Model); err == nil {
				cronModel = m
			} else {
				slog.Warn("cron: model not found, using default", "model", cfg.Cron.Model, "error", err)
			}
		}
		cronSystemPrompt := defaultSystemPrompt()
		if skillMgr != nil {
			cronSystemPrompt += skillMgr.ForSystemPrompt()
		}
		cronAgent := agent.New(agent.Config{
			Model:        cronModel,
			Tools:        toolRegistry,
			SystemPrompt: cronSystemPrompt,
			MaxTurns:     10,
			Autonomy:     agent.Interactive,
			Permissions:  permChecker,
		})
		cronSched = cron.NewScheduler(cronAgent)

		for _, j := range cfg.Cron.Jobs {
			job := &cron.Job{
				Name:         j.Name,
				Schedule:     j.Schedule,
				Prompt:       j.Prompt,
				Enabled:      j.Enabled,
				NotifyWeixin: j.NotifyWeixin,
			}

			// Wire WeChat notification if configured.
			if j.NotifyWeixin {
				job.OnResult = func(name, prompt, response string) {
					if weixinCh == nil {
						slog.Warn("cron: OnResult skipped, weixin not enabled")
						return
					}
					if !weixinCh.IsConnected() {
						slog.Warn("cron: OnResult skipped, weixin not connected")
						return
					}
					to := weixinCh.LastUserID()
					if to == "" {
						slog.Warn("cron: OnResult skipped, no WeChat user (send a message in WeChat first)")
						return
					}
					slog.Info("cron: pushing result to WeChat", "job", name, "to", to, "len", len(response))
					msg := fmt.Sprintf("[定时任务 %s]\n%s", name, response)
					if err := weixinCh.Send(stdctx.Background(), to, msg); err != nil {
						slog.Warn("cron: push to weixin failed", "error", err)
					} else {
						slog.Info("cron: pushed to WeChat successfully")
					}
				}
			}

			if err := cronSched.AddJob(job); err != nil {
				slog.Error("failed to add cron job", "name", j.Name, "error", err)
				continue
			}
		}
		cronSched.Start()
		defer cronSched.Stop()
		fmt.Printf("Cron scheduler active: %d jobs loaded.\n", len(cfg.Cron.Jobs))
	}

	// Setup delegate dispatcher and meta tool references
	if cfg.Delegate.Enabled {
		factory := func() delegate.AgentRunner {
			return agent.New(agent.Config{
				Model:        modelProvider,
				Tools:        toolRegistry,
				SystemPrompt: defaultSystemPrompt(),
				MaxTurns:     10,
				Autonomy:     agent.Interactive,
				Permissions:  permChecker,
			})
		}
		dispatcher := delegate.NewDispatcher(
			factory,
			cfg.Delegate.MaxConcurrent,
			cfg.Delegate.MaxDepth,
		)
		meta.DispatcherRef = dispatcher
	}

	meta.CronSchedRef = cronSched
	if weixinCh != nil {
		meta.WeixinChRef = weixinCh
	}
	meta.TaskMgrRef = taskMgr

	// Wire session search tool
	if sessionStore != nil {
		sessiontool.StoreRef = sessionStore
	}

	// Wire web search tool
	searchFactory := websearch.NewDefaultFactory(cfg.WebSearch.Backend)
	if backends := searchFactory.Available(); len(backends) > 0 {
		webtool.SearchFactory = searchFactory
		slog.Info("websearch: available backends", "backends", backends)
	}

	// Wire web extract LLM summarization (optional)
	webtool.ModelFn = func(ctx stdctx.Context, prompt string) (string, error) {
		resp, err := modelProvider.Call(ctx, model.CallParams{
			SystemPrompt: "Summarize the following web page content concisely, preserving key information.",
			Messages:     []model.Message{{Role: "user", Content: prompt}},
			MaxTokens:    4096,
		})
		if err != nil {
			return "", err
		}
		return resp.Text, nil
	}

	
		// Wire vision tool - pass model reference for image analysis
		visiontool.ModelRef = modelProvider

		// Wire image generation tool
		imgFactory := imagegen.NewDefaultFactory("")
		if backends := imgFactory.Available(); len(backends) > 0 {
			imagetool.GenFactory = imgFactory
			slog.Info("imagegen: available backends", "backends", backends)
		}

		// Wire TTS tool
		ttsFactory := ttsbackend.NewDefaultFactory("")
		if backends := ttsFactory.Available(); len(backends) > 0 {
			ttstool.TTSFactory = ttsFactory
			slog.Info("tts: available backends", "backends", backends)
		}
		
		// Wire browser automation (optional - requires Chrome/Chromium)
		if cfg.Agent.BrowserEnabled {
			b, err := browser.NewChromedpBrowser()
			if err != nil {
				slog.Warn("browser: failed to start", "error", err)
			} else {
				browsertool.BrowserRef = b
				slog.Info("browser: headless Chrome started")
			}
		}

		// Wire MCP client (optional)
		if len(cfg.MCP.Servers) > 0 {
			mgr := mcp.NewManager()
			for _, srv := range cfg.MCP.Servers {
				mgr.AddServer(mcp.ServerConfig{
					Name:    srv.Name,
					Command: srv.Command,
					URL:     srv.URL,
					Env:     srv.Env,
				})
			}
			mcptool.ManagerRef = mgr
			slog.Info("mcp: configured servers", "count", len(cfg.MCP.Servers))
		}
		// Setup autonomous scheduler for semi/full modes
	var scheduler *autonomous.Scheduler
	if autonomyLevel >= agent.SemiAutonomous {
		tickInterval, err := config.Duration(cfg.Agent.TickInterval)
		if err != nil {
			tickInterval = 30 * time.Second
		}
		idleSleep, err := config.Duration(cfg.Agent.IdleSleep)
		if err != nil {
			idleSleep = 5 * time.Minute
		}

		sleepTool, _ := toolRegistry.Get("SleepTool")
		sleepToolInstance := sleepTool.(*timetool.SleepTool)

		scheduler = autonomous.NewScheduler(autonomous.Config{
			Level:        autonomous.ParseLevel(cfg.Agent.Autonomy),
			TickInterval: tickInterval,
			IdleSleep:    idleSleep,
		}, ag)

		// Wire up the sleeper to SleepTool
		sleepToolInstance.Sleeper = scheduler.Sleeper()

		scheduler.Start()
		defer scheduler.Stop()
		fmt.Println("Autonomous mode active. The agent will work independently.")
		fmt.Println("Enter messages to send to the agent, or /help for commands.")
	}

	scanner := bufio.NewScanner(os.Stdin)

	// Wire clarify tool callback for interactive mode
	clarifypkg.Callback = func(question string, options []clarifypkg.Option) (string, error) {
		fmt.Println()
		fmt.Printf("? %s\n", question)
		if len(options) > 0 {
			for i, o := range options {
				fmt.Printf("  %d. %s", i+1, o.Label)
				if o.Description != "" {
					fmt.Printf(" - %s", o.Description)
				}
				fmt.Println()
			}
			fmt.Print("Choose (number or text): ")
		} else {
			fmt.Print("Your answer: ")
		}
		if !scanner.Scan() {
			return "", fmt.Errorf("input ended")
		}
		answer := strings.TrimSpace(scanner.Text())
		if len(options) > 0 {
			idx := 0
			if _, err := fmt.Sscanf(answer, "%d", &idx); err == nil && idx >= 1 && idx <= len(options) {
				return options[idx-1].Label, nil
			}
		}
		return answer, nil
	}

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Handle slash commands
		if strings.HasPrefix(input, "/") {
			handleCommand(input, cfg, ctxManager, taskMgr, ag, scheduler, cronSched, weixinCh)
			continue
		}

		// In autonomous mode, publish user input as event
		if scheduler != nil {
			scheduler.Publish(autonomous.Event{
				Type:    autonomous.EventUser,
				Source:  "cli",
				Payload: input,
				Time:    time.Now(),
			})
			fmt.Println("(message sent to autonomous agent)")
			continue
		}

		// Interactive mode: run agent loop directly
		slog.Debug("running agent", "input", input)

		agentInput := input
		if memoryMgr != nil {
			if ctx := memoryMgr.Prefetch(stdctx.Background(), input, cfg.Memory.PrefetchLimit); ctx != "" {
				agentInput = ctx + "\n\nUser message: " + input
			}
		}

		response, err := ag.Run(stdctx.Background(), agentInput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
			continue
		}

		if memoryMgr != nil {
			memoryMgr.SyncTurn(stdctx.Background(), input, response)
		}

		if sessionStore != nil {
			_ = sessionStore.AddMessage(currentSessionID, "user", input, "", "", "")
			_ = sessionStore.AddMessage(currentSessionID, "assistant", response, "", "", "")
		}

		fmt.Println()
		fmt.Println(response)
		fmt.Println()
	}
}

func handleCommand(cmd string, cfg *config.Config, ctxMgr *context.Manager, taskMgr *task.Manager, ag *agent.Agent, scheduler *autonomous.Scheduler, cronSched *cron.Scheduler, weixinCh *weixin.Channel) {
	switch {
	case cmd == "/help":
		fmt.Println(`
Commands:
  /help        Show this help
  /stats       Show context and usage stats
  /autonomy    Show autonomous scheduler stats
  /cron        Show cron job status
  /weixin      WeChat channel: login|logout|status
  /tasks       List running tasks
  /config      Show current config
  /compact     Force context compaction
  /interrupt   Inject a message into the running agent (autonomous mode)
  /clear       Clear conversation
  /exit        Exit gclaw`)
	case cmd == "/stats":
		fmt.Println("\n--- Context Stats ---")
		fmt.Println(ctxMgr.UsageStats())
		fmt.Println("--- Model Stats ---")
		fmt.Printf("Input tokens: %d\n", ag.Usage().InputTokens)
		fmt.Printf("Output tokens: %d\n", ag.Usage().OutputTokens)
	case cmd == "/cron":
		if cronSched != nil {
			jobs := cronSched.Jobs()
			fmt.Printf("\n--- Cron Jobs (%d) ---\n", len(jobs))
			for _, j := range jobs {
				fmt.Printf("  %s:\n", j.Name)
				fmt.Printf("    schedule: %s\n", j.Schedule)
				fmt.Printf("    next_run: %s\n", j.NextRun.Format("15:04:05"))
				fmt.Printf("    run_count: %d\n", j.RunCount)
				fmt.Printf("    notify_weixin: %v\n", j.NotifyWeixin)
			}
		} else {
			fmt.Println("Cron scheduler is not active (add cron.jobs in config)")
		}
	case strings.HasPrefix(cmd, "/weixin"):
		if weixinCh == nil {
			fmt.Println("微信通道未启用 (设置 channels.weixin.enabled: true)")
		} else {
			handleWeixinCommand(cmd, weixinCh)
		}
	case cmd == "/autonomy":
		if scheduler != nil {
			stats := scheduler.Stats()
			fmt.Println("\n--- Autonomous Scheduler ---")
			for k, v := range stats {
				fmt.Printf("%s: %v\n", k, v)
			}
		} else {
			fmt.Println("Autonomous mode is not active (set agent.autonomy: semi or full in config)")
		}
	case cmd == "/tasks":
		tasks := taskMgr.List()
		fmt.Printf("\n%d tasks:\n", len(tasks))
		for _, t := range tasks {
			fmt.Printf("  [%s] %s %s\n", t.Status, t.Type, t.Description)
		}
	case cmd == "/compact":
		ctxMgr.Compact(10)
		fmt.Println("Context compacted.")
	case strings.HasPrefix(cmd, "/interrupt"):
		if !ag.IsBusy() {
			fmt.Println("Agent is not currently running.")
			break
		}
		msg := strings.TrimSpace(strings.TrimPrefix(cmd, "/interrupt"))
		if msg == "" {
			fmt.Println("Usage: /interrupt <message>")
			break
		}
		ag.Interrupt(msg)
		fmt.Printf("Interrupt sent: %q\n", msg)
	case cmd == "/config":
		fmt.Println("\n--- Config ---")
		fmt.Printf("Model: %s\n", cfg.Model.Default)
		fmt.Printf("Fallback: %v\n", cfg.Model.Fallback)
		fmt.Printf("Autonomy: %s\n", cfg.Agent.Autonomy)
		fmt.Printf("Max turns: %d\n", cfg.Agent.MaxTurns)
		fmt.Printf("Permission: %s\n", cfg.Permission.Mode)
	case cmd == "/clear":
		ctxMgr.Reset()
		ag.Reset()
		fmt.Println("Conversation cleared.")
	case cmd == "/exit":
		if scheduler != nil {
			scheduler.Stop()
		}
		if weixinCh != nil {
			weixinCh.Stop()
		}
		os.Exit(0)
	default:
		fmt.Printf("Unknown command: %s (type /help)\n", cmd)
	}
}

func handleWeixinCommand(cmd string, ch *weixin.Channel) {
	args := strings.Fields(cmd)
	if len(args) < 2 {
		fmt.Println("Usage: /weixin login|logout|status")
		return
	}
	switch args[1] {
	case "login":
		fmt.Println("正在启动微信扫码登录...")
		if err := ch.Login(); err != nil {
			fmt.Fprintf(os.Stderr, "登录失败: %v\n", err)
		}
	case "logout":
		fmt.Println("正在解绑微信账号...")
		if err := ch.Logout(); err != nil {
			fmt.Fprintf(os.Stderr, "解绑失败: %v\n", err)
		}
	case "status":
		s := ch.Status()
		fmt.Println("\n--- 微信通道 ---")
		if s.Connected {
			fmt.Println("状态: 已连接")
			fmt.Printf("账号: %s\n", s.AccountID)
			fmt.Printf("用户: %s\n", s.UserID)
			if !s.LastMsgAt.IsZero() {
				fmt.Printf("最后消息: %s\n", s.LastMsgAt.Format("15:04:05"))
			}
			fmt.Printf("消息数: %d\n", s.MsgCount)
		} else {
			fmt.Println("状态: 未连接")
			if s.AccountID != "" {
				fmt.Printf("账号: %s\n", s.AccountID)
			}
		}
	default:
		fmt.Println("Usage: /weixin login|logout|status")
	}
}

func setupProviders(cfg *config.Config) *provider.Factory {
	f := provider.DefaultFactory()

	// Merge providers from config file (overrides auto-detected ones)
	for name, p := range cfg.Model.Providers {
		keys := config.GetProviderKeys(p)
		apiKey := ""
		if len(keys) > 0 {
			apiKey = keys[0]
		}
		models := p.Models
		if len(models) == 0 {
			if p.Model != "" {
				models = []string{p.Model}
			} else {
				models = []string{cfg.Model.Default}
			}
		}
		for _, modelName := range models {
			f.Register(modelName, provider.Config{
				Type:             resolveProviderType(name),
				Model:            modelName,
				APIKey:           apiKey,
				Keys:             keys,
				BaseURL:          p.BaseURL,
				Endpoint:         p.Endpoint,
				SupportsThinking: p.Thinking,
			})
		}
	}

	return f
}

func resolveProviderType(name string) string {
	switch name {
	case "anthropic", "claude":
		return "claude"
	case "openai":
		return "openai"
	case "deepseek", "zhipu", "qianfan", "moonshot":
		return "openai-compatible"
	case "ollama":
		return "ollama"
	default:
		return name
	}
}

func resolveModel(f *provider.Factory, cfg *config.Config) model.Model {
	names := f.Names()
	if len(names) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no model providers configured\n")
		os.Exit(1)
	}

	// Try configured default as provider name
	if cfg.Model.Default != "" {
		m, err := f.Build(cfg.Model.Default)
		if err == nil {
			return m
		}
		slog.Warn("cannot build default provider", "name", cfg.Model.Default, "error", err)
	}

	// Try fallback chain
	for _, name := range cfg.Model.Fallback {
		m, err := f.Build(name)
		if err == nil {
			slog.Info("using fallback provider", "name", name)
			return m
		}
		slog.Warn("fallback provider failed", "name", name, "error", err)
	}

	// Last resort: first available registered provider
	for _, name := range names {
		if name == cfg.Model.Default || findInFallback(cfg.Model.Fallback, name) {
			continue
		}
		m, err := f.Build(name)
		if err == nil {
			slog.Info("using auto-selected provider", "name", name)
			return m
		}
	}

	fmt.Fprintf(os.Stderr, "Error: no available model provider\n")
	os.Exit(1)
	return nil
}

func findInFallback(fallback []string, name string) bool {
	for _, f := range fallback {
		if f == name {
			return true
		}
	}
	return false
}

func setupPermissions(cfg *config.Config) *perm.Checker {
	var rules []perm.Rule
	for _, r := range cfg.Permission.Rules {
		if r.Allow != "" {
			rules = append(rules, perm.Rule{Action: "allow", Pattern: r.Allow})
		}
		if r.Deny != "" {
			rules = append(rules, perm.Rule{Action: "deny", Pattern: r.Deny})
		}
		if r.Ask != "" {
			rules = append(rules, perm.Rule{Action: "ask", Pattern: r.Ask})
		}
	}
	return perm.NewChecker(perm.Mode(cfg.Permission.Mode), rules)
}

func setupContext(cfg *config.Config, factory *provider.Factory) *context.Manager {
	ctxCfg := context.Config{
		MaxTokens:    cfg.Context.MaxTokens,
		CompactAt:    cfg.Context.CompactAt,
		ReserveRatio: cfg.Context.ReserveRatio,
		SystemPrompt: defaultSystemPrompt() + "\n" + autonomous.SystemPrompt(autonomous.ParseLevel(cfg.Agent.Autonomy)),
	}

	if cfg.Context.CompressorEnabled && cfg.Context.CompressorModel != "" {
		compModel, err := factory.Build(cfg.Context.CompressorModel)
		if err != nil {
			slog.Warn("compressor model not available, falling back to truncation", "model", cfg.Context.CompressorModel, "error", err)
			return context.NewManager(ctxCfg)
		}
		slog.Info("context: LLM compressor enabled", "model", cfg.Context.CompressorModel)
		compactor := context.NewLLMCompactor(compModel, 1, 10, 2048)
		return context.NewManagerWithCompactor(ctxCfg, compactor)
	}

	return context.NewManager(ctxCfg)
}

func setupLogging(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetLogLoggerLevel(l)
}

func parseAutonomy(s string) agent.AutonomyLevel {
	switch s {
	case "full":
		return agent.FullyAutonomous
	case "semi":
		return agent.SemiAutonomous
	default:
		return agent.Interactive
	}
}

func defaultSystemPrompt() string {
	return `You are gclaw, a helpful and versatile autonomous assistant.

Core rules:
- Answer directly from your knowledge when possible. Do NOT call tools for simple factual questions, general knowledge, summaries, translations, or explanations.
- Only use tools (Bash, ReadFile, WriteFile, Glob, Grep) when the task genuinely requires file access, code execution, or current data from the internet.
- If a tool fails twice in a row, STOP and tell the user what went wrong. Never retry the same approach more than twice.
- Be concise and direct. Respond in the user's language.
- Do not list your capabilities unless asked.`
}
