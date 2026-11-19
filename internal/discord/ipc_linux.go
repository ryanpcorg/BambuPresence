package discord

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

func dialIPC() (net.Conn, error) {
	dirs := []string{
		os.Getenv("XDG_RUNTIME_DIR"),
		os.Getenv("TMPDIR"),
		os.Getenv("TMP"),
		os.Getenv("TEMP"),
		"/tmp",
	}
	var lastErr error
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		for i := 0; i < 10; i++ {
			sock := filepath.Join(dir, fmt.Sprintf("discord-ipc-%d", i))
			conn, err := net.DialTimeout("unix", sock, 2*time.Second)
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
	}
	return nil, fmt.Errorf("could not connect to Discord IPC socket: %w", lastErr)
}
