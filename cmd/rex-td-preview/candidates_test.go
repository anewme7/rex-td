package main

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"rex-td/rex"
)

func candidateFixture() rex.Pane {
	return rex.Pane{
		ID: "block:one", SessionID: "session:one", WindowID: "window:one",
		App: "codex", Status: "working", AgentTitle: "Check export", Label: "Import work",
		Window: "Quarter close", Host: rex.Host{ID: "local", Label: "Local"},
	}
}

func TestCandidateUsesLiveMetadataAndExplicitDiscovery(t *testing.T) {
	p := candidateFixture()
	m := newModel(false)
	m.feed = rex.Fleet{Hosts: []rex.HostStatus{{Host: p.Host, State: rex.Live}}, Panes: []rex.Pane{p}}
	m.refreshCandidates()
	if len(m.candidates) != 0 {
		t.Fatal("suggested a pane without a discovery request")
	}
	m.scene, m.page = &paneScene{pane: p}, scenePage
	m.discoverCandidate()
	if len(m.candidates) != 1 || m.candidates[0].title != p.AgentTitle || len(m.tasks) != 0 {
		t.Fatal("discovery should suggest the reported title without creating a task")
	}
	m.candidateKey("enter")
	m.edit(tea.KeyMsg{Type: tea.KeyEsc})
	if m.editing || m.candidateDraft != nil || len(m.tasks) != 0 || len(m.candidates) != 1 {
		t.Fatal("cancelling review must leave the candidate available")
	}
	p.AgentTitle = ""
	if result, ok := m.identifyCandidate(p); !ok || result.title != p.Label {
		t.Fatal("missing agent title should fall back to a named pane")
	}
	p.Label = "shell"
	second := p
	second.ID = "block:two"
	second.App = "claude-code"
	m.feed.Panes = append(m.feed.Panes, second)
	if _, ok := m.identifyCandidate(p); ok {
		t.Fatal("a shared window title cannot name either agent's work")
	}
}

func TestCandidateAcceptancePersistsOneTaskAndLink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	s, err := openTaskStore(path)
	if err != nil {
		t.Fatal(err)
	}
	p := candidateFixture()
	if _, err := s.addCandidateTask(&task{title: "Review export"}, p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.addCandidateTask(&task{title: "Duplicate"}, p); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected already linked pane, got %v", err)
	}
	if err := s.ignoreCandidate(identityOf(p), "old topic"); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openTaskStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.db.Close()
	tasks, err := reopened.load()
	if err != nil || len(tasks) != 1 || tasks[0].title != "Review export" ||
		len(tasks[0].panes) != 1 || identityOf(tasks[0].panes[0]) != identityOf(p) {
		t.Fatalf("expected exactly one saved task and pane, got %v (%v)", tasks, err)
	}
	ignored, err := reopened.loadCandidateIgnores()
	if err != nil || ignored[identityOf(p)] != "old topic" {
		t.Fatalf("ignore was not restored: %v (%v)", ignored, err)
	}
}

func TestCandidateWatchSurvivesRestartAndIgnoresSameTopic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	store, err := openTaskStore(path)
	if err != nil {
		t.Fatal(err)
	}
	pane := candidateFixture()
	id := identityOf(pane)
	if err := store.setCandidateWatch(id, true); err != nil {
		t.Fatal(err)
	}
	if err := store.ignoreCandidate(id, "check export"); err != nil {
		t.Fatal(err)
	}
	if err := store.db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openTaskStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.db.Close()
	m := newModel(false)
	m.candidateWatches, err = store.loadCandidateWatches()
	if err != nil || !m.candidateWatches[id] {
		t.Fatalf("watch should persist: %v", err)
	}
	m.candidateIgnored, err = store.loadCandidateIgnores()
	if err != nil {
		t.Fatal(err)
	}
	feed := rex.Fleet{Hosts: []rex.HostStatus{{Host: pane.Host, State: rex.Live}}, Panes: []rex.Pane{pane}}
	m.Update(feed)
	if len(m.candidates) != 0 {
		t.Fatal("the same ignored topic returned")
	}
	feed.Panes[0].AgentTitle, feed.Panes[0].Status = "Verify invoice", "idle"
	m.Update(feed)
	if len(m.candidates) != 0 {
		t.Fatal("an idle watched pane yielded a candidate")
	}
	feed.Panes[0].Status = "question"
	m.Update(feed)
	if len(m.candidates) != 1 || m.candidates[0].title != "Verify invoice" || m.candidatesOpen {
		t.Fatal("an active new topic should appear in the collapsed Candidates group")
	}
	m.Update(feed)
	if len(m.candidates) != 1 {
		t.Fatal("repeated Rex reports duplicated a candidate")
	}
	feed.Hosts[0].State = rex.Offline
	m.Update(feed)
	if len(m.candidates) != 0 || !m.candidateWatches[id] {
		t.Fatal("an offline pane should disappear without losing its watch")
	}
}
