package backend

import (
	"fmt"
	"testing"
)

func TestLogHistoryRetentionAndCursor(t *testing.T) {
	h := &logHistory{entries: make(map[string][]LogEntry)}
	h.append("sender", "send-1", "first")
	initial := h.snapshot("sender", 0)
	h.append("receiver", "recv-1", "independent")
	h.append("sender", "send-2", "second")
	delta := h.snapshot("sender", initial.Latest)
	if len(delta.Entries) != 1 || delta.Entries[0].Message != "second" {
		t.Fatal(delta)
	}
	delta.Entries[0].Message = "modified copy"
	if h.snapshot("sender", initial.Latest).Entries[0].Message != "second" {
		t.Fatal("snapshot aliases history")
	}
	for i := 0; i < maxLogEntries+10; i++ {
		h.append("sender", "send-2", fmt.Sprint(i))
	}
	retained := h.snapshot("sender", 0)
	if len(retained.Entries) != maxLogEntries || retained.Entries[0].Message != "10" {
		t.Fatal("retention bound broken")
	}
	if len(h.snapshot("sender", retained.Latest).Entries) != 0 {
		t.Fatal("clear cursor replayed old logs")
	}
	if len(h.snapshot("receiver", 0).Entries) != 1 {
		t.Fatal("sender retention removed receiver history")
	}
}
