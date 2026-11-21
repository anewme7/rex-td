package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"rex-td/rex"
)

const codexQuotaInterval = 5 * time.Minute

type metricEvent struct {
	feed  *metricFeed
	value tea.Msg
}

type cpuUpdate struct {
	host  string
	value *int
}
type codexUpdate struct {
	reading      quotaReading
	failed       bool
	refreshAfter time.Time
}
type claudeUpdate struct{ reading quotaReading }

type metricFeed struct {
	ctx      context.Context
	cancel   context.CancelFunc
	messages chan tea.Msg
	workers  *sync.WaitGroup
	remote   map[string]bool
}

func startMetrics(parent context.Context, workers *sync.WaitGroup, lastClaude quotaReading, codexRefreshAfter time.Time) *metricFeed {
	ctx, cancel := context.WithCancel(parent)
	f := &metricFeed{ctx: ctx, cancel: cancel, messages: make(chan tea.Msg, 4), workers: workers, remote: make(map[string]bool)}
	f.run(func() { f.cpu("local", nativeCPUTicks) })
	f.run(func() {
		last := lastClaude
		for ctx.Err() == nil {
			next := claudeQuota()
			if next != last {
				f.publish(claudeUpdate{next})
				last = next
			}
			if !every(ctx, 5*time.Second) {
				return
			}
		}
	})
	f.run(func() {
		if delay := time.Until(codexRefreshAfter); delay > 0 && !every(ctx, delay) {
			return
		}
		for ctx.Err() == nil {
			reading, err := codexQuota(ctx)
			f.publish(codexUpdate{reading, err != nil, time.Now().Add(codexQuotaInterval)})
			if !every(ctx, codexQuotaInterval) {
				return
			}
		}
	})
	return f
}

func (m *model) syncMetrics() tea.Cmd {
	visible := m.focusPreview && m.ctx != nil && !m.demo && !m.editing &&
		m.page == listPage && m.focus != 0 && m.width >= 22 && m.height >= 10
	if !visible {
		if m.metrics != nil {
			m.metrics.cancel()
			m.metrics = nil
		}
		return nil
	}
	if m.metrics != nil {
		return nil
	}
	clear(m.hostCPU)
	m.metrics = startMetrics(m.ctx, &m.metricWorkers, m.claudeUsage, m.codexRefreshAfter)
	m.metrics.addHosts(m.feed.Hosts)
	return m.watchMetrics()
}

func (f *metricFeed) run(job func()) {
	f.workers.Add(1)
	go func() {
		defer f.workers.Done()
		job()
	}()
}

func (f *metricFeed) publish(m tea.Msg) {
	select {
	case f.messages <- m:
	case <-f.ctx.Done():
	}
}

func (f *metricFeed) cpu(host string, snapshot func() (cpuTicks, error)) {
	last, err := snapshot()
	var lastValue *int
	for every(f.ctx, 5*time.Second) {
		current, snapshotErr := snapshot()
		var value *int
		if err == nil && snapshotErr == nil {
			value = cpuPercent(last, current)
		}
		if (value == nil) != (lastValue == nil) || (value != nil && *value != *lastValue) {
			f.publish(cpuUpdate{host, value})
		}
		lastValue = value
		last, err = current, snapshotErr
	}
}

func (f *metricFeed) addHosts(hosts []rex.HostStatus) {
	for _, host := range hosts {
		if host.Server == "" || host.State != rex.Live || f.remote[host.ID] {
			continue
		}
		f.remote[host.ID] = true
		f.run(func() {
			if host.ExposesCPU(f.ctx) {
				f.cpu(host.ID, func() (cpuTicks, error) {
					c, err := host.ReadCPU(f.ctx)
					return cpuTicks(c), err
				})
			}
		})
	}
}

func every(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func quotaSpan(d int) string {
	switch {
	case d <= 0:
		return ""
	case d%1440 == 0:
		return fmt.Sprintf("%dd", d/1440)
	case d%60 == 0:
		return fmt.Sprintf("%dh", d/60)
	default:
		return fmt.Sprintf("%dm", d)
	}
}

func quotaValue(w quotaWindow) string {
	if !w.Present {
		return "—"
	}
	return fmt.Sprintf("%s %.0f%%", quotaSpan(w.Minutes), w.Used)
}

func resetIn(w quotaWindow, now time.Time) string {
	if !w.Present || w.Reset <= now.Unix() {
		return ""
	}
	left := time.Unix(w.Reset, 0).Sub(now)
	if left >= 24*time.Hour {
		return fmt.Sprintf("%dd", int(left/(24*time.Hour)))
	}
	if left >= time.Hour {
		return fmt.Sprintf("%dh%02d", int(left/time.Hour), int(left/time.Minute)%60)
	}
	return fmt.Sprintf("%dm", max(1, int(left/time.Minute)))
}
