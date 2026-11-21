package main

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestResourcePanelStopsSamplingAndDiscardsOldReadings(t *testing.T) {
	t.Setenv("REX_TD_DATA_DIR", t.TempDir())
	m := newModel(false)
	m.focusPreview = true
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	defer func() {
		cancel()
		m.metricWorkers.Wait()
	}()
	// Keep the unit test away from the user's Codex account.
	m.codexRefreshAfter = time.Now().Add(time.Hour)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 28})
	boardKeys(m, "f")
	first := m.metrics
	if first == nil {
		t.Fatal("opening resources did not start sampling")
	}

	boardKeys(m, tea.KeyEsc)
	if m.metrics != nil || first.ctx.Err() != context.Canceled {
		t.Fatal("hiding resources left sampling active")
	}

	boardKeys(m, "f")
	if m.metrics == nil || m.metrics == first {
		t.Fatal("opening resources again did not start a fresh stream")
	}
	value := 37
	m.Update(metricEvent{feed: first, value: cpuUpdate{host: "local", value: &value}})
	if m.hostCPU["local"] != nil {
		t.Fatal("an update from the closed panel was shown after reopening")
	}
}
