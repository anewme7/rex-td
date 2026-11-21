package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"rex-td/rex"
)

func TestSeenResultClearsOnlyTheReviewedCompletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	m := openTaskBoard(t, path)
	boardKeys(m, "+", "Release", tea.KeyEnter, tea.KeyEsc)
	host := rex.Host{ID: "local", Label: "Test Host"}
	pane := rex.Pane{Host: host, ID: "done", SessionID: "test", App: "codex", Status: "done",
		ReportID: "job", Message: "Built", ReportedAt: time.Now()}
	approval := rex.Pane{Host: host, ID: "approve", SessionID: "test", App: "codex", Status: "permission"}
	show := func(m *model, state rex.State, panes ...rex.Pane) {
		m.Update(rex.Fleet{Hosts: []rex.HostStatus{{Host: host, State: state}}, Panes: panes})
	}
	show(m, rex.Live, pane, approval)
	m.showPane(pane)
	boardShows(t, m, "标为已看")
	boardKeys(m, "y")
	boardShows(t, m, "撤销已看")
	if m.isAlert(pane) || !m.isAlert(approval) || m.tasks[0].state != todo {
		t.Fatal("marking the completion affected approval or task state")
	}

	// A partial feed and duplicate native report must preserve the mark.
	original := pane.ReportedAt
	show(m, rex.Partial, approval)
	pane.ReportedAt = original.Add(time.Second)
	show(m, rex.Live, pane, approval)
	results, err := m.store.loadSeenResults()
	if err != nil || len(results) != 1 {
		t.Fatalf("could not reload the mark: %v, %v", results, err)
	}
	reopened := openTaskBoard(t, path)
	reopened.seenResults, err = reopened.store.loadSeenResults()
	if err != nil {
		t.Fatal(err)
	}
	show(reopened, rex.Live, pane, approval)
	if reopened.isAlert(pane) {
		t.Fatal("a repeated completion reappeared after reopening")
	}

	// An intermediate working report can be coalesced into the next done snapshot.
	pane.ReportedAt = original.Add(3 * time.Second)
	pane.ActivityAt = original.Add(2 * time.Second)
	show(reopened, rex.Live, pane, approval)
	if !reopened.isAlert(pane) {
		t.Fatal("new activity did not restore the completion reminder")
	}
	reopened.showPane(pane)
	for row, line := range strings.Split(ansi.Strip(reopened.View()), "\n") {
		if strings.Contains(line, "标为已看") {
			reopened.Update(tea.MouseMsg{X: 2, Y: row, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			break
		}
	}
	if reopened.isAlert(pane) {
		t.Fatal("clicking Mark seen did not clear the reminder")
	}
	boardKeys(reopened, "y")
	if !reopened.isAlert(pane) {
		t.Fatal("undo did not restore the reminder")
	}
	reopened.showPane(approval)
	boardKeys(reopened, "y")
	if !reopened.isAlert(approval) {
		t.Fatal("the shortcut cleared an approval")
	}
}
