package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const traceEnv = "CLAUDE_SESSIONS_TRACE"

var sideTrace = struct {
	once    sync.Once
	mu      sync.Mutex
	path    string
	enabled bool
}{}

func traceEnabled() bool {
	sideTrace.once.Do(func() {
		sideTrace.path = os.Getenv(traceEnv)
		sideTrace.enabled = sideTrace.path != ""
	})
	return sideTrace.enabled
}

func traceEvent(event string, fields map[string]any) {
	if !traceEnabled() {
		return
	}

	payload := make(map[string]any, len(fields)+3)
	payload["ts"] = time.Now().Format(time.RFC3339Nano)
	payload["event"] = event
	payload["pid"] = os.Getpid()
	for k, v := range fields {
		payload[k] = v
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	data = append(data, '\n')

	sideTrace.mu.Lock()
	defer sideTrace.mu.Unlock()

	if dir := filepath.Dir(sideTrace.path); dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	f, err := os.OpenFile(sideTrace.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(data)
}

func traceDurationMS(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}
