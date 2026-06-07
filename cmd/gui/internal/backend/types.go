//go:build windows

package backend

import "time"

type TaskState string

const (
	StateIdle    TaskState = "idle"
	StateRunning TaskState = "running"
	StatePaused  TaskState = "paused"
	StateStopped TaskState = "stopped"
	StateDone    TaskState = "done"
	StateError   TaskState = "error"
)

type TransferConfig struct {
	RQ             int    `json:"rq"`
	Q              int    `json:"q,omitempty"`
	Screen         int    `json:"screen"`
	Cell           string `json:"cell"`
	ECC            *int   `json:"ecc"`
	Packets        int    `json:"packets"`
	Position       string `json:"position"`
	Scale          int    `json:"scale"`
	FPS            int    `json:"fps"`
	Output         string `json:"output"`
	Backend        string `json:"backend"`
	SymbolDir      string `json:"symbols"`
	NoZstd         bool   `json:"noZstd"`
	DecodeWorkers  int    `json:"decodeWorkers"`
	CaptureBackend string `json:"captureBackend"`
}

func DefaultTransferConfig() TransferConfig {
	ecc := 3
	return TransferConfig{
		RQ:             120,
		Screen:         0,
		Cell:           "8t4s2c",
		ECC:            &ecc,
		Packets:        1,
		Position:       "-0:-0",
		Scale:          1,
		FPS:            120,
		Output:         ".",
		Backend:        "symbols",
		SymbolDir:      "",
		NoZstd:         false,
		DecodeWorkers:  0,
		CaptureBackend: "auto",
	}
}

type SelectedFile struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type ScreenInfo struct {
	Index  int    `json:"index"`
	Label  string `json:"label"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type SenderSession struct {
	ID       string         `json:"id"`
	FilePath string         `json:"filePath"`
	FileName string         `json:"fileName"`
	FileSize int64          `json:"fileSize"`
	State    TaskState      `json:"state"`
	Config   TransferConfig `json:"config"`
}

type ReceiverSession struct {
	ID     string         `json:"id"`
	State  TaskState      `json:"state"`
	Config TransferConfig `json:"config"`
}

type ReceiverMetrics struct {
	SessionID  string    `json:"sessionId"`
	State      TaskState `json:"state"`
	Progress   float64   `json:"progress"`
	SpeedKBps  float64   `json:"speedKBps"`
	FPS        float64   `json:"fps"`
	ETASeconds int       `json:"etaSeconds"`
	Rank       int       `json:"rank"`
	Blocks     int       `json:"blocks"`
	Output     string    `json:"output"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type AppInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
