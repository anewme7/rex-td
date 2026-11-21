package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	_ "modernc.org/sqlite"

	"rex-td/rex"
)

var storedStates = [...]string{"To do", "In progress", "Blocked", "Done", "Cancelled"}
var storedColors = [...]string{"", "blue", "green", "amber", "violet"}

type taskStore struct{ db *sql.DB }

func taskDBPath() (string, error) {
	dir := os.Getenv("REX_TD_DATA_DIR")
	if !filepath.IsAbs(dir) {
		base := os.Getenv("XDG_DATA_HOME")
		if !filepath.IsAbs(base) {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, ".local", "share")
		}
		dir = filepath.Join(base, "rex-td")
	}
	return filepath.Join(dir, "tasks.sqlite3"), nil
}

func openTaskStore(path string) (*taskStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		if err := file.Close(); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	location := url.URL{Scheme: "file", Path: path, RawQuery: "_pragma=foreign_keys%3DON&_pragma=busy_timeout%3D1200&_txlock=immediate"}
	db, err := sql.Open("sqlite", location.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &taskStore{db: db}
	if err := s.initialize(path); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *taskStore) initialize(path string) error {
	// Incremental auto-vacuum is selected before any table exists. An
	// existing database keeps its original settings and reusable free pages.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if info, err := os.Stat(path); err == nil && info.Size() == 0 {
		if _, err := s.db.ExecContext(ctx, "PRAGMA auto_vacuum=INCREMENTAL"); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, taskSchema); err != nil {
		return err
	}
	for _, column := range []string{"session_title", "agent_app"} {
		var exists bool
		err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pragma_table_info('rex_pane_binding') WHERE name=?)", column).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := tx.ExecContext(ctx, "ALTER TABLE rex_pane_binding ADD COLUMN "+column+" TEXT NOT NULL DEFAULT ''"); err != nil {
				return err
			}
		}
	}
	for _, column := range []struct{ name, definition string }{
		{"previous_state", "TEXT NOT NULL DEFAULT 'To do'"},
		{"archived", "INTEGER NOT NULL DEFAULT 0"},
		{"sort_order", "INTEGER NOT NULL DEFAULT 0"},
		{"parent_id", "INTEGER REFERENCES task(id)"},
	} {
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pragma_table_info('task') WHERE name=?)", column.name).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if _, err := tx.ExecContext(ctx, "ALTER TABLE task ADD COLUMN "+column.name+" "+column.definition); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

const taskSchema = `CREATE TABLE IF NOT EXISTS task(id INTEGER PRIMARY KEY, title TEXT NOT NULL, state TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '', review INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS task_pin(task INTEGER PRIMARY KEY REFERENCES task(id));
CREATE TABLE IF NOT EXISTS task_color(task INTEGER PRIMARY KEY REFERENCES task(id), color TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS task_kind(task INTEGER PRIMARY KEY REFERENCES task(id) ON DELETE CASCADE, kind INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS result(id INTEGER PRIMARY KEY, task INTEGER NOT NULL, body TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS rex_td_result_task ON result(task,id);
CREATE TABLE IF NOT EXISTS pane_link(task INTEGER, session_id TEXT, block_id TEXT, label TEXT,
    PRIMARY KEY(task,session_id,block_id));
CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS pane_agent_binding(
    host_id TEXT NOT NULL, session_id TEXT NOT NULL, block_id TEXT NOT NULL, app TEXT NOT NULL,
    agent_id TEXT NOT NULL, pid INTEGER NOT NULL,
    PRIMARY KEY(host_id,session_id,block_id,app));
CREATE TABLE IF NOT EXISTS candidate_ignore(
    host_id TEXT NOT NULL, session_id TEXT NOT NULL, block_id TEXT NOT NULL, topic TEXT NOT NULL,
    PRIMARY KEY(host_id, session_id, block_id));
CREATE TABLE IF NOT EXISTS candidate_watch(
    host_id TEXT NOT NULL, session_id TEXT NOT NULL, block_id TEXT NOT NULL,
    PRIMARY KEY(host_id, session_id, block_id));
CREATE TABLE IF NOT EXISTS rex_pane_binding(
    id INTEGER PRIMARY KEY, task INTEGER NOT NULL REFERENCES task(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL, host_label TEXT NOT NULL, server TEXT NOT NULL,
    session_id TEXT NOT NULL, window_id TEXT NOT NULL, block_id TEXT NOT NULL, title TEXT NOT NULL,
    UNIQUE(task,host_id,session_id,block_id));
INSERT OR IGNORE INTO rex_pane_binding(task,host_id,host_label,server,session_id,window_id,block_id,title)
SELECT p.task,'local','本机','',p.session_id,'',p.block_id,p.label
FROM pane_link AS p JOIN task AS t ON t.id=p.task
WHERE NOT EXISTS (SELECT 1 FROM meta WHERE key='go-pane-binding-v1')
ORDER BY p.rowid;
INSERT OR IGNORE INTO meta(key) VALUES ('go-pane-binding-v1');`

func (s *taskStore) exec(statement string, args ...any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := s.db.ExecContext(ctx, statement, args...)
	return err
}

