package main

import (
	"encoding/json"
	"io"
)

// Counters include busy and idle ticks from the same operating system sample.
type cpuTicks struct{ Busy, Idle uint64 }

func cpuPercent(previous, current cpuTicks) *int {
	if current.Busy < previous.Busy || current.Idle < previous.Idle {
		return nil
	}
	busy, idle := current.Busy-previous.Busy, current.Idle-previous.Idle
	if busy+idle == 0 {
		return nil
	}
	percent := int(100*float64(busy)/float64(busy+idle) + 0.5)
	return &percent
}

func printCPUSnapshot(out io.Writer) error {
	snapshot, err := nativeCPUTicks()
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(snapshot)
}
