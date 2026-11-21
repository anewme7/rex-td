package rex

import (
	"context"
	"encoding/json"
)

const cpuAction = "rex_td_cpu_snapshot"

type CPUCounters struct{ Busy, Idle uint64 }

func (h Host) ExposesCPU(ctx context.Context) bool {
	raw, err := h.command(ctx, "actions", "--json").Output()
	if err != nil {
		return false
	}
	var actions []struct{ Name string }
	if json.Unmarshal(raw, &actions) != nil {
		var envelope struct{ Actions []struct{ Name string } }
		if json.Unmarshal(raw, &envelope) != nil {
			return false
		}
		actions = envelope.Actions
	}
	for _, action := range actions {
		if action.Name == cpuAction {
			return true
		}
	}
	return false
}

func (h Host) ReadCPU(ctx context.Context) (CPUCounters, error) {
	var ticks CPUCounters
	raw, err := h.command(ctx, "do", cpuAction, "--json").Output()
	if err != nil {
		return ticks, err
	}
	return ticks, json.Unmarshal(raw, &ticks)
}
