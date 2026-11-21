package rex

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDecodeSnapshotKeepsPaneIdentityAndWaitStart(t *testing.T) {
	raw := []byte(`{"panes":{"block:b":{"label":"窗格","window":"窗口","records":[
		{"state":"working"},{"state":"blocked","kind":"permission","updated_at":"2026-11-18T02:00:00Z","app":"codex"}
	]},"block:a":{"records":[{"state":"idle","app":"claude-code"}]},"block:shell":{"records":{}}}}`)
	snapshot, ok := decodeSnapshot(raw, nil)
	if !ok || len(snapshot.Panes) != 2 || snapshot.Panes[0].ID != "block:b" ||
		snapshot.Panes[0].Status != "permission" || !snapshot.Panes[0].NeedsAttention() {
		t.Fatalf("bad snapshot: %+v", snapshot)
	}
	last := snapshot.Panes
	snapshot, ok = decodeSnapshot([]byte(`{"partial":true,"panes":{"block:b":{"records":[
		{"state":"blocked","kind":"permission","updated_at":"2026-11-18T02:01:00Z"}
	]}}}`), last)
	if !ok || snapshot.State != Partial || len(snapshot.Panes) != 1 || !snapshot.Panes[0].Since.Equal(last[0].Since) {
		t.Fatalf("bad updated snapshot: %+v", snapshot)
	}
}

func TestWatchCancelsAndReapsRexProcess(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	t.Setenv("REX_TEST_PID_FILE", pidFile)
	const script = `#!/bin/sh
cat >/dev/null
echo $$ > "$REX_TEST_PID_FILE"
echo '{"panes":{}}'
exec sleep 60
`
	if err := os.WriteFile(filepath.Join(dir, "rex"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	updates, done := (Context{"session:test", "block:test"}).watchHost(ctx, localHost, logger)
	select {
	case snapshot := <-updates:
		if snapshot.State != Live {
			t.Fatalf("got %s, want live", snapshot.State)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("no snapshot")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("watch failed to stop")
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
		t.Fatalf("Rex subprocess %d was not reaped: %v", pid, err)
	}
}

func TestFocusUsesTheSameNamedAppOnRemoteServer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REX_CLIENT", "")
	logfile := filepath.Join(dir, "calls")
	t.Setenv("REX_TEST_CALLS", logfile)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	const script = `#!/bin/sh
printf '%s\n' "$*" >> "$REX_TEST_CALLS"
case "$*" in
  *"session.view"*"session:home"*)
    printf '%s\n' '{"session_id":"session:home","windows":[{"window_id":"window:here","layers":[{"blocks":[{"block_id":"block:sidebar","owner":"client:here"}]}]}]}';;
  *"session.view"*"session:remote"*)
    printf '%s\n' '{"session_id":"session:remote","windows":[{"window_id":"window:new","layers":[{"blocks":[{"block_id":"block:target"}]}]}]}';;
  *"-S build"*"client.list"*)
    printf '%s\n' '{"clients":[{"client_id":"client:other","info":{"kind":"app","label":"Other"}},{"client_id":"client:matching","info":{"kind":"app","label":"Device"}}]}';;
  *"client.list"*)
    printf '%s\n' '{"clients":[{"client_id":"client:here","info":{"kind":"app","label":"Device"}}]}';;
  *"session.focus_block"*) printf '{}\n';;
  *"-C client:matching do session.select session_id=session:remote window_id=window:new"*) printf 'null\n';;
  *) exit 1;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "rex"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	target := Pane{Host: Host{"host:id", "build", "build"}, SessionID: "session:remote",
		WindowID: "window:old", ID: "block:target"}
	if err := (Context{"session:home", "block:sidebar"}).Focus(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(logfile)
	if err != nil {
		t.Fatal(err)
	}
	calls := string(raw)
	focus := strings.Index(calls, "session.focus_block")
	open := strings.Index(calls, "-S build -C client:matching do session.select session_id=session:remote window_id=window:new")
	if focus < 0 || open < focus || strings.Contains(calls, "-C client:other") {
		t.Fatalf("wrong pane or application selected:\n%s", calls)
	}
}
