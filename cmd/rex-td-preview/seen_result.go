package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"time"

	"rex-td/rex"
)

type seenResult struct {
	fingerprint string
	reportedAt  int64
}

func resultFingerprint(p rex.Pane) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{p.App, p.ReportID, p.AgentTitle, p.Message}, "\x00")))
	return fmt.Sprintf("%x", digest)
}

func (result seenResult) matches(p rex.Pane) bool {
	return p.Status == "done" && result.fingerprint != "" &&
		(p.ActivityAt.IsZero() || p.ActivityAt.UnixNano() <= result.reportedAt) &&
		result.fingerprint == resultFingerprint(p)
}

func (s *taskStore) loadSeenResults() (map[paneIdentity]seenResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, "SELECT host_id,session_id,block_id,fingerprint,reported_at FROM seen_pane_result")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make(map[paneIdentity]seenResult)
	for rows.Next() {
		var id paneIdentity
		var result seenResult
		if err := rows.Scan(&id.host, &id.session, &id.block, &result.fingerprint, &result.reportedAt); err != nil {
			return nil, err
		}
		results[id] = result
	}
	return results, rows.Err()
}

func (m *model) sceneResult() (rex.Pane, bool) {
	if m.store == nil || m.scene == nil {
		return rex.Pane{}, false
	}
	index := slices.IndexFunc(m.feed.Panes, func(live rex.Pane) bool {
		return samePane(live, m.scene.pane)
	})
	if index < 0 {
		return rex.Pane{}, false
	}
	pane := m.feed.Panes[index]
	state := m.feed.HostState(pane.Host.ID)
	return pane, pane.Status == "done" && (state == rex.Live || state == rex.Partial)
}

func (m *model) toggleSeenResult() {
	pane, ok := m.sceneResult()
	if !ok {
		return
	}
	id := identityOf(pane)
	old := m.seenResults[id]
	if old.matches(pane) {
		if !m.saved(m.store.exec(seenResultDelete, id.host, id.session, id.block, old.fingerprint, old.reportedAt)) {
			return
		}
		delete(m.seenResults, id)
	} else {
		// Identical repeated reports have new timestamps. Only activity that
		// started after this completion or different result data clears it.
		stamp := pane.ReportedAt
		if pane.ActivityAt.After(stamp) {
			stamp = pane.ActivityAt
		}
		result := seenResult{fingerprint: resultFingerprint(pane)}
		if !stamp.IsZero() {
			result.reportedAt = stamp.UnixNano()
		}
		if !m.saved(m.store.exec(`INSERT INTO seen_pane_result VALUES(?,?,?,?,?)
ON CONFLICT(host_id,session_id,block_id) DO UPDATE SET fingerprint=excluded.fingerprint,reported_at=excluded.reported_at`,
			id.host, id.session, id.block, result.fingerprint, result.reportedAt)) {
			return
		}
		m.seenResults[id] = result
	}
	m.notice = ""
	m.refreshPanes()
}

const seenResultDelete = `DELETE FROM seen_pane_result
WHERE host_id=? AND session_id=? AND block_id=? AND fingerprint=? AND reported_at=?`

func (m *model) pruneSeenResults() {
	if len(m.seenResults) == 0 || m.store == nil {
		return
	}
	// A partial or offline snapshot can omit a live pane. Only prune missing
	// panes once Rex has returned a complete snapshot for their host.
	missing := make(map[paneIdentity]seenResult)
	for id, result := range m.seenResults {
		if m.feed.HostState(id.host) == rex.Live {
			missing[id] = result
		}
	}
	for _, p := range m.feed.Panes {
		id := identityOf(p)
		delete(missing, id)
		result, ok := m.seenResults[id]
		state := m.feed.HostState(id.host)
		if ok && (state == rex.Live || (state == rex.Partial && p.Status != "")) && !result.matches(p) {
			missing[id] = result
		}
	}
	if len(missing) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()
	tx, err := m.store.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	for id, result := range missing {
		if _, err := tx.ExecContext(ctx, seenResultDelete, id.host, id.session, id.block, result.fingerprint, result.reportedAt); err != nil {
			return
		}
	}
	if tx.Commit() == nil {
		for id := range missing {
			delete(m.seenResults, id)
		}
	}
}
