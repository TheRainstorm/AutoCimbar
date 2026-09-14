package backend

import (
	"sync"
	"time"
)

const maxLogEntries = 2000

type LogEntry struct {
	ID        uint64 `json:"id"`
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
	At        string `json:"at"`
}
type LogBatch struct {
	Entries []LogEntry `json:"entries"`
	Latest  uint64     `json:"latest"`
}
type logHistory struct {
	mu      sync.Mutex
	latest  uint64
	entries map[string][]LogEntry
}

var transferLogs = &logHistory{entries: make(map[string][]LogEntry)}

func (h *logHistory) append(kind, session, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.latest++
	entries := h.entries[kind]
	if len(entries) == maxLogEntries {
		copy(entries, entries[1:])
		entries = entries[:len(entries)-1]
	}
	h.entries[kind] = append(entries, LogEntry{h.latest, session, message, time.Now().Format("15:04:05.000")})
}
func (h *logHistory) snapshot(kind string, after uint64) LogBatch {
	h.mu.Lock()
	defer h.mu.Unlock()
	batch := LogBatch{Latest: h.latest, Entries: []LogEntry{}}
	for _, entry := range h.entries[kind] {
		if entry.ID > after {
			batch.Entries = append(batch.Entries, entry)
		}
	}
	return batch
}
