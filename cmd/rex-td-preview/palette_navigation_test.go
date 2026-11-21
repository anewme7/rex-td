package main

import (
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"rex-td/rex"
)

func TestPaletteKeepsTheSelectedTaskAndPaneOnReturn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	board := openTaskBoard(t, path)
	boardKeys(board, "+", "First task", tea.KeyEnter, tea.KeyEsc)
	boardKeys(board, "+", "Release review", tea.KeyEnter)
	panes := []rex.Pane{
		{Host: rex.Host{ID: "local", Label: "Local"}, SessionID: "session:local", ID: "block:local", Label: "Local work"},
		{Host: rex.Host{ID: "host:remote", Label: "Build"}, SessionID: "session:remote", ID: "block:remote", Label: "Remote review"},
	}
	other, err := openTaskStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.db.Close()
	for _, pane := range panes {
		if err := other.exec(insertPaneBinding, paneBindingValues(2, pane)...); err != nil {
			t.Fatal(err)
		}
	}
	board.Update(paletteStoreTick{})
	boardShows(t, board, "› Local work")
	if err := handlePaletteNavigation(other, json.NewEncoder(io.Discard), []string{"resume",
		`{"task_id":2,"pane":{"host_id":"host:remote","session_id":"session:remote","block_id":"block:remote"},"reason":"host_unavailable"}`}); err != nil {
		t.Fatal(err)
	}
	board.Update(paletteStoreTick{})
	boardShows(t, board, "› Remote review")
	boardShows(t, board, "无法连接目标主机")
}
