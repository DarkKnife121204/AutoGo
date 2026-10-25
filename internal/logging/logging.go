package logging

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const logDir = "logs"
const logRetention = 30 * 24 * time.Hour

func cleanupOldLogs(now time.Time) {
	files, err := os.ReadDir(logDir)
	if err != nil {
		return
	}

	cutoff := now.Add(-logRetention)

	for _, file := range files {
		name := file.Name()

		if file.IsDir() || file.Type()&os.ModeSymlink != 0 ||
			len(name) != len("autogo-2006-01-02.log") ||
			name[:7] != "autogo-" || name[17:] != ".log" {
			continue
		}

		if _, err := time.Parse("2006-01-02", name[7:17]); err != nil {
			continue
		}

		info, err := file.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}

		if err := os.Remove(filepath.Join(logDir, name)); err != nil {
			_, _ = os.Stderr.WriteString(
				"[logging] ошибка удаления " + name + ": " + err.Error() + "\n",
			)
		}
	}
}

type dailyFileWriter struct {
	mu   sync.Mutex
	day  string
	file *os.File
}

func (w *dailyFileWriter) rotateLocked(now time.Time) error {
	day := now.Format("2006-01-02")
	if day == w.day && w.file != nil {
		return nil
	}

	filename := filepath.Join(logDir, "autogo-"+day+".log")
	file, err := os.OpenFile(
		filename,
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0o644,
	)
	if err != nil {
		return err
	}

	previous := w.file
	w.file = file
	w.day = day

	if previous != nil {
		_ = previous.Close()
	}

	cleanupOldLogs(now)
	return nil
}

func (w *dailyFileWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.rotateLocked(time.Now()); err != nil {
		if w.file == nil {
			return 0, err
		}
	}

	return w.file.Write(data)
}

func (w *dailyFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}

	err := w.file.Close()
	w.file = nil
	return err
}

func Setup() (func(), error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}

	writer := &dailyFileWriter{}
	if err := writer.rotateLocked(time.Now()); err != nil {
		return nil, err
	}

	log.SetOutput(io.MultiWriter(os.Stdout, writer))

	return func() {
		log.SetOutput(os.Stdout)
		_ = writer.Close()
	}, nil
}
