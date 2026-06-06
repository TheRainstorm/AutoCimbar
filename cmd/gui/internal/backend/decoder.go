//go:build windows

package backend

import (
	"errors"
	"fmt"
	"sync"
	"time"

	coreapp "github.com/autocambar/autocambar/pkg/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type DecoderService struct {
	app   *application.App
	mu    sync.Mutex
	tasks map[string]*decoderTask
}

type decoderTask struct {
	session ReceiverSession
	stop    chan struct{}
	pause   chan bool
	running bool
}

func NewDecoderService() *DecoderService {
	return &DecoderService{tasks: make(map[string]*decoderTask)}
}

func (s *DecoderService) Attach(app *application.App) {
	s.app = app
}

func (s *DecoderService) PrepareReceive(cfg TransferConfig) (ReceiverSession, error) {
	cfg = normalizeConfig(cfg)
	if err := ValidateConfig(cfg); err != nil {
		return ReceiverSession{}, err
	}
	id := fmt.Sprintf("recv-%d", time.Now().UnixNano())
	session := ReceiverSession{ID: id, State: StateIdle, Config: cfg}
	s.mu.Lock()
	s.tasks[id] = &decoderTask{session: session}
	s.mu.Unlock()
	s.emit("receiver:state", session)
	return session, nil
}

func (s *DecoderService) StartReceive(id string) error {
	task, err := s.get(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if task.running {
		if task.session.State == StatePaused {
			task.session.State = StateRunning
			session := task.session
			pause := task.pause
			s.mu.Unlock()
			sendPauseSignal(pause, false)
			s.emit("receiver:state", session)
			return nil
		}
		s.mu.Unlock()
		return nil
	}
	task.stop = make(chan struct{})
	task.pause = make(chan bool, 1)
	task.running = true
	task.session.State = StateRunning
	session := task.session
	stop := task.stop
	pause := task.pause
	s.mu.Unlock()
	s.emit("receiver:state", session)
	go s.run(task, session, stop, pause)
	return nil
}

func (s *DecoderService) PauseReceive(id string) error {
	return s.setPaused(id, true)
}

func (s *DecoderService) ResumeReceive(id string) error {
	return s.setPaused(id, false)
}

func (s *DecoderService) StopReceive(id string) error {
	return s.stopWithState(id, StateStopped)
}

func (s *DecoderService) GetReceiverState(id string) (ReceiverSession, error) {
	task, err := s.get(id)
	if err != nil {
		return ReceiverSession{}, err
	}
	return task.session, nil
}

func (s *DecoderService) setPaused(id string, paused bool) error {
	task, err := s.get(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if !task.running {
		s.mu.Unlock()
		if paused {
			return nil
		}
		return s.StartReceive(id)
	}
	if paused {
		task.session.State = StatePaused
	} else {
		task.session.State = StateRunning
	}
	session := task.session
	pause := task.pause
	s.mu.Unlock()

	sendPauseSignal(pause, paused)
	s.emit("receiver:state", session)
	return nil
}

func sendPauseSignal(pause chan bool, paused bool) {
	if pause == nil {
		return
	}
	select {
	case pause <- paused:
	default:
		select {
		case <-pause:
		default:
		}
		select {
		case pause <- paused:
		default:
		}
	}
}

func (s *DecoderService) run(task *decoderTask, session ReceiverSession, stop <-chan struct{}, pause <-chan bool) {
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
	log := newEventLogWriter(s.app, "receiver:log", session.ID)
	result, err := coreapp.DecodeScreenToPath(coreapp.ScreenDecodeConfig{
		OutputPath:      cfg.Output,
		Backend:         cfg.Backend,
		GridSize:        gridSize,
		Scale:           cfg.Scale,
		SymbolDir:       cfg.SymbolDir,
		ECCPercent:      eccValue(cfg),
		ColorBits:       colorBits,
		ShapeBits:       shapeBits,
		Tile:            tile,
		PacketsPerFrame: cfg.Packets,
		Region:          regionFromConfig(cfg),
		FPS:             cfg.FPS,
		DecodeWorkers:   cfg.DecodeWorkers,
		Progress:        log,
		Stop:            stop,
		Pause:           pause,
	})
	s.mu.Lock()
	if task.session.State == StateStopped {
		task.running = false
		session := task.session
		s.mu.Unlock()
		s.emit("receiver:state", session)
		return
	}
	task.running = false
	if err != nil && !errors.Is(err, coreapp.ErrStopped) {
		task.session.State = StateError
		session = task.session
		s.mu.Unlock()
		s.emit("receiver:error", map[string]any{"sessionId": session.ID, "error": err.Error()})
		s.emit("receiver:state", session)
		return
	}
	task.session.State = StateDone
	session = task.session
	s.mu.Unlock()
	output := ""
	md5 := ""
	if result != nil {
		output = result.OutputPath
		if result.Source != nil {
			md5 = result.Source.MD5
		}
	}
	s.emit("receiver:metrics", ReceiverMetrics{
		SessionID: session.ID,
		State:     StateDone,
		Progress:  100,
		Output:    output,
		UpdatedAt: time.Now(),
	})
	s.emit("receiver:done", map[string]any{"sessionId": session.ID, "output": output, "md5": md5})
	s.emit("receiver:state", session)
}

func (s *DecoderService) stopWithState(id string, state TaskState) error {
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
	s.emit("receiver:state", session)
	return nil
}

func (s *DecoderService) fail(task *decoderTask, err error) {
	s.mu.Lock()
	task.running = false
	task.session.State = StateError
	session := task.session
	s.mu.Unlock()
	s.emit("receiver:error", map[string]any{"sessionId": session.ID, "error": err.Error()})
	s.emit("receiver:state", session)
}

func (s *DecoderService) get(id string) (*decoderTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task := s.tasks[id]
	if task == nil {
		return nil, errors.New("receiver session not found")
	}
	return task, nil
}

func (s *DecoderService) emit(name string, payload any) {
	if s.app != nil {
		s.app.Event.Emit(name, payload)
	}
}
