package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type agentDetail struct {
	SessionID string `json:"session_id"`
	Result    string `json:"result,omitempty"`
	CWD       string `json:"cwd,omitempty"`
	Model     string `json:"model,omitempty"`
	Effort    string `json:"effort,omitempty"`
	Waiting   string `json:"waiting,omitempty"`
	Updated   int64  `json:"updated,omitempty"`
}

type agentBinding struct {
	id  string
	pid int
}

var agentIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

func decodeCodexThread(raw json.RawMessage, id string) (agentDetail, error) {
	var reply struct {
		Thread struct {
			ID, CWD, Model, ReasoningEffort string
			UpdatedAt                       int64
		}
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return agentDetail{}, err
	}
	thread := reply.Thread
	if !agentIDPattern.MatchString(id) || thread.ID != id {
		return agentDetail{}, errors.New("Codex returned a different session")
	}
	return agentDetail{
		SessionID: id, CWD: compactAgentText(thread.CWD, 180), Model: compactAgentText(thread.Model, 60),
		Effort: compactAgentText(thread.ReasoningEffort, 25), Updated: thread.UpdatedAt,
	}, nil
}

type codexDetailItem struct {
	Type  string
	Phase string
	Text  string
}

func recentCodexResult(call codexCall, id string) string {
	raw, err := call("thread/turns/list", map[string]any{
		"threadId": id, "limit": 5, "sortDirection": "desc", "itemsView": "summary",
	})
	if err != nil {
		return ""
	}
	var turns struct {
		Data []struct {
			Status string
			Items  []codexDetailItem
		}
	}
	if json.Unmarshal(raw, &turns) != nil {
		return ""
	}
	for _, turn := range turns.Data {
		if turn.Status != "completed" {
			continue
		}
		for i := len(turn.Items) - 1; i >= 0; i-- {
			item := turn.Items[i]
			if item.Type == "agentMessage" && (item.Phase == "final_answer" || item.Phase == "") {
				if result := compactAgentText(item.Text, 420); result != "" {
					return result
				}
			}
		}
		return ""
	}
	return ""
}

func codexAgentDetail(ctx context.Context, id string) (agentDetail, error) {
	if !agentIDPattern.MatchString(id) {
		return agentDetail{}, errors.New("invalid Codex session ID")
	}
	var detail agentDetail
	err := withCodexRPC(ctx, []string{"app-server", "--listen", "stdio://"}, func(call codexCall) error {
		raw, err := call("thread/read", map[string]any{"threadId": id, "includeTurns": false})
		if err != nil {
			return err
		}
		detail, err = decodeCodexThread(raw, id)
		if err != nil {
			return err
		}
		detail.Result = recentCodexResult(call, id)
		return nil
	})
	return detail, err
}

func selectClaudeSession(raw json.RawMessage, pid int) (agentDetail, error) {
	var sessions []struct {
		PID             int
		SessionID       string `json:"sessionId"`
		CWD, WaitingFor string
		Status          string
	}
	if err := json.Unmarshal(raw, &sessions); err != nil {
		return agentDetail{}, err
	}
	var detail agentDetail
	for _, session := range sessions {
		if pid <= 0 || session.PID != pid || !agentIDPattern.MatchString(session.SessionID) {
			continue
		}
		if detail.SessionID != "" && detail.SessionID != session.SessionID {
			return agentDetail{}, errors.New("multiple Claude sessions match this process")
		}
		detail = agentDetail{SessionID: session.SessionID, CWD: session.CWD}
		if session.Status == "waiting" {
			detail.Waiting = compactAgentText(session.WaitingFor, 80)
		}
	}
	if detail.SessionID == "" {
		return agentDetail{}, errors.New("no matching Claude session for this pane")
	}
	return detail, nil
}

func claudeAgentDetail(ctx context.Context, pid int) (agentDetail, error) {
	if pid <= 0 {
		return agentDetail{}, errors.New("missing foreground process")
	}
	raw, err := exec.CommandContext(ctx, agentProgram("claude"), "agents", "--json", "--all").Output()
	if err != nil {
		return agentDetail{}, err
	}
	detail, err := selectClaudeSession(raw, pid)
	if err != nil {
		return agentDetail{}, err
	}
	if reader := claudeReaderPath(); reader != "" {
		ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		defer cancel()
		raw, err := exec.CommandContext(ctx, agentProgram("node"), reader, detail.SessionID, detail.CWD).Output()
		if err == nil {
			var last agentDetail
			if json.Unmarshal(raw, &last) == nil && last.SessionID == detail.SessionID {
				detail.Result, detail.Model, detail.Updated = compactAgentText(last.Result, 420), compactAgentText(last.Model, 60), last.Updated
			}
		}
	}
	detail.CWD = compactAgentText(detail.CWD, 180)
	return detail, nil
}

func agentProgram(name string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	dirs := []string{"/opt/homebrew/bin", "/usr/local/bin"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	for _, dir := range dirs {
		if path, err := exec.LookPath(filepath.Join(dir, name)); err == nil {
			return path
		}
	}
	return name
}

func claudeReaderPath() string {
	if reader := os.Getenv("REXTD_CLAUDE_READER"); filepath.IsAbs(reader) {
		return reader
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(base) {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	reader := filepath.Join(base, "rex", "rextd", "agent-details", "claude-reader", "read.mjs")
	if info, err := os.Stat(reader); err == nil && info.Mode().IsRegular() {
		return reader
	}
	return ""
}

func compactAgentText(input string, limit int) string {
	var out strings.Builder
	count, gap := 0, false
	for _, ch := range input {
		if ch <= ' ' || ch == 0x7f || (ch >= 0x80 && ch <= 0x9f) {
			gap = count > 0
			continue
		}
		if count >= limit || (gap && count+1 >= limit) {
			out.WriteRune('…')
			break
		}
		if gap {
			out.WriteByte(' ')
			count++
		}
		out.WriteRune(ch)
		count++
		gap = false
	}
	return out.String()
}

func runAgentDetails(args []string) error {
	if len(args) != 3 {
		return errors.New("expected provider, session ID and foreground PID")
	}
	pid, err := strconv.Atoi(args[2])
	if err != nil || pid <= 0 {
		return errors.New("invalid foreground PID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var detail agentDetail
	switch args[0] {
	case "codex":
		detail, err = codexAgentDetail(ctx, args[1])
	case "claude-code":
		detail, err = claudeAgentDetail(ctx, pid)
	default:
		err = fmt.Errorf("unsupported provider")
	}
	if err == nil {
		err = json.NewEncoder(os.Stdout).Encode(detail)
	}
	return err
}
