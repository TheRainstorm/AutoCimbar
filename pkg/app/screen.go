package app

import (
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/autocambar/autocambar/pkg/codec"
	colorpkg "github.com/autocambar/autocambar/pkg/color"
	"github.com/autocambar/autocambar/pkg/fountain"
	"github.com/kbinani/screenshot"
)

type ScreenEncodeConfig struct {
	InputPath string
	GridSize  int
	Scale     int
	SymbolDir string
	BlockSize int
	Region    string
	FPS       int
	Addr      string
	Open      bool
}

type ScreenDecodeConfig struct {
	OutputPath string
	GridSize   int
	Scale      int
	SymbolDir  string
	FileSize   int
	BlockSize  int
	Region     string
	FPS        int
	Timeout    time.Duration
}

func EncodeFileToScreen(cfg ScreenEncodeConfig) (*EncodeResult, error) {
	if cfg.FPS <= 0 {
		cfg.FPS = 30
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8080"
	}
	if err := validateGridScale(cfg.GridSize, cfg.Scale); err != nil {
		return nil, err
	}

	payloadCapacity := PayloadCapacityBytes(cfg.GridSize)
	if payloadCapacity <= 0 {
		return nil, fmt.Errorf("grid Q=%d capacity is too small: frame capacity %d, frame id %d", cfg.GridSize, GridCapacityBytes(cfg.GridSize), FrameIDSize)
	}
	blockSize := cfg.BlockSize
	if blockSize == 0 {
		blockSize = payloadCapacity
	}
	if blockSize <= 0 || blockSize > payloadCapacity {
		return nil, fmt.Errorf("block size %d exceeds frame payload capacity %d", blockSize, payloadCapacity)
	}

	data, err := os.ReadFile(cfg.InputPath)
	if err != nil {
		return nil, fmt.Errorf("read input file: %w", err)
	}
	fountainEnc, err := fountain.NewEncoder(data, blockSize)
	if err != nil {
		return nil, err
	}

	symRec, err := LoadLibcimbarSymbols(cfg.SymbolDir)
	if err != nil {
		return nil, err
	}
	codecEnc := codec.NewEncoder(symRec, colorpkg.NewRecognizer4Color(), CellSize(cfg.Scale), cfg.GridSize)

	imageSize := cfg.GridSize * CellSize(cfg.Scale)
	rect, err := ResolveEncoderRegion(cfg.Region, imageSize, imageSize)
	if err != nil {
		return nil, err
	}

	player := &screenFrameServer{
		codecEnc:    codecEnc,
		fountainEnc: fountainEnc,
		width:       imageSize,
		height:      imageSize,
	}

	result := &EncodeResult{
		GridSize:        cfg.GridSize,
		Scale:           cfg.Scale,
		CellSize:        CellSize(cfg.Scale),
		ImageSize:       imageSize,
		FrameCapacity:   GridCapacityBytes(cfg.GridSize),
		PayloadCapacity: payloadCapacity,
		FileSize:        len(data),
		BlockSize:       fountainEnc.BlockSize(),
		BlockCount:      fountainEnc.BlockCount(),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", player.pageHandler(cfg.FPS, rect))
	mux.HandleFunc("/frame.png", player.frameHandler)
	if cfg.Open {
		go func() {
			time.Sleep(300 * time.Millisecond)
			_ = openBrowser("http://" + cfg.Addr + "/")
		}()
	}
	if err := http.ListenAndServe(cfg.Addr, mux); err != nil {
		return result, err
	}
	return result, nil
}

func DecodeScreenToFile(cfg ScreenDecodeConfig) error {
	if cfg.FPS <= 0 {
		cfg.FPS = 30
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if err := validateGridScale(cfg.GridSize, cfg.Scale); err != nil {
		return err
	}
	if cfg.FileSize < 0 {
		return fmt.Errorf("file size must be >= 0")
	}

	payloadCapacity := PayloadCapacityBytes(cfg.GridSize)
	blockSize := cfg.BlockSize
	if blockSize == 0 {
		blockSize = payloadCapacity
	}
	if blockSize <= 0 || blockSize > payloadCapacity {
		return fmt.Errorf("block size %d exceeds frame payload capacity %d", blockSize, payloadCapacity)
	}
	blockCount := (cfg.FileSize + blockSize - 1) / blockSize
	if blockCount == 0 {
		blockCount = 1
	}

	symRec, err := LoadLibcimbarSymbols(cfg.SymbolDir)
	if err != nil {
		return err
	}
	codecDec := codec.NewDecoder(symRec, colorpkg.NewRecognizer4Color(), CellSize(cfg.Scale), cfg.GridSize)
	fountainDec, err := fountain.NewDecoder(cfg.FileSize, blockSize, blockCount)
	if err != nil {
		return err
	}

	imageSize := cfg.GridSize * CellSize(cfg.Scale)
	rect, err := ResolveDecoderRegion(cfg.Region, imageSize, imageSize)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(cfg.Timeout)
	interval := time.Second / time.Duration(cfg.FPS)
	if interval <= 0 {
		interval = time.Millisecond
	}

	for time.Now().Before(deadline) {
		img, err := screenshot.CaptureRect(rect)
		if err != nil {
			return fmt.Errorf("capture screen: %w", err)
		}
		packet, err := codecDec.Decode(img)
		if err != nil {
			return fmt.Errorf("decode captured frame: %w", err)
		}
		frame, err := ParsePacket(packet, blockSize)
		if err != nil {
			return fmt.Errorf("parse captured frame: %w", err)
		}
		if _, err := fountainDec.AddFrame(frame.FrameID, frame.Payload); err != nil {
			return fmt.Errorf("add captured frame: %w", err)
		}
		if fountainDec.Complete() {
			result, err := fountainDec.Decode()
			if err != nil {
				return err
			}
			return os.WriteFile(cfg.OutputPath, result, 0644)
		}
		time.Sleep(interval)
	}

	return fmt.Errorf("timeout waiting for enough frames: rank %d of %d", fountainDec.Rank(), blockCount)
}

type screenFrameServer struct {
	codecEnc    *codec.Encoder
	fountainEnc *fountain.Encoder
	frameID     uint32
	width       int
	height      int
	mu          sync.Mutex
}

func (s *screenFrameServer) frameHandler(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	block := s.fountainEnc.Encode(s.frameID)
	s.frameID++
	s.mu.Unlock()

	packet := BuildPacket(block.FrameID, block.Data)
	img, err := s.codecEnc.Encode(packet)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_ = png.Encode(w, img)
}

func (s *screenFrameServer) pageHandler(fps int, rect image.Rectangle) http.HandlerFunc {
	intervalMS := 1000 / fps
	if intervalMS <= 0 {
		intervalMS = 1
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>AutoCamBar Encoder</title>
<style>
html,body{margin:0;padding:0;background:#000;overflow:hidden;width:100%%;height:100%%}
#frame{display:block;width:%dpx;height:%dpx;image-rendering:pixelated}
</style>
</head>
<body>
<img id="frame" alt="">
<script>
const interval = %d;
let seq = 0;
try {
  window.resizeTo(%d, %d);
  window.moveTo(%d, %d);
} catch (_) {}
function next() {
  const img = new Image();
  img.onload = () => {
    document.getElementById("frame").src = img.src;
    setTimeout(next, interval);
  };
  img.onerror = () => setTimeout(next, interval);
  img.src = "/frame.png?seq=" + (seq++);
}
next();
</script>
</body>
</html>`, s.width, s.height, intervalMS, rect.Dx(), rect.Dy(), rect.Min.X, rect.Min.Y)
	}
}

func ResolveEncoderRegion(region string, width int, height int) (image.Rectangle, error) {
	if region == "" {
		region = "0:0"
	}
	bounds := screenshot.GetDisplayBounds(0)
	parts := strings.Split(region, ":")
	if len(parts) != 2 {
		return image.Rectangle{}, fmt.Errorf("encoder region must be X:Y")
	}
	x, err := resolveAxis(parts[0], bounds.Min.X, bounds.Max.X, width)
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("invalid region x: %w", err)
	}
	y, err := resolveAxis(parts[1], bounds.Min.Y, bounds.Max.Y, height)
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("invalid region y: %w", err)
	}
	return image.Rect(x, y, x+width, y+height), nil
}

func ResolveDecoderRegion(region string, width int, height int) (image.Rectangle, error) {
	if region == "" {
		return image.Rectangle{}, fmt.Errorf("decoder region must be SCREEN:X:Y")
	}
	parts := strings.Split(region, ":")
	if len(parts) != 3 {
		return image.Rectangle{}, fmt.Errorf("decoder region must be SCREEN:X:Y")
	}
	screenIndex, err := strconv.Atoi(parts[0])
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("invalid screen index: %w", err)
	}
	if screenIndex < 0 || screenIndex >= screenshot.NumActiveDisplays() {
		return image.Rectangle{}, fmt.Errorf("screen index %d out of range", screenIndex)
	}
	bounds := screenshot.GetDisplayBounds(screenIndex)
	x, err := resolveAxis(parts[1], bounds.Min.X, bounds.Max.X, width)
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("invalid region x: %w", err)
	}
	y, err := resolveAxis(parts[2], bounds.Min.Y, bounds.Max.Y, height)
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("invalid region y: %w", err)
	}
	return image.Rect(x, y, x+width, y+height), nil
}

func resolveAxis(token string, min int, max int, size int) (int, error) {
	if token == "" {
		return 0, fmt.Errorf("empty coordinate")
	}
	if strings.HasPrefix(token, "-") {
		offsetText := strings.TrimPrefix(token, "-")
		if offsetText == "" {
			offsetText = "0"
		}
		offset, err := strconv.Atoi(offsetText)
		if err != nil {
			return 0, err
		}
		return max - size - offset, nil
	}
	offset, err := strconv.Atoi(token)
	if err != nil {
		return 0, err
	}
	return min + offset, nil
}

func validateGridScale(gridSize int, scale int) error {
	if gridSize <= 0 {
		return fmt.Errorf("Q must be > 0")
	}
	if scale <= 0 {
		return fmt.Errorf("B must be > 0")
	}
	return nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
