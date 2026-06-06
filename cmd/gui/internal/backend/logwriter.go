//go:build windows

package backend

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	progressPattern = regexp.MustCompile(`\]\s+([0-9.]+)%`)
	speedPattern    = regexp.MustCompile(`(?:ema|avg)=\s*([0-9.]+)`)
	fpsPattern      = regexp.MustCompile(`(?:cap|capture_fps)=\s*([0-9.]+)`)
	rankPattern     = regexp.MustCompile(`rank=([0-9]+)/([0-9]+)`)
	etaPattern      = regexp.MustCompile(`eta=([0-9hms]+)`)
)

type eventLogWriter struct {
	app       *application.App
	eventName string
	sessionID string
	mu        sync.Mutex
	buffer    strings.Builder
}

func newEventLogWriter(app *application.App, eventName string, sessionID string) *eventLogWriter {
	return &eventLogWriter{app: app, eventName: eventName, sessionID: sessionID}
}

func (w *eventLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	text := strings.ReplaceAll(string(p), "\r", "\n")
	w.buffer.WriteString(text)
	for {
		content := w.buffer.String()
		idx := strings.IndexByte(content, '\n')
		if idx < 0 {
			if strings.Contains(content, "]") {
				w.emitLine(strings.TrimSpace(content))
				w.buffer.Reset()
			}
			break
		}
		line := strings.TrimSpace(content[:idx])
		w.buffer.Reset()
		w.buffer.WriteString(content[idx+1:])
		if line != "" {
			w.emitLine(line)
		}
	}
	return len(p), nil
}

func (w *eventLogWriter) emitLine(line string) {
	w.app.Event.Emit(w.eventName, map[string]any{
		"sessionId": w.sessionID,
		"message":   line,
		"at":        time.Now(),
	})
	if w.eventName == "receiver:log" {
		if metrics, ok := parseMetricsLine(w.sessionID, line); ok {
			w.app.Event.Emit("receiver:metrics", metrics)
		}
	}
}

func parseMetricsLine(sessionID string, line string) (ReceiverMetrics, bool) {
	if !strings.Contains(line, "%") {
		return ReceiverMetrics{}, false
	}
	metrics := ReceiverMetrics{SessionID: sessionID, State: StateRunning, UpdatedAt: time.Now()}
	if m := progressPattern.FindStringSubmatch(line); len(m) == 2 {
		metrics.Progress, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := speedPattern.FindStringSubmatch(line); len(m) == 2 {
		metrics.SpeedKBps, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := fpsPattern.FindStringSubmatch(line); len(m) == 2 {
		metrics.FPS, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := rankPattern.FindStringSubmatch(line); len(m) == 3 {
		metrics.Rank, _ = strconv.Atoi(m[1])
		metrics.Blocks, _ = strconv.Atoi(m[2])
	}
	if m := etaPattern.FindStringSubmatch(line); len(m) == 2 {
		metrics.ETASeconds = parseShortDurationSeconds(m[1])
	}
	return metrics, true
}

func parseShortDurationSeconds(text string) int {
	total := 0
	num := 0
	for _, r := range text {
		if r >= '0' && r <= '9' {
			num = num*10 + int(r-'0')
			continue
		}
		switch r {
		case 'h':
			total += num * 3600
		case 'm':
			total += num * 60
		case 's':
			total += num
		}
		num = 0
	}
	return total
}
