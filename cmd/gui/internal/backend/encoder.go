//go:build windows

package backend

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	coreapp "github.com/autocambar/autocambar/pkg/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type EncoderService struct {
	app   *application.App
	mu    sync.Mutex
	tasks map[string]*encoderTask
}

type encoderTask struct {
	session SenderSession
	stop    chan struct{}
	running bool
}

func NewEncoderService() *EncoderService {
	return &EncoderService{tasks: make(map[string]*encoderTask)}
}

func (s *EncoderService) Attach(app *application.App) {
	s.app = app
}

func (s *EncoderService) PrepareSend(path string, cfg TransferConfig) (SenderSession, error) {
	cfg = normalizeConfig(cfg)
	if err := ValidateConfig(cfg); err != nil {
		return SenderSession{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return SenderSession{}, err
	}
	id := fmt.Sprintf("send-%d", time.Now().UnixNano())
	session := SenderSession{
		ID:       id,
		FilePath: path,
		FileName: filepath.Base(path),
		FileSize: info.Size(),
		State:    StateIdle,
		Config:   cfg,
	}
	s.mu.Lock()
	s.tasks[id] = &encoderTask{session: session}
	s.mu.Unlock()
	s.emit("sender:state", session)
	return session, nil
}

func (s *EncoderService) StartSend(id string) error {
	task, err := s.get(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if task.running {
		s.mu.Unlock()
		return nil
	}
	task.stop = make(chan struct{})
	task.running = true
	task.session.State = StateRunning
	session := task.session
	stop := task.stop
	s.mu.Unlock()
	s.emit("sender:state", session)
	go s.run(task, session, stop)
	return nil
}

func (s *EncoderService) PauseSend(id string) error {
	return s.stopWithState(id, StatePaused)
}

func (s *EncoderService) ResumeSend(id string) error {
	return s.StartSend(id)
}

func (s *EncoderService) StopSend(id string) error {
	return s.stopWithState(id, StateStopped)
}

func (s *EncoderService) GetSenderState(id string) (SenderSession, error) {
	task, err := s.get(id)
	if err != nil {
		return SenderSession{}, err
	}
	return task.session, nil
}

func (s *EncoderService) run(task *encoderTask, session SenderSession, stop <-chan struct{}) {
	cfg := session.Config
	tile, shapeBits, colorBits, err := cellParts(cfg)
	if err != nil {
		s.fail(task, err)
		return
	}
	gridSize, err := gridSizeFromConfig(cfg, tile, shapeBits)
	if err != nil {
		s.fail(task, err)
		return
	}
	log := newEventLogWriter(s.app, "sender:log", session.ID)
	result, err := coreapp.EncodeFileToScreen(coreapp.ScreenEncodeConfig{
		InputPath:       session.FilePath,
		Backend:         cfg.Backend,
		GridSize:        gridSize,
		Scale:           cfg.Scale,
		SymbolDir:       cfg.SymbolDir,
		ECCPercent:      eccValue(cfg),
		ColorBits:       colorBits,
		ShapeBits:       shapeBits,
		Tile:            tile,
		PacketsPerFrame: cfg.Packets,
		NoZstd:          cfg.NoZstd,
		Region:          regionFromConfig(cfg),
		FPS:             cfg.FPS,
		Progress:        log,
		Stop:            stop,
	})
	s.mu.Lock()
	if task.session.State == StatePaused || task.session.State == StateStopped {
		task.running = false
		session := task.session
		s.mu.Unlock()
		s.emit("sender:window-closed", map[string]any{"sessionId": session.ID})
		s.emit("sender:state", session)
		return
	}
	task.running = false
	if err != nil && !errors.Is(err, coreapp.ErrStopped) {
		task.session.State = StateError
		session = task.session
		s.mu.Unlock()
		s.emit("sender:error", map[string]any{"sessionId": session.ID, "error": err.Error()})
		s.emit("sender:state", session)
		return
	}
	task.session.State = StateDone
	session = task.session
	s.mu.Unlock()
	s.emit("sender:window-closed", map[string]any{"sessionId": session.ID})
	s.emit("sender:done", map[string]any{
		"sessionId": session.ID,
		"fileName":  session.FileName,
		"md5":       md5FromEncodeResult(result),
	})
	s.emit("sender:state", session)
}

func (s *EncoderService) stopWithState(id string, state TaskState) error {
	task, err := s.get(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if task.stop != nil && task.running {
		close(task.stop)
	}
	task.running = false
	task.session.State = state
	session := task.session
	s.mu.Unlock()
	s.emit("sender:window-closed", map[string]any{"sessionId": id})
	s.emit("sender:state", session)
	return nil
}

func (s *EncoderService) fail(task *encoderTask, err error) {
	s.mu.Lock()
	task.running = false
	task.session.State = StateError
	session := task.session
	s.mu.Unlock()
	s.emit("sender:error", map[string]any{"sessionId": session.ID, "error": err.Error()})
	s.emit("sender:state", session)
}

func (s *EncoderService) get(id string) (*encoderTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task := s.tasks[id]
	if task == nil {
		return nil, errors.New("sender session not found")
	}
	return task, nil
}

func (s *EncoderService) emit(name string, payload any) {
	if s.app != nil {
		s.app.Event.Emit(name, payload)
	}
}

func md5FromEncodeResult(result *coreapp.EncodeResult) string {
	if result == nil {
		return ""
	}
	return result.MD5
}