func (s *taskStore) load() ([]*task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT t.id,t.title,t.note,t.state,t.previous_state,t.archived,t.sort_order,COALESCE(t.parent_id,0),
    (p.task IS NOT NULL) AS pinned, COALESCE(c.color,'') AS color, COALESCE(k.kind,0),
    COALESCE((SELECT body FROM result r WHERE r.task=t.id ORDER BY r.id DESC LIMIT 1),'') AS result,
    (SELECT count(*) FROM result r WHERE r.task=t.id) AS results,
    json(COALESCE((SELECT json_group_array(json_object(
        'host_id',b.host_id,'host_label',b.host_label,'server',b.server,'session_id',b.session_id,
        'window_id',b.window_id,'block_id',b.block_id,'title',b.title,
        'session_title',b.session_title,'agent_app',b.agent_app))
        FROM (SELECT * FROM rex_pane_binding WHERE task=t.id ORDER BY id) b),'[]')) AS bindings
FROM task t LEFT JOIN task_pin p ON p.task=t.id LEFT JOIN task_color c ON c.task=t.id
LEFT JOIN task_kind k ON k.task=t.id
ORDER BY (t.parent_id IS NULL AND pinned) DESC,COALESCE(NULLIF(t.sort_order,0),t.id),t.id;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []*task
	for rows.Next() {
		var title, note, state, previousState, color, result, links string
		t := &task{}
		if err := rows.Scan(&t.id, &title, &note, &state, &previousState, &t.archived, &t.sortOrder, &t.parentID, &t.pinned, &color, &t.kind, &result, &t.resultCount, &links); err != nil {
			return nil, err
		}
		t.title, t.note, t.result = storedDisplay(title), storedDisplay(note), storedDisplay(result)
		t.state = taskState(max(0, slices.Index(storedStates[:], state)))
		t.previousState = taskState(max(0, slices.Index(storedStates[:done], previousState)))
		t.color = max(0, slices.Index(storedColors[:], color))
		if t.kind < uncategorized || int(t.kind) >= len(kindLabels) {
			t.kind = uncategorized
		}
		var bindings []struct {
			HostID       string `json:"host_id"`
			HostLabel    string `json:"host_label"`
			Server       string
			SessionID    string `json:"session_id"`
			WindowID     string `json:"window_id"`
			BlockID      string `json:"block_id"`
			Title        string
			SessionTitle string `json:"session_title"`
			AgentApp     string `json:"agent_app"`
		}
		if err := json.Unmarshal([]byte(links), &bindings); err != nil {
			return nil, err
		}
		for _, b := range bindings {
			t.panes = append(t.panes, rex.Pane{
				ID: b.BlockID, SessionID: b.SessionID, WindowID: b.WindowID, Label: storedDisplay(b.Title),
				Session: storedDisplay(b.SessionTitle), App: storedDisplay(b.AgentApp),
				Host: rex.Host{ID: b.HostID, Label: storedDisplay(b.HostLabel), Server: b.Server},
			})
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func storedDisplay(value string) string {
	return strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, value)
}

func (s *taskStore) addTask(title string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var id int64
	err := s.db.QueryRowContext(ctx, "INSERT INTO task(title,state) VALUES (?,'To do') RETURNING id", title).Scan(&id)
	return id, err
}

func (s *taskStore) pin(id int64, value bool) error {
	if value {
		return s.exec("INSERT OR IGNORE INTO task_pin(task) VALUES(?)", id)
	}
	return s.exec("DELETE FROM task_pin WHERE task=?", id)
}

func (s *taskStore) color(id int64, color string) error {
	if color == "" {
		return s.exec("DELETE FROM task_color WHERE task=?", id)
	}
	return s.exec("INSERT INTO task_color(task,color) VALUES(?,?) ON CONFLICT(task) DO UPDATE SET color=excluded.color", id, color)
}

func (s *taskStore) state(id int64, value taskState) error {
	return s.exec("UPDATE task SET state=? WHERE id=?", storedStates[value], id)
}

func (s *taskStore) setKind(id int64, kind taskKind) error {
	if kind == uncategorized {
		return s.exec("DELETE FROM task_kind WHERE task=?", id)
	}
	return s.exec("INSERT INTO task_kind(task,kind) VALUES(?,?) ON CONFLICT(task) DO UPDATE SET kind=excluded.kind", id, kind)
}

func (s *taskStore) bind(id int64, pane rex.Pane, remove bool) error {
	if remove {
		if err := s.exec("DELETE FROM rex_pane_binding WHERE task=? AND host_id=? AND session_id=? AND block_id=?",
			id, pane.Host.ID, pane.SessionID, pane.ID); err != nil {
			return err
		}
		// A busy database can reuse its free pages until the next unlink.
		_ = s.exec("PRAGMA incremental_vacuum(8)")
		return nil
	}
	return s.exec(insertPaneBinding, paneBindingValues(id, pane)...)
}

const insertPaneBinding = `INSERT INTO rex_pane_binding(task,host_id,host_label,server,session_id,window_id,block_id,title,session_title,agent_app)
VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(task,host_id,session_id,block_id) DO NOTHING`

func paneBindingValues(id int64, pane rex.Pane) []any {
	return []any{id, pane.Host.ID, pane.Host.Label, pane.Host.Server, pane.SessionID, pane.WindowID, pane.ID, paneTitle(pane), pane.Session, pane.App}
}
