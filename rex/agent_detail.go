package rex

import (
	"context"
	"encoding/json"
	"errors"
)

func (p Pane) ForegroundPID(ctx context.Context) (int, error) {
	var process struct {
		Foreground struct{ PID int }
	}
	err := p.Host.call(ctx, "com.superlogical.terminal.process", map[string]string{
		"session_id": p.SessionID, "block_id": p.ID,
	}, &process)
	if err != nil {
		return 0, err
	}
	if process.Foreground.PID <= 0 {
		return 0, errors.New("Rex has no foreground process for this pane")
	}
	return process.Foreground.PID, nil
}

// RequestAgentDetail uses an optional, read-only action on a connected Rex host.
func (p Pane) RequestAgentDetail(ctx context.Context, threadID string, pid int) ([]byte, error) {
	args, err := json.Marshal(struct {
		Provider string `json:"provider"`
		ID       string `json:"thread_id,omitempty"`
		PID      int    `json:"pid,omitempty"`
	}{p.App, threadID, pid})
	if err != nil {
		return nil, err
	}
	return p.Host.command(ctx, "do", "-s", p.SessionID, "-b", p.ID, "rextd_agent_detail",
		"--args", string(args), "--json").Output()
}
