package main

import (
	"bytes"
	stdctx "context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/openclaw/gclaw/internal/agent"
	"github.com/openclaw/gclaw/internal/autonomous"
	"github.com/openclaw/gclaw/internal/channel/weixin"
	"github.com/openclaw/gclaw/internal/config"
	"github.com/openclaw/gclaw/internal/context"
	"github.com/openclaw/gclaw/internal/cron"
	"github.com/openclaw/gclaw/internal/delegate"
	curatorpkg "github.com/openclaw/gclaw/internal/curator"
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
	"github.com/openclaw/gclaw/internal/tui"
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
	"github.com/openclaw/gclaw/internal/checkpoint"
	"github.com/openclaw/gclaw/internal/mcp"

	// Blank imports trigger tool self-registration via init().
	filetool "github.com/openclaw/gclaw/internal/tool/builtin/file"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/clarify"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/memory"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/search"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/session"
	shelltool "github.com/openclaw/gclaw/internal/tool/builtin/shell"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/todo"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/web"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/vision"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/image"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/tts"
	_ "github.com/openclaw/gclaw/internal/tool/builtin/video"
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
	earlyLogBuf := tui.NewLogBuffer(1000)
	slog.SetDefault(slog.New(earlyLogBuf.SlogHandler()))
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

		// Install builtin skills on first run
		if n, err := skill.InstallBuiltin(skillDir); err != nil {
			slog.Warn("skill: failed to install builtin", "error", err)
		} else if n > 0 {
			slog.Info("skill: installed builtin", "count", n)
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
		slog.Info("initializing weixin channel")
		var err error
		weixinCh, err = weixin.New(weixin.Config{
			Verbose: cfg.Channels.Weixin.Verbose,
			OnMessageHandled: func() {
				
			},
		})
		if err != nil {
			slog.Warn("weixin init failed", "error", err)
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
				slog.Info("weixin: stored account found")
			}
			if gw != nil {
				gw.Register("weixin", weixinCh)
				slog.Info("gateway: weixin platform registered")
			}
			go func() {
				bus := autonomous.NewEventBus()
				if err := weixinCh.Start(stdctx.Background(), bus); err != nil {
					slog.Warn("weixin start failed", "error", err)
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
		scriptTimeout, err := config.Duration(cfg.Cron.ScriptTimeout)
		if err != nil {
			scriptTimeout = 120 * time.Second
		}
		scriptsDir := cfg.Cron.ScriptsDir
		if scriptsDir == "" {
			scriptsDir = config.ExpandPath("~/.gclaw/scripts")
		}
		os.MkdirAll(scriptsDir, 0755)

		cronSched = cron.NewScheduler(cronAgent,
			cron.WithScriptTimeout(scriptTimeout),
			cron.WithScriptsDir(scriptsDir),
		)

		for _, j := range cfg.Cron.Jobs {
			job := &cron.Job{
				Name:         j.Name,
				Schedule:     j.Schedule,
				Prompt:       j.Prompt,
				Enabled:      j.Enabled,
				NotifyWeixin: j.NotifyWeixin,
				Script:       j.Script,
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
	}

	// Setup delegate dispatcher and meta tool references
	var mcpMgr *mcp.Manager
	var factory delegate.AgentFactory
	if cfg.Delegate.Enabled {
		factory = func() delegate.AgentRunner {
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
			mcpMgr = mcp.NewManager()
			for _, srv := range cfg.MCP.Servers {
				mcpMgr.AddServer(mcp.ServerConfig{
					Name:    srv.Name,
					Command: srv.Command,
					URL:     srv.URL,
					Env:     srv.Env,
				})
			}
			mcptool.ManagerRef = mcpMgr
			slog.Info("mcp: configured servers", "count", len(cfg.MCP.Servers))
		}

		// Wire checkpoint manager
		if cfg.Checkpoint.Enabled {
			cpMgr := checkpoint.NewManager(true, cfg.Checkpoint.MaxSnapshots, config.ExpandPath("~/.gclaw/checkpoints"))
			filetool.CheckpointManager = cpMgr
			shelltool.CheckpointManager = cpMgr
			slog.Info("checkpoint manager enabled", "max_snapshots", cfg.Checkpoint.MaxSnapshots)
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
	}

	// Initialize curator (auto skill maintenance)
	var curatorInst *curatorpkg.Curator
	if cfg.Curator.Enabled && skillMgr != nil {
		curatorCfg := curatorpkg.Config{
			Enabled:      true,
			Interval:     time.Duration(cfg.Curator.IntervalH) * time.Hour,
			MinIdle:      time.Duration(cfg.Curator.MinIdleH) * time.Hour,
			StaleAfter:   time.Duration(cfg.Curator.StaleAfterD) * 24 * time.Hour,
			ArchiveAfter: time.Duration(cfg.Curator.ArchiveAfterD) * 24 * time.Hour,
		}
		if curatorCfg.Interval <= 0 {
			curatorCfg.Interval = 7 * 24 * time.Hour
		}
		curatorInst = curatorpkg.New(curatorCfg, skillMgr, factory)
		if scheduler != nil {
			scheduler.SetOnIdleHook(func(idle time.Duration) {
				curatorInst.MaybeRun(stdctx.Background(), idle)
			})
		}
		slog.Info("curator enabled", "interval", curatorCfg.Interval)
	}

	// Wire TUI
	theme := tui.LoadTheme(cfg.TUI.Theme)
	logBuf := earlyLogBuf
	hist := tui.NewHistory(config.ExpandPath("~/.gclaw/history"), cfg.TUI.History.MaxEntries)
	compEng := tui.DefaultCompletionEngine()

	clarifypkg.Callback = func(question string, options []clarifypkg.Option) (string, error) {
		return "", fmt.Errorf("clarify not yet supported in TUI mode")
	}

	cmdContext := &cmdCtx{
		cfg:             cfg,
		ctxMgr:          ctxManager,
		taskMgr:         taskMgr,
		ag:              ag,
		scheduler:       scheduler,
		cronSched:       cronSched,
		weixinCh:        weixinCh,
		providerFactory: providerFactory,
		memoryMgr:       memoryMgr,
		sessionStore:    sessionStore,
		skillMgr:        skillMgr,
		curatorInst:     curatorInst,
		mcpMgr:          mcpMgr,
		gw:              gw,
		logBuf:          logBuf,
	}

	var app *tui.App
	app = tui.NewApp(tui.Deps{
		Config:  cfg,
		Agent:   ag,
		Theme:   theme,
		LogBuf:  logBuf,
		History: hist,
		CompEng: compEng,
		OnSubmit: func(ctx stdctx.Context, input string, images []string) (string, error) {
			if scheduler != nil {
				scheduler.Publish(autonomous.Event{
					Type:    autonomous.EventUser,
					Source:  "cli",
					Payload: input,
					Time:    time.Now(),
				})
				return "(message sent to autonomous agent)", nil
			}

			agentInput := input
			if memoryMgr != nil {
				if memCtx := memoryMgr.Prefetch(ctx, input, cfg.Memory.PrefetchLimit); memCtx != "" {
					agentInput = memCtx + "\n\nUser message: " + input
				}
			}

			response, err := ag.Run(ctx, agentInput)
			if err != nil {
				return "", err
			}

			if memoryMgr != nil {
				memoryMgr.SyncTurn(ctx, input, response)
			}
			if sessionStore != nil {
				_ = sessionStore.AddMessage(currentSessionID, "user", input, "", "", "")
				_ = sessionStore.AddMessage(currentSessionID, "assistant", response, "", "", "")
			}
			return response, nil
		},
		OnSlash: func(cmd string) {
			var buf bytes.Buffer
			handleCommand(cmd, cmdContext, &buf)
			output := strings.TrimRight(buf.String(), "\n")
			if output != "" {
				app.AppendEvent("cmd", strings.Fields(cmd)[0], output)
			}
		},
		GetAgentCount: func() int {
			if scheduler == nil {
				return 0
			}
			stats := scheduler.Stats()
			if busy, ok := stats["agent_busy"].(bool); ok && busy {
				return 1
			}
			return 0
		},
		GetBgTasks: func() int {
			if taskMgr == nil {
				return 0
			}
			return len(taskMgr.ListByStatus(task.StatusRunning)) + len(taskMgr.ListByStatus(task.StatusPending))
		},
		IsCronActive: func() bool {
			return cronSched != nil
		},
	})
		app.AppendWelcome("type /help for commands")
	app.SetBanner(Version, cfg.Model.Default, cfg.Agent.Autonomy)
		

	p := tea.NewProgram(app)
	app.SetSend(p.Send)
	if ag != nil {
		ag.OnToolStart = func(name, detail string) {
			slog.Info("TUI tool start", "name", name, "detail", detail)
			p.Send(tui.ToolStartEvent{Name: name, Detail: detail})
		}
		ag.OnToolEnd = func(name string, output string, err error, duration time.Duration) {
			slog.Info("TUI tool end", "name", name, "err", err, "duration", duration)
			p.Send(tui.ToolEndEvent{Name: name, Output: output, Err: err, Duration: duration})
		}
	}
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
	}

		hist.Save()
}

// cmdCtx holds all runtime dependencies needed by slash commands.
type cmdCtx struct {
	cfg             *config.Config
	ctxMgr          *context.Manager
	taskMgr         *task.Manager
	ag              *agent.Agent
	scheduler       *autonomous.Scheduler
	cronSched       *cron.Scheduler
	weixinCh        *weixin.Channel
	providerFactory *provider.Factory
	memoryMgr       *memory.Manager
	sessionStore    session.Store
	skillMgr        *skill.Manager
	curatorInst     *curatorpkg.Curator
	mcpMgr          *mcp.Manager
	gw              *gateway.Gateway
	logBuf          *tui.LogBuffer
}

func handleCommand(cmd string, c *cmdCtx, w io.Writer) {
	args := strings.Fields(cmd)
	switch {
	// ---- Help ----
	case cmd == "/help":
		fmt.Fprintln(w, `
Session:
  /clear             Clear conversation and reset
  /compact           Force context compaction
  /interrupt <msg>   Inject message into running agent

Info:
  /help              Show this help
  /version           Show version
  /status            Show comprehensive status panel
  /stats             Show context and token usage
  /config            Show current configuration
  /model [name]      Show or switch active model
  /fallback          Show fallback model chain
  /tools [all]       List registered tools

Subsystems:
  /skills            List loaded skills
  /memory [list|clear]  Manage persistent memory
  /sessions          List session history
  /mcp               Show MCP server status
  /cron [run|pause|resume] <name>  Manage cron jobs
  /tasks             List running tasks

Channels:
  /weixin login|logout|status  WeChat channel
  /gateway           Show gateway platform status

Diagnostics:
  /doctor            Run system health checks
  /debug             Toggle debug logging
  /dump              Export state snapshot to file
  /backup            Backup ~/.gclaw directory

Scheduling:
  /autonomy          Show autonomous scheduler stats

Exit:
  /exit              Exit gclaw`)

	// ---- Session ----
	case cmd == "/clear":
		c.ctxMgr.Reset()
		c.ag.Reset()
		fmt.Fprintln(w, "Conversation cleared.")
	case cmd == "/compact":
		c.ctxMgr.Compact(10)
		fmt.Fprintln(w, "Context compacted.")
	case strings.HasPrefix(cmd, "/interrupt"):
		if !c.ag.IsBusy() {
			fmt.Fprintln(w, "Agent is not currently running.")
			break
		}
		msg := strings.TrimSpace(strings.TrimPrefix(cmd, "/interrupt"))
		if msg == "" {
			fmt.Fprintln(w, "Usage: /interrupt <message>")
			break
		}
		c.ag.Interrupt(msg)
		fmt.Fprintf(w, "Interrupt sent: %q\n", msg)

	// ---- Info ----
	case cmd == "/version":
		fmt.Fprintf(w, "gclaw version %s\n", Version)
	case cmd == "/status":
		fmt.Fprintln(w, "\n--- gclaw Status ---")
		fmt.Fprintf(w, "Version:    %s\n", Version)
		fmt.Fprintf(w, "Model:      %s\n", c.cfg.Model.Default)
		fmt.Fprintf(w, "Autonomy:   %s\n", c.cfg.Agent.Autonomy)
		fmt.Fprintf(w, "Permission: %s\n", c.cfg.Permission.Mode)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "--- Tokens ---")
		u := c.ag.Usage()
		fmt.Fprintf(w, "Input:  %d\n", u.InputTokens)
		fmt.Fprintf(w, "Output: %d\n", u.OutputTokens)
		fmt.Fprintln(w)
		fmt.Fprintln(w, c.ctxMgr.UsageStats())
			fmt.Fprintln(w, "--- Subsystems ---")
			fmt.Fprintf(w, "Memory:     %s\n", boolStr(c.memoryMgr != nil, "enabled", "disabled"))
			skillCount := 0
			if c.skillMgr != nil { skillCount = len(c.skillMgr.List()) }
			fmt.Fprintf(w, "Skills:     %s\n", boolStr(c.skillMgr != nil, countStr(skillCount, "loaded"), "disabled"))
			fmt.Fprintf(w, "Checkpoint: %s\n", boolStr(c.cfg.Checkpoint.Enabled, "enabled", "disabled"))
			cronJobs := 0
			if c.cronSched != nil { cronJobs = len(c.cronSched.Jobs()) }
			fmt.Fprintf(w, "Cron:       %s\n", boolStr(c.cronSched != nil, countStr(cronJobs, "jobs"), "disabled"))
			fmt.Fprintf(w, "Gateway:    %s\n", boolStr(c.gw != nil, "enabled", "disabled"))
			fmt.Fprintf(w, "WeChat:     %s\n", weixinStatusStr(c.weixinCh))
			mcpServers := 0
			if c.mcpMgr != nil { mcpServers = len(c.mcpMgr.ListServers()) }
			fmt.Fprintf(w, "MCP:        %s\n", boolStr(c.mcpMgr != nil, countStr(mcpServers, "servers"), "disabled"))
			fmt.Fprintf(w, "Session:    %s\n", boolStr(c.sessionStore != nil, "enabled", "disabled"))
	case cmd == "/stats":
		fmt.Fprintln(w, "\n--- Context Stats ---")
		fmt.Fprintln(w, c.ctxMgr.UsageStats())
		fmt.Fprintln(w, "--- Model Stats ---")
		fmt.Fprintf(w, "Input tokens: %d\n", c.ag.Usage().InputTokens)
		fmt.Fprintf(w, "Output tokens: %d\n", c.ag.Usage().OutputTokens)
	case cmd == "/config":
		fmt.Fprintln(w, "\n--- Config ---")
		fmt.Fprintf(w, "Model: %s\n", c.cfg.Model.Default)
		fmt.Fprintf(w, "Fallback: %v\n", c.cfg.Model.Fallback)
		fmt.Fprintf(w, "Autonomy: %s\n", c.cfg.Agent.Autonomy)
		fmt.Fprintf(w, "Max turns: %d\n", c.cfg.Agent.MaxTurns)
		fmt.Fprintf(w, "Permission: %s\n", c.cfg.Permission.Mode)
		fmt.Fprintf(w, "Context max_tokens: %d\n", c.cfg.Context.MaxTokens)
		fmt.Fprintf(w, "Checkpoint: %v\n", c.cfg.Checkpoint.Enabled)
		fmt.Fprintf(w, "Delegate: %v\n", c.cfg.Delegate.Enabled)
	case cmd == "/model":
		fmt.Fprintf(w, "\nCurrent model: %s\n", c.cfg.Model.Default)
		fmt.Fprintln(w, "\nAvailable models:")
		for _, name := range c.providerFactory.Names() {
			marker := ""
			if name == c.cfg.Model.Default {
				marker = " (active)"
			}
			fmt.Fprintf(w, "  %s%s\n", name, marker)
		}
	case strings.HasPrefix(cmd, "/model "):
		name := strings.TrimSpace(strings.TrimPrefix(cmd, "/model "))
		m, err := c.providerFactory.Build(name)
		if err != nil {
			fmt.Fprintf(w, "Error: model %q not available: %v\n", name, err)
			break
		}
		c.ag.SetModel(m)
		c.cfg.Model.Default = name
		fmt.Fprintf(w, "Switched to model: %s\n", name)
	case cmd == "/fallback":
		fmt.Fprintln(w, "\n--- Fallback Chain ---")
		if len(c.cfg.Model.Fallback) == 0 {
			fmt.Fprintln(w, "  (none configured)")
		}
		for i, name := range c.cfg.Model.Fallback {
			fmt.Fprintf(w, "  %d. %s\n", i+1, name)
		}
	case strings.HasPrefix(cmd, "/fallback "):
		models := strings.Fields(strings.TrimPrefix(cmd, "/fallback "))
		c.cfg.Model.Fallback = models
		fmt.Fprintf(w, "Fallback chain updated: %v\n", models)

	case cmd == "/tools":
		listTools(c, false, w)
	case cmd == "/tools all":
		listTools(c, true, w)

	// ---- Subsystems ----
	case cmd == "/skills":
		if c.skillMgr == nil {
			fmt.Fprintln(w, "Skills system is not enabled (set skills.enabled: true)")
			break
		}
		fmt.Fprint(w, c.skillMgr.ForSystemIndex())
	
	case strings.HasPrefix(cmd, "/curator"):
		if c.curatorInst == nil {
			fmt.Fprintln(w, "Curator is not enabled (set curator.enabled: true)")
			break
		}
		sub := strings.TrimPrefix(cmd, "/curator")
		sub = strings.TrimSpace(sub)
		switch {
		case sub == "" || sub == "status":
			st := c.curatorInst.Status()
			fmt.Fprintln(w, "Curator status:")
			fmt.Fprintln(w, "  Paused:", st.Paused)
			fmt.Fprintln(w, "  Runs:", st.RunCount)
			if !st.LastRunAt.IsZero() {
				fmt.Fprintln(w, "  Last run:", st.LastRunAt.Format("2006-01-02 15:04:05"))
			}
			if st.LastSummary != "" {
				fmt.Fprintln(w, "  Summary:", st.LastSummary)
			}
		case sub == "run":
			fmt.Fprintln(w, "Running curator...")
			c.curatorInst.RunNow(stdctx.Background())
			fmt.Fprintln(w, "Curator pass complete.")
		case sub == "pause":
			c.curatorInst.Pause()
			fmt.Fprintln(w, "Curator paused.")
		case sub == "resume":
			c.curatorInst.Resume()
			fmt.Fprintln(w, "Curator resumed.")
		case strings.HasPrefix(sub, "restore "):
			name := strings.TrimPrefix(sub, "restore ")
			if err := c.skillMgr.UnarchiveSkill(name); err != nil {
				fmt.Fprintln(w, "Failed to restore:", err)
			} else {
				fmt.Fprintln(w, "Skill", name, "restored from archive.")
			}
		default:
			fmt.Fprintln(w, "Usage: /curator [status|run|pause|resume|restore <name>]")
		}

	case cmd == "/memory":
		if c.memoryMgr == nil {
			fmt.Fprintln(w, "Memory system is not enabled (set memory.enabled: true)")
			break
		}
		memDir := c.cfg.Memory.Dir
		if memDir == "" {
			memDir = config.ExpandPath("~/.gclaw/memory")
		}
		files, _ := os.ReadDir(memDir)
		fmt.Fprintf(w, "\n--- Memory ---\n")
		fmt.Fprintf(w, "Directory: %s\n", memDir)
		fmt.Fprintf(w, "Entries: %d\n", len(files))
	case cmd == "/memory list":
		if c.memoryMgr == nil {
			fmt.Fprintln(w, "Memory system is not enabled")
			break
		}
		memDir := c.cfg.Memory.Dir
		if memDir == "" {
			memDir = config.ExpandPath("~/.gclaw/memory")
		}
		entries, _ := os.ReadDir(memDir)
		fmt.Fprintf(w, "\n--- Memory Entries (%d) ---\n", len(entries))
		for _, e := range entries {
			fmt.Fprintf(w, "  %s\n", e.Name())
		}
	case cmd == "/memory clear":
		if c.memoryMgr == nil {
			fmt.Fprintln(w, "Memory system is not enabled")
			break
		}
		memDir := c.cfg.Memory.Dir
		if memDir == "" {
			memDir = config.ExpandPath("~/.gclaw/memory")
		}
		entries, _ := os.ReadDir(memDir)
		for _, e := range entries {
			os.Remove(filepath.Join(memDir, e.Name()))
		}
		fmt.Fprintf(w, "Cleared %d memory entries.\n", len(entries))
	case cmd == "/sessions":
		if c.sessionStore == nil {
			fmt.Fprintln(w, "Session store is not enabled (set session.enabled: true)")
			break
		}
		sessions, err := c.sessionStore.ListSessions()
		if err != nil {
			fmt.Fprintf(w, "Error listing sessions: %v\n", err)
			break
		}
		fmt.Fprintf(w, "\n--- Sessions (%d) ---\n", len(sessions))
		for _, s := range sessions {
			fmt.Fprintf(w, "  %s  agent=%s  msgs=%d  tokens=%d  %s\n",
				s.ID[:8], s.AgentType, s.MsgCount, s.TokenEst,
				s.StartTime.Format("01-02 15:04"))
		}
	case cmd == "/mcp":
		if c.mcpMgr == nil {
			fmt.Fprintln(w, "MCP is not configured (add mcp.servers in config)")
			break
		}
		servers := c.mcpMgr.ListServers()
		statuses := c.mcpMgr.ServerStatus()
		fmt.Fprintf(w, "\n--- MCP Servers (%d) ---\n", len(servers))
		for _, name := range servers {
			connected := statuses[name]
			fmt.Fprintf(w, "  %-20s %s\n", name, boolStr(connected, "connected", "disconnected"))
		}

	// ---- Cron (extended) ----
	case cmd == "/cron":
		cmdCronList(c, w)
	case len(args) >= 3 && args[0] == "/cron":
		cmdCronSubcommand(c, args[1], args[2:], w)

	// ---- Tasks ----
	case cmd == "/tasks":
		tasks := c.taskMgr.List()
		fmt.Fprintf(w, "\n%d tasks:\n", len(tasks))
		for _, t := range tasks {
			fmt.Fprintf(w, "  [%s] %s %s\n", t.Status, t.Type, t.Description)
		}

	// ---- Channels ----
	case strings.HasPrefix(cmd, "/weixin"):
		if c.weixinCh == nil {
			fmt.Fprintln(w, "微信通道未启用 (设置 channels.weixin.enabled: true)")
		} else {
			handleWeixinCommand(cmd, c.weixinCh, w)
		}
	case cmd == "/gateway":
		if c.gw == nil {
			fmt.Fprintln(w, "Gateway is not enabled (set gateway.enabled: true)")
			break
		}
		statuses := c.gw.Statuses()
		fmt.Fprintln(w, "\n--- Gateway Platforms ---")
		for name, st := range statuses {
			fmt.Fprintf(w, "  %-10s connected=%v\n", name, st.Connected)
		}

	case cmd == "/logs" || strings.HasPrefix(cmd, "/logs "):
		args := strings.Fields(cmd)
		n := 20
		if len(args) > 1 {
			if args[1] == "-f" {
				fmt.Fprintln(w, "Follow mode not yet supported in TUI. Use /logs [N] instead.")
				break
			}
			if v, err := strconv.Atoi(args[1]); err == nil && v > 0 {
				n = v
			}
		}
		if c.logBuf == nil {
			fmt.Fprintln(w, "Log buffer not available.")
			break
		}
		lines := c.logBuf.Last(n)
		fmt.Fprintf(w, "\n--- Recent %d logs ---\n", len(lines))
		for _, line := range lines {
			fmt.Fprintf(w, "[%s] %s\n", line.Level, line.Content)
		}

	// ---- Diagnostics ----
	case cmd == "/doctor":
		cmdDoctor(c, w)
	case cmd == "/debug":
		cmdDebug(w)
	case cmd == "/dump":
		cmdDump(c, w)
	case cmd == "/backup":
		cmdBackup(w)

	// ---- Scheduling ----
	case cmd == "/autonomy":
		if c.scheduler != nil {
			stats := c.scheduler.Stats()
			fmt.Fprintln(w, "\n--- Autonomous Scheduler ---")
			for k, v := range stats {
				fmt.Fprintf(w, "%s: %v\n", k, v)
			}
		} else {
			fmt.Fprintln(w, "Autonomous mode is not active (set agent.autonomy: semi or full in config)")
		}

	// ---- Exit ----
	case cmd == "/exit":
		if c.scheduler != nil {
			c.scheduler.Stop()
		}
		if c.weixinCh != nil {
			c.weixinCh.Stop()
		}
		os.Exit(0)

	default:
		fmt.Fprintf(w, "Unknown command: %s (type /help)\n", cmd)
	}
}

func listTools(c *cmdCtx, verbose bool, w io.Writer) {
	tools := tool.GlobalRegistry.AllTools()
	toolsets := tool.GlobalRegistry.Toolsets()
	fmt.Fprintf(w, "\n--- Tools (%d, %d toolsets) ---\n", len(tools), len(toolsets))

	if verbose {
		for _, t := range tools {
			fmt.Fprintf(w, "  %-18s [%s] %s\n", t.Name(), t.Toolset(), t.Description())
		}
	} else {
		prev := ""
		for _, t := range tools {
			if t.Toolset() != prev {
				fmt.Fprintf(w, "\n  [%s]\n", t.Toolset())
				prev = t.Toolset()
			}
			fmt.Fprintf(w, "    %s\n", t.Name())
		}
	}
}

func cmdCronList(c *cmdCtx, w io.Writer) {
	if c.cronSched == nil {
		fmt.Fprintln(w, "Cron scheduler is not active (add cron.jobs in config)")
		return
	}
	jobs := c.cronSched.Jobs()
	fmt.Fprintf(w, "\n--- Cron Jobs (%d) ---\n", len(jobs))
	for _, j := range jobs {
		status := "enabled"
		if !j.Enabled {
			status = "paused"
		}
		fmt.Fprintf(w, "  %s [%s]\n", j.Name, status)
		fmt.Fprintf(w, "    schedule: %s\n", j.Schedule)
		fmt.Fprintf(w, "    next_run: %s\n", j.NextRun.Format("15:04:05"))
		fmt.Fprintf(w, "    run_count: %d\n", j.RunCount)
		if j.Script != "" {
			fmt.Fprintf(w, "    script: %s\n", j.Script)
		}
	}
}

func cmdCronSubcommand(c *cmdCtx, sub string, nameArgs []string, w io.Writer) {
	if c.cronSched == nil {
		fmt.Fprintln(w, "Cron scheduler is not active")
		return
	}
	if len(nameArgs) == 0 {
		fmt.Fprintf(w, "Usage: /cron %s <name>\n", sub)
		return
	}
	name := nameArgs[0]
	switch sub {
	case "run":
		result, err := c.cronSched.RunNow(stdctx.Background(), name)
		if err != nil {
			fmt.Fprintf(w, "Error: %v\n", err)
			break
		}
		fmt.Fprintf(w, "Job %q executed:\n%s\n", name, result)
	case "pause":
		if err := c.cronSched.PauseJob(name); err != nil {
			fmt.Fprintf(w, "Error: %v\n", err)
		} else {
			fmt.Fprintf(w, "Job %q paused.\n", name)
		}
	case "resume":
		if err := c.cronSched.ResumeJob(name); err != nil {
			fmt.Fprintf(w, "Error: %v\n", err)
		} else {
			fmt.Fprintf(w, "Job %q resumed.\n", name)
		}
	default:
		fmt.Fprintf(w, "Unknown cron subcommand: %s (run|pause|resume)\n", sub)
	}
}

func cmdDoctor(c *cmdCtx, w io.Writer) {
	fmt.Fprintln(w, "\n--- gclaw Doctor ---")
	ok := true

	// Check config
	userPath, _ := config.UserConfigPath()
	if _, err := os.Stat(userPath); err == nil {
		fmt.Fprintf(w, "  Config file:       OK (%s)\n", userPath)
	} else {
		fmt.Fprintln(w, "  Config file:       MISSING (using defaults)")
	}

	// Check model connectivity
	models := c.providerFactory.Names()
	fmt.Fprintf(w, "  Registered models: %d (%v)\n", len(models), models)
	if m, err := c.providerFactory.Build(c.cfg.Model.Default); err != nil {
		fmt.Fprintf(w, "  Default model:     FAIL (%v)\n", err)
		ok = false
	} else {
		fmt.Fprintf(w, "  Default model:     OK (%s, max_tokens=%d)\n", m.ID(), m.MaxTokens())
	}

	// Check memory dir
	memDir := config.ExpandPath("~/.gclaw")
	if fi, err := os.Stat(memDir); err == nil && fi.IsDir() {
		fmt.Fprintf(w, "  Data directory:    OK (%s)\n", memDir)
	} else {
		fmt.Fprintln(w, "  Data directory:    MISSING")
		ok = false
	}

	// Check disk space
	if home, err := os.UserHomeDir(); err == nil {
		if usage, err := getDiskUsage(home); err == nil {
			fmt.Fprintf(w, "  Disk usage:        %s\n", usage)
		}
	}

	if ok {
		fmt.Fprintln(w, "\n  All checks passed.")
	} else {
		fmt.Fprintln(w, "\n  Some checks failed.")
	}
}

func cmdDebug(w io.Writer) {
	current := slog.Default().Enabled(stdctx.Background(), slog.LevelDebug)
	if current {
		slog.SetLogLoggerLevel(slog.LevelInfo)
		fmt.Fprintln(w, "Debug logging: OFF (level=info)")
	} else {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		fmt.Fprintln(w, "Debug logging: ON (level=debug)")
	}
}

func cmdDump(c *cmdCtx, w io.Writer) {
	ts := time.Now().Format("20060102-150405")
	path := filepath.Join(os.TempDir(), fmt.Sprintf("gclaw-dump-%s.json", ts))

	data := map[string]any{
		"version":     Version,
		"model":       c.cfg.Model.Default,
		"fallback":    c.cfg.Model.Fallback,
		"autonomy":    c.cfg.Agent.Autonomy,
		"permission":  c.cfg.Permission.Mode,
		"token_usage": c.ag.Usage(),
		"context":     c.ctxMgr.UsageStats(),
	}

	// Write simple JSON
	var buf strings.Builder
	buf.WriteString("{\n")
	for k, v := range data {
		buf.WriteString(fmt.Sprintf("  %q: %q,\n", k, fmt.Sprintf("%v", v)))
	}
	buf.WriteString("}\n")

	if err := os.WriteFile(path, []byte(buf.String()), 0644); err != nil {
		fmt.Fprintf(w, "Error writing dump: %v\n", err)
		return
	}
	fmt.Fprintf(w, "State dump saved to: %s\n", path)
}

func cmdBackup(w io.Writer) {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(w, "Error: %v\n", err)
		return
	}
	gclawDir := filepath.Join(home, ".gclaw")
	backupDir := filepath.Join(gclawDir, "backups")
	os.MkdirAll(backupDir, 0755)

	ts := time.Now().Format("20060102-150405")
	archive := filepath.Join(backupDir, fmt.Sprintf("gclaw-%s.tar.gz", ts))

	// Use tar command (available on Linux/macOS, Git Bash on Windows)
	cmd := exec.Command("tar", "czf", archive,
		"--exclude="+filepath.Join(gclawDir, "checkpoints"),
		"--exclude="+backupDir,
		"-C", home, ".gclaw")
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(w, "Backup failed: %v\n%s\n", err, string(output))
		return
	}
	fmt.Fprintf(w, "Backup saved to: %s\n", archive)
}

func boolStr(cond bool, trueVal, falseVal string) string {
	if cond {
		return trueVal
	}
	return falseVal
}

func countStr(count int, label string) string {
	return fmt.Sprintf("%d %s", count, label)
}

func weixinStatusStr(ch *weixin.Channel) string {
	if ch == nil {
		return "disabled"
	}
	s := ch.Status()
	if s.Connected {
		return "connected"
	}
	return "not connected"
}

func getDiskUsage(path string) (string, error) {
	if runtime.GOOS == "windows" {
		return "N/A (windows)", nil
	}
	out, err := exec.Command("df", "-h", path).Output()
	if err != nil {
		return "N/A", nil
	}
		lines := strings.Split(string(out), "\n")
	if len(lines) >= 2 {
		fields := strings.Fields(lines[1])
		if len(fields) >= 2 {
			return fields[1], nil
		}
	}
	return "N/A", nil
}


func handleWeixinCommand(cmd string, ch *weixin.Channel, w io.Writer) {
	args := strings.Fields(cmd)
	if len(args) < 2 {
		fmt.Fprintln(w, "Usage: /weixin login|logout|status")
		return
	}
	switch args[1] {
	case "login":
		fmt.Fprintln(w, "正在启动微信扫码登录...")
		if err := ch.Login(); err != nil {
			fmt.Fprintf(w, "登录失败: %v\n", err)
		}
	case "logout":
		fmt.Fprintln(w, "正在解绑微信账号...")
		if err := ch.Logout(); err != nil {
			fmt.Fprintf(w, "解绑失败: %v\n", err)
		}
	case "status":
		s := ch.Status()
		fmt.Fprintln(w, "\n--- 微信通道 ---")
		if s.Connected {
			fmt.Fprintln(w, "状态: 已连接")
			fmt.Fprintf(w, "账号: %s\n", s.AccountID)
			fmt.Fprintf(w, "用户: %s\n", s.UserID)
			if !s.LastMsgAt.IsZero() {
				fmt.Fprintf(w, "最后消息: %s\n", s.LastMsgAt.Format("15:04:05"))
			}
			fmt.Fprintf(w, "消息数: %d\n", s.MsgCount)
		} else {
			fmt.Fprintln(w, "状态: 未连接")
			if s.AccountID != "" {
				fmt.Fprintf(w, "账号: %s\n", s.AccountID)
			}
		}
	default:
		fmt.Fprintln(w, "Usage: /weixin login|logout|status")
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
	case "deepseek", "zhipu", "qianfan", "moonshot", "ark", "doubao", "kimi":
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

# Tool-use rules

NEVER answer these from memory — ALWAYS use a tool:
- Current facts: weather, news, stock prices, exchange rates, latest versions → web_search or web_extract
- Arithmetic, math, calculations → Bash
- Current time, date → Bash (date)
- File contents, sizes, line counts → ReadFile, Glob, Grep
- System state: OS, CPU, memory, disk, processes → Bash
- Git history, branches, diffs → Bash (git)

When you say you will perform an action (e.g. "let me check", "I will search"), you MUST immediately make the corresponding tool call in the same response. Never end your turn with a promise of future action — execute it now.

If a tool returns empty or partial results, retry with a different query before giving up. Keep working until the task is complete.

# General rules
- For pure general knowledge, summaries, translations, or explanations where no tools are needed, answer directly.
- If a tool fails twice in a row, STOP and tell the user what went wrong.
- Be concise and direct. Respond in the user's language.
- Do not list your capabilities unless asked.`
}
