package main

import (
	"encoding/json"
	"path/filepath"

	"rex-td/rex"
	"testing"
)

func TestClaudeDetailKeepsSameNamedSessionsSeparate(t *testing.T) {
	raw := json.RawMessage(`[
		{"name":"Release QA","pid":200,"sessionId":"6b340000-1000-7000-b000-111111111111","cwd":"/work/a","status":"waiting","waitingFor":"permission prompt"},
		{"name":"Release QA","pid":300,"sessionId":"6b340000-1000-7000-b000-222222222222","cwd":"/work/b","status":"busy"}
	]`)
	first := "6b340000-1000-7000-b000-111111111111"
	second := "6b340000-1000-7000-b000-222222222222"
	for _, c := range []struct {
		pid  int
		want string
	}{
		{300, second},
		{200, first},
	} {
		got, err := selectClaudeSession(raw, c.pid)
		if err != nil || got.SessionID != c.want {
			t.Fatalf("PID %d selected the wrong session: %+v %v", c.pid, got, err)
		}
	}
	if _, err := selectClaudeSession(raw, 400); err == nil {
		t.Fatal("unmatched pane received another session's detail")
	}
}

func TestCodexDetailRejectsReplyForDifferentThread(t *testing.T) {
	raw := json.RawMessage(`{"thread":{"id":"other-session","name":"Release QA","model":"gpt-test","cwd":"/work/a"}}`)
	if _, err := decodeCodexThread(raw, "wanted-session"); err == nil {
		t.Fatal("same name must not override the session ID")
	}
}

func TestAgentBindingUsesHostAndPaneIdentity(t *testing.T) {
	store, err := openTaskStore(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.db.Close() })
	local := rex.Pane{Host: rex.Host{ID: "local"}, SessionID: "session:working", ID: "block:working", App: "codex", Label: "Release QA"}
	remote := local
	remote.Host.ID = "remote"
	one := agentBinding{id: "first-session-id", pid: 500}
	two := agentBinding{id: "second-session-id", pid: 500}
	if err := store.savePaneAgentBinding(local, one); err != nil {
		t.Fatal(err)
	}
	if err := store.savePaneAgentBinding(remote, two); err != nil {
		t.Fatal(err)
	}
	for pane, want := range map[rex.Pane]agentBinding{local: one, remote: two} {
		if got, err := store.paneAgentBinding(pane); err != nil || got != want {
			t.Fatalf("wrong session for host %s: %v %v", pane.Host.ID, got, err)
		}
	}
}
