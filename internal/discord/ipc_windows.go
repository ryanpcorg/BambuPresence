package discord

import (
	"fmt"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

func dialIPC() (net.Conn, error) {
	timeout := 2 * time.Second
	var lastErr error
	for i := 0; i < 10; i++ {
		pipePath := fmt.Sprintf(`\\.\pipe\discord-ipc-%d`, i)
		conn, err := winio.DialPipe(pipePath, &timeout)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("could not connect to Discord IPC pipe: %w", lastErr)
}
