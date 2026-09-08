package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	a2tea "github.com/Ostrovsky42/agent-interaction-runtime/adapter/bubbletea"
	"github.com/Ostrovsky42/agent-interaction-runtime/e2e"
	"github.com/Ostrovsky42/agent-interaction-runtime/engine"
	"github.com/Ostrovsky42/agent-interaction-runtime/layout"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	scenario := flag.String("scenario", "form", "Scenario to run: form, stream, table, showcase, interactive, or all")
	runTUI := flag.Bool("tui", false, "Launch full interactive Bubble Tea TUI session")
	presetName := flag.String("preset", "dashboard", "Bubble Tea presentation preset: minimal, dashboard, or dense")
	serverAddr := flag.String("server", "", "Run MCP HTTP streaming server (e.g. ':8080') for agent connections")
	width := flag.Int("width", 80, "Terminal width")
	height := flag.Int("height", 24, "Terminal height")
	delayMs := flag.Int("delay", 150, "Delay in ms between streaming frames")
	useANSI := flag.Bool("ansi", true, "Use ANSI colors")
	clearScreen := flag.Bool("clear", true, "Clear screen / update in-place to prevent terminal jitter")
	flag.Parse()

	preset, presetErr := a2tea.ParsePreset(*presetName)
	if presetErr != nil {
		fmt.Fprintln(os.Stderr, presetErr)
		os.Exit(2)
	}

	if *serverAddr != "" {
		fmt.Printf(">> A2UI MCP HTTP Server listening on %s (Protocol: 2026-07-28)...\n", *serverAddr)
		runner := e2e.NewRunner("agent-session", protocol.DefaultLimits(), *width, *height, nil)
		runner.SetFrameHook(func(f *e2e.Frame, rev uint64) {
			if *clearScreen {
				fmt.Print("\033[H\033[2J")
			}
			fmt.Printf("=== [Publication Barrier Committed - Revision %d] ===\n", rev)
			if *useANSI {
				fmt.Print(f.ANSI())
			} else {
				fmt.Println(f.PlainText())
			}
		})
		if err := http.ListenAndServe(*serverAddr, runner); err != nil {
			fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *runTUI {
		var eng *engine.Engine
		if *scenario == "showcase" || *scenario == "interactive" {
			var err error
			if *scenario == "interactive" {
				eng, err = buildInteractiveEngine()
			} else {
				eng, err = buildShowcaseEngine()
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s setup failed: %v\n", *scenario, err)
				os.Exit(1)
			}
		} else {
			eng = engine.New(protocol.DefaultLimits(), 64, engine.NewNoopActions())
			_ = eng.Apply(protocol.Operation{
				V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "main", Type: protocol.NodeBox, Parent: "root",
				Props: json.RawMessage(`{"dir":"col","border":"rounded","padding":1,"gap":1,"style":{"fg":"primary"}}`),
			})
			_ = eng.Apply(protocol.Operation{
				V: 1, Seq: 2, Op: protocol.OpUpsert, ID: "title", Type: protocol.NodeText, Parent: "main",
				Props: json.RawMessage(`{"text":"A2UI Bubble Tea Interactive Terminal Adapter","style":{"fg":"primary","bold":true}}`),
			})
			_ = eng.Apply(protocol.Operation{
				V: 1, Seq: 3, Op: protocol.OpUpsert, ID: "desc", Type: protocol.NodeText, Parent: "main",
				Props: json.RawMessage(`{"text":"Type below and press Enter to submit. Use Tab to switch focus, or Ctrl+C to quit:","style":{"dim":true}}`),
			})
			_ = eng.Apply(protocol.Operation{
				V: 1, Seq: 4, Op: protocol.OpUpsert, ID: "user-input", Type: protocol.NodeInput, Parent: "main",
				Props: json.RawMessage(`{"placeholder":"Type prompt or operator command...","value":"","action":"submit_cmd"}`),
			})
			_ = eng.Apply(protocol.Operation{
				V: 1, Seq: 5, Op: protocol.OpUpsert, ID: "pbar", Type: protocol.NodeProgress, Parent: "main",
				Props: json.RawMessage(`{"value":0.85,"label":"Core Engine Active"}`),
			})
			_ = eng.Apply(protocol.Operation{
				V: 1, Seq: 6, Op: protocol.OpUpsert, ID: "actions", Type: protocol.NodeActions, Parent: "main",
				Props: json.RawMessage(`{"items":[{"key":"q","label":"Quit","action":"quit"}]}`),
			})
			_ = eng.Focus("user-input")
			_ = eng.Apply(protocol.Operation{V: 1, Seq: 7, Op: protocol.OpCommit, Frame: "tui-init"})
		}

		m := a2tea.NewModelWithPreset(eng, a2tea.DefaultTheme, preset, nil)
		// Enable alternate screen buffer to prevent scrolling jitter and jumping
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
			os.Exit(1)
		}
		return
	}

	renderFrame := func(title string, frame *e2e.Frame) {
		if *clearScreen {
			// Clear screen and move cursor to home position
			fmt.Print("\033[H\033[2J")
		} else {
			fmt.Println()
		}
		fmt.Printf("=== %s ===\n", title)
		if *useANSI {
			fmt.Print(frame.ANSI())
		} else {
			fmt.Println(frame.PlainText())
		}
	}

	runForm := func() {
		runner := e2e.NewRunner("cli-form-sess", protocol.DefaultLimits(), *width, *height, nil)
		agent := e2e.NewAgentSimulator("cli-form-sess", runner)

		_ = agent.Connect("commit-barrier", "incremental-props", "action-events")
		_ = agent.SetupFormScreen()
		renderFrame("Step 1: Agent builds interactive form", runner.LastFrame())

		time.Sleep(time.Duration(*delayMs) * time.Millisecond)
		_ = runner.TypeInput("username", "AdaLovelace")
		renderFrame("Step 2: Operator types identity", runner.LastFrame())

		time.Sleep(time.Duration(*delayMs) * time.Millisecond)
		_ = runner.SubmitInput("username")
		events, _ := agent.FetchEvents()
		for _, ev := range events {
			if ev.Ev == "submit" {
				_ = agent.CompleteFormSuccess(ev.Value)
				break
			}
		}
		renderFrame("Step 3: Agent verifies and presents authenticated dashboard", runner.LastFrame())
	}

	runStream := func() {
		runner := e2e.NewRunner("cli-stream-sess", protocol.DefaultLimits(), *width, *height, nil)
		agent := e2e.NewAgentSimulator("cli-stream-sess", runner)

		_ = agent.Connect()
		_ = agent.Upsert("box", "root", protocol.NodeBox, map[string]any{
			"dir":     string(layout.Column),
			"border":  string(layout.BorderRounded),
			"padding": 1,
			"gap":     1,
			"style": map[string]any{
				"fg": "primary",
			},
		}, nil)
		_ = agent.Upsert("title", "box", protocol.NodeText, map[string]any{
			"text": "Antigravity Assistant Inference Stream",
			"style": map[string]any{
				"fg":   "primary",
				"bold": true,
			},
		}, nil)
		_ = agent.Upsert("pbar", "box", protocol.NodeProgress, map[string]any{
			"value": 0.0,
			"label": "Connecting to model...",
		}, nil)
		_ = agent.Upsert("content", "box", protocol.NodeText, map[string]any{
			"text": "",
		}, nil)
		_ = agent.Commit("init")

		renderFrame("Stream Initialized", runner.LastFrame())

		tokens := []string{
			"A2UI", " protocol", " provides", " high-assurance", " UI", " state",
			" projection", " for", " autonomous", " AI", " agents.",
			"\n\n- Deterministic", " commit", " barriers",
			"\n- Strict", " MCP", " 2026-07-28", " transport",
			"\n- Bounded", " resources", " and", " backpressure",
		}

		for i, tok := range tokens {
			time.Sleep(time.Duration(*delayMs) * time.Millisecond)
			_ = agent.AppendText("content", tok)
			pct := float64(i+1) / float64(len(tokens))
			_ = agent.UpdateProps("pbar", map[string]any{
				"value": pct,
				"label": fmt.Sprintf("Token %d/%d", i+1, len(tokens)),
			})
			_ = agent.Commit(fmt.Sprintf("tok-%d", i+1))
			renderFrame(fmt.Sprintf("Stream Progress: Token %d/%d", i+1, len(tokens)), runner.LastFrame())
		}
	}

	runTable := func() {
		runner := e2e.NewRunner("cli-table-sess", protocol.DefaultLimits(), *width, *height, nil)
		agent := e2e.NewAgentSimulator("cli-table-sess", runner)

		_ = agent.Connect()
		_ = agent.Upsert("main", "root", protocol.NodeBox, map[string]any{
			"dir":     "col",
			"border":  "normal",
			"padding": 1,
			"gap":     1,
		}, nil)
		_ = agent.Upsert("header", "main", protocol.NodeText, map[string]any{
			"text": "Cluster Node Status Dashboard",
			"style": map[string]any{
				"bold": true,
				"fg":   "primary",
			},
		}, nil)
		_ = agent.Upsert("tbl", "main", protocol.NodeTable, map[string]any{
			"columns": []map[string]any{
				{"title": "Node ID", "width": 16},
				{"title": "Role", "width": 14},
				{"title": "Status", "width": 12},
				{"title": "Memory", "width": 10},
			},
			"rows": [][]string{
				{"node-us-east-1", "core-engine", "HEALTHY", "1.2 GB"},
				{"node-us-west-2", "mcp-gateway", "HEALTHY", "450 MB"},
				{"node-eu-west-1", "terminal-view", "DEGRADED", "890 MB"},
			},
		}, nil)
		_ = agent.Commit("dashboard-ready")
		renderFrame("Cluster Metrics Table", runner.LastFrame())
	}

	runShowcase := func() {
		eng, err := buildShowcaseEngine()
		if err != nil {
			fmt.Fprintf(os.Stderr, "showcase setup failed: %v\n", err)
			return
		}
		m := a2tea.NewModelWithPreset(eng, a2tea.DefaultTheme, preset, nil)
		m.Width = *width
		m.Height = *height
		if *clearScreen {
			fmt.Print("\033[H\033[2J")
		}
		fmt.Printf("=== A2UI Renderer V2 · preset=%s · %dx%d ===\n", preset, *width, *height)
		fmt.Print(m.View())
		fmt.Println()
	}

	runInteractive := func() {
		eng, err := buildInteractiveEngine()
		if err != nil {
			fmt.Fprintf(os.Stderr, "interactive setup failed: %v\n", err)
			return
		}
		m := a2tea.NewModelWithPreset(eng, a2tea.DefaultTheme, preset, nil)
		m.Width = *width
		m.Height = *height
		if *clearScreen {
			fmt.Print("\033[H\033[2J")
		}
		fmt.Printf("=== A2UI V3 Interactive Runtime · preset=%s · %dx%d ===\n", preset, *width, *height)
		fmt.Print(m.View())
		fmt.Println("\nRun with -tui to use keyboard navigation and editing.")
	}

	switch *scenario {
	case "form":
		runForm()
	case "stream":
		runStream()
	case "table":
		runTable()
	case "showcase":
		runShowcase()
	case "interactive":
		runInteractive()
	case "all":
		runForm()
		runStream()
		runTable()
		runInteractive()
	default:
		fmt.Fprintf(os.Stderr, "Unknown scenario %q. Choose form, stream, table, showcase, interactive, or all\n", *scenario)
		os.Exit(1)
	}
}
