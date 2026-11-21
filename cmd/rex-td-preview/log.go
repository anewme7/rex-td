package main

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
)

const logLimit = 2 * 1024 * 1024

func previewLogger() *slog.Logger {
	var out io.Writer = io.Discard
	if data := os.Getenv("REX_TD_DATA_DIR"); filepath.IsAbs(data) {
		dir := filepath.Join(data, "logs")
		if os.MkdirAll(dir, 0700) == nil {
			out = &rotatingLog{path: filepath.Join(dir, "activity.log"), limit: logLimit}
		}
	} else if cache, err := os.UserCacheDir(); err == nil {
		dir := filepath.Join(cache, "rex-td", "preview")
		if os.MkdirAll(dir, 0700) == nil {
			out = &rotatingLog{path: filepath.Join(dir, "activity.log"), limit: logLimit}
		}
	}
	return slog.New(slog.NewTextHandler(out, nil))
}

type rotatingLog struct {
	path  string
	limit int64
}

func (w *rotatingLog) Write(p []byte) (int, error) {
	size := int64(len(p))
	if size > w.limit {
		return 0, errors.New("log record exceeds file limit")
	}
	// Lock a separate inode so another preview cannot write during a rotation.
	lock, err := os.OpenFile(w.path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return 0, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return 0, err
	}

	info, err := os.Stat(w.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	if info != nil && info.Size()+size > w.limit {
		for _, paths := range [][2]string{{w.path + ".1", w.path + ".2"}, {w.path, w.path + ".1"}} {
			if err := os.Rename(paths[0], paths[1]); err != nil && !errors.Is(err, os.ErrNotExist) {
				return 0, err
			}
		}
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return f.Write(p)
}
