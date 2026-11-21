package rex

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// PaneTurn contains short excerpts from one completed turn in the selected pane.
type PaneTurn struct{ Input, Result string }

const paneTailBytes = 8 << 10

func (p Pane) RecentCodexTurn(ctx context.Context) PaneTurn {
	if p.App != "codex" || p.Status != "done" || p.SessionID == "" || p.ID == "" {
		return PaneTurn{}
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	call := func(method, args string, target any) bool {
		raw, err := p.Host.command(ctx, "block", "call", "-s", p.SessionID, "-b", p.ID, method, args).Output()
		return err == nil && json.Unmarshal(raw, target) == nil
	}
	var capture struct {
		Content string
		ID      *int64
		Total   int
	}
	if !call("format", `{"format":"text","trim":true,"unwrap":true,"max_bytes":1024}`, &capture) {
		return PaneTurn{}
	}
	tail := capture.Content
	if capture.ID != nil {
		if capture.Total <= 0 {
			return PaneTurn{}
		}
		offset := max(0, capture.Total-paneTailBytes)
		args := fmt.Sprintf(`{"id":%d,"offset":%d,"max_bytes":%d}`, *capture.ID, offset, 3*paneTailBytes)
		var chunk struct {
			Data string
			EOF  bool
		}
		if !call("blob_read", args, &chunk) || !chunk.EOF {
			return PaneTurn{}
		}
		tail = chunk.Data
		if offset > 0 {
			_, tail, _ = strings.Cut(tail, "\n")
		}
	}
	return completedCodexTurn(tail)
}

var codexTurnEnd = regexp.MustCompile(`^Worked for [^•·]+[•·] [0-9]{1,2}:[0-9]{2}$`)

func completedCodexTurn(tail string) PaneTurn {
	lines := strings.Split(tail, "\n")
	end := len(lines) - 1
	for end >= 0 && !codexTurnEnd.MatchString(strings.TrimSpace(lines[end])) {
		end--
	}
	if end < 0 {
		return PaneTurn{}
	}
	// An assistant line after this footer belongs to a later turn. Avoid
	// presenting a previous result as the work the user just selected.
	prompts := 0
	for _, line := range lines[end+1:] {
		if strings.HasPrefix(line, "› ") {
			prompts++
		}
		if prompts > 1 || strings.HasPrefix(line, "• ") || strings.HasPrefix(strings.TrimSpace(line), "Working (") {
			return PaneTurn{}
		}
	}
	start, result := -1, -1
	for i := end - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "› ") {
			start = i
			break
		}
		if result == -1 && strings.HasPrefix(lines[i], "• ") {
			result = i
		}
	}
	if start < 0 {
		return PaneTurn{}
	}
	turn := PaneTurn{Input: turnExcerpt(strings.TrimPrefix(lines[start], "› "))}
	if result >= 0 {
		line := strings.TrimPrefix(lines[result], "• ")
		for _, prefix := range []string{"Ran ", "Explored", "Edited ", "Added ", "Deleted ", "Used ", "Called ", "Working "} {
			if strings.HasPrefix(line, prefix) {
				return turn
			}
		}
		turn.Result = turnExcerpt(line)
	}
	return turn
}

func turnExcerpt(line string) string {
	const limit = 220
	runes := []rune(cleaned(line))
	if len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}
	return string(runes)
}
