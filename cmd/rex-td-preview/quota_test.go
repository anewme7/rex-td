package main

import (
	"strings"
	"testing"
)

func TestCodexQuotaKeepsProviderWindowDuration(t *testing.T) {
	q := decodeCodexQuota([]byte(`{"rateLimits":{"primary":{"usedPercent":91}},
		"rateLimitsByLimitId":{"codex":{"primary":{"usedPercent":0,"windowDurationMins":10080,"resetsAt":1999999999},"secondary":null}}}`))
	w := q.Windows[0]
	if !w.Present || w.Used != 0 || w.Minutes != 10080 || w.Reset != 1999999999 || q.Windows[1].Present {
		t.Fatalf("unexpected provider quota: %+v", q)
	}
}

func TestClaudeStatusKeepsAvailableWindows(t *testing.T) {
	q, err := decodeClaudeStatus(strings.NewReader(`{"session_id":"ignored","rate_limits":{"five_hour":{
		"used_percentage":20.5,"resets_at":1999999999}}}`))
	if err != nil || !q.Windows[0].Present || q.Windows[0].Used != 20.5 || q.Windows[0].Minutes != 300 || q.Windows[1].Present {
		t.Fatalf("unexpected Claude status: %+v, %v", q, err)
	}
	absent, err := decodeClaudeStatus(strings.NewReader(`{"model":{}}`))
	if err != nil || absent.available() {
		t.Fatalf("missing real metrics must not become zero: %+v, %v", absent, err)
	}
}
