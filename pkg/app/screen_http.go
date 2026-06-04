//go:build !windows

package app

import (
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

func runScreenEncoderBackend(cfg ScreenEncodeConfig, source *screenFrameSource, rect image.Rectangle) error {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", source.pageHandler(cfg.FPS, rect))
	mux.HandleFunc("/frame.png", source.frameHandler)
	if cfg.Open {
		go func() {
			time.Sleep(300 * time.Millisecond)
			_ = openBrowser("http://" + cfg.Addr + "/")
		}()
	}
	return http.ListenAndServe(cfg.Addr, mux)
}

func (s *screenFrameSource) frameHandler(w http.ResponseWriter, _ *http.Request) {
	img, err := s.NextImage()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, img); err != nil {
		return
	}
	s.notePresented()
}

func (s *screenFrameSource) pageHandler(fps int, rect image.Rectangle) http.HandlerFunc {
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

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
