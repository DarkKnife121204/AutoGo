package logging

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

const logDir = "logs"

func Setup() (func(), error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}

	filename := filepath.Join(
		logDir,
		"autogo-"+time.Now().Format("2006-01-02")+".log",
	)

	file, err := os.OpenFile(
		filename,
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0o644,
	)
	if err != nil {
		return nil, err
	}

	log.SetOutput(io.MultiWriter(os.Stdout, file))

	return func() { _ = file.Close() }, nil
}
