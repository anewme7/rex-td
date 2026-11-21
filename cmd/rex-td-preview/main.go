// rextd runs the Go sidebar in a terminal or a Rex pane.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"rex-td/rex"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "agent-details" {
		if err := runAgentDetails(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "palette" {
		runPalette(os.Args[2:])
		return
	}
	demo := flag.Bool("demo", false, "show sample tasks and metrics")
	defaultLanguage := english
	if os.Getenv("REX_TD_LANG") == string(chinese) {
		defaultLanguage = chinese
	}
	selectedLanguage := flag.String("lang", string(defaultLanguage), "interface language (en or zh)")
	showVersion := flag.Bool("version", false, "print the build version")
	cpu := flag.Bool("cpu-snapshot", false, "print native CPU counters as JSON")
	claude := flag.Bool("claude-statusline", false, "read Claude Code status-line JSON from stdin")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [--lang en|zh] [--demo | --version | --cpu-snapshot | --claude-statusline]\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	uiLanguage := language(*selectedLanguage)
	if uiLanguage != english && uiLanguage != chinese {
		fmt.Fprintln(os.Stderr, defaultLanguage.text("无效的界面语言，请使用 --lang en 或 --lang zh。"))
		os.Exit(2)
	}

	var err error
	switch {
	case *showVersion:
		fmt.Println("rextd", version)
	case *cpu:
		err = printCPUSnapshot(os.Stdout)
	case *claude:
		err = recordClaudeStatus(os.Stdin)
	default:
		err = run(*demo, uiLanguage)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, uiLanguage.text("无法启动 rextd")+":", err)
		os.Exit(1)
	}
}

func run(demo bool, uiLanguage language) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := previewLogger()
	logger.Info("rex-td started", "demo", demo)
	defer logger.Info("rex-td stopped")

	m := newModel(demo)
	m.language, m.focusPreview, m.ctx = uiLanguage, true, ctx
	if !demo {
		path, err := taskDBPath()
		if err != nil {
			return err
		}
		m.store, err = openTaskStore(path)
		if err != nil {
			return err
		}
		defer m.store.db.Close()
		m.paletteStoreVersion, _ = m.store.changeVersion()
		m.tasks, err = m.store.load()
		if err != nil {
			return err
		}
		m.selectVisibleTask()
		m.paletteResume, _ = m.store.readPaletteResume()
		m.applyPaletteResume()
		m.candidateIgnored, err = m.store.loadCandidateIgnores()
		if err != nil {
			return err
		}
		m.seenResults, err = m.store.loadSeenResults()
		if err != nil {
			return err
		}
		m.candidateWatches, err = m.store.loadCandidateWatches()
		if err != nil {
			return err
		}
		defer func() {
			cancel()
			m.metricWorkers.Wait()
		}()
	}
	c := rex.Current()
	if !demo && c.Inside() {
		var done <-chan struct{}
		m.ctx, m.rexContext = ctx, c
		m.updates, done = c.WatchFleet(ctx, logger)
		defer func() {
			cancel()
			<-done
		}()
	}
	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithFPS(24)).Run()
	return err
}
