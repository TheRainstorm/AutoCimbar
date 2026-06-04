package app

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

type screenEncoderProgress struct {
	out           io.Writer
	start         time.Time
	last          time.Time
	fileSize      int
	checksum      uint32
	blockSize     int
	blockCount    int
	frameCapacity int
	payloadBytes  int
	encoded       uint64
	presented     uint64
	lastEncoded   uint64
	lastPresented uint64
	mu            sync.Mutex
}

func newScreenEncoderProgress(out io.Writer, result *EncodeResult, checksum uint32) *screenEncoderProgress {
	if out == nil {
		return nil
	}
	now := time.Now()
	return &screenEncoderProgress{
		out:           out,
		start:         now,
		last:          now,
		fileSize:      result.FileSize,
		checksum:      checksum,
		blockSize:     result.BlockSize,
		blockCount:    result.BlockCount,
		frameCapacity: result.FrameCapacity,
		payloadBytes:  result.PayloadCapacity,
	}
}

func (p *screenEncoderProgress) startSummary() {
	if p == nil {
		return
	}
	fmt.Fprintf(p.out, "file=%d bytes crc32=%08x blocks=%d\n", p.fileSize, p.checksum, p.blockCount)
	fmt.Fprintf(p.out, "raw block=%d bytes (%d bits)\n", p.blockSize, p.blockSize*8)
	fmt.Fprintf(p.out, "fountain payload=%d bytes (%d bits)\n", p.blockSize, p.blockSize*8)
	fmt.Fprintf(p.out, "ecc=disabled effective=%d bytes (%d bits)\n", p.blockSize, p.blockSize*8)
	fmt.Fprintf(p.out, "frame header=%d bytes (magic=ACB1 file_size=8 frame_id=4) block_payload=%d/%d bytes codec_payload=%d/%d bytes\n",
		FrameHeaderSize, p.blockSize, p.payloadBytes, FrameHeaderSize+p.blockSize, p.frameCapacity)
}

func (p *screenEncoderProgress) noteEncoded() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.encoded++
	p.render(false)
}

func (p *screenEncoderProgress) notePresented() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.presented++
	p.render(false)
}

func (p *screenEncoderProgress) finishLine() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.render(true)
	fmt.Fprintln(p.out)
}

func (p *screenEncoderProgress) render(force bool) {
	now := time.Now()
	window := now.Sub(p.last)
	if !force && window < time.Second {
		return
	}
	if window <= 0 {
		window = time.Nanosecond
	}

	encodeFPS := float64(p.encoded-p.lastEncoded) / window.Seconds()
	refreshFPS := float64(p.presented-p.lastPresented) / window.Seconds()
	done := p.encoded
	if uint64(p.blockCount) < done {
		done = uint64(p.blockCount)
	}
	progress := 1.0
	if p.blockCount > 0 {
		progress = float64(done) / float64(p.blockCount)
	}
	extra := ""
	if p.encoded > uint64(p.blockCount) {
		extra = fmt.Sprintf(" +%d", p.encoded-uint64(p.blockCount))
	}

	fmt.Fprintf(p.out, "\r%s encode_fps=%5.1f refresh_fps=%5.1f frames=%d/%d%s elapsed=%s%s",
		progressBar(progress, 24), encodeFPS, refreshFPS, done, p.blockCount, extra, shortDuration(now.Sub(p.start)), clearLine())

	p.last = now
	p.lastEncoded = p.encoded
	p.lastPresented = p.presented
}

type screenDecoderProgress struct {
	out               io.Writer
	start             time.Time
	last              time.Time
	fileSize          int
	blockSize         int
	blockCount        int
	rank              int
	captured          uint64
	valid             uint64
	invalid           uint64
	lastCaptured      uint64
	lastValid         uint64
	lastRecoveredByte int
}

func newScreenDecoderProgress(out io.Writer, blockSize int) *screenDecoderProgress {
	if out == nil {
		return nil
	}
	now := time.Now()
	return &screenDecoderProgress{
		out:       out,
		start:     now,
		last:      now,
		fileSize:  -1,
		blockSize: blockSize,
	}
}

func (p *screenDecoderProgress) noteCaptured() {
	if p == nil {
		return
	}
	p.captured++
	p.render(false)
}

func (p *screenDecoderProgress) noteInvalid() {
	if p == nil {
		return
	}
	p.invalid++
	p.render(false)
}

func (p *screenDecoderProgress) noteStarted(fileSize int, blockCount int) {
	if p == nil {
		return
	}
	if p.fileSize >= 0 {
		return
	}
	p.fileSize = fileSize
	p.blockCount = blockCount
	fmt.Fprintf(p.out, "\nvalid frame detected: file=%d bytes blocks=%d block=%d bytes\n", fileSize, blockCount, p.blockSize)
}

func (p *screenDecoderProgress) noteValid(rank int) {
	if p == nil {
		return
	}
	p.valid++
	p.rank = rank
	p.render(false)
}

func (p *screenDecoderProgress) finishLine() {
	if p == nil {
		return
	}
	p.render(true)
	fmt.Fprintln(p.out)
}

func (p *screenDecoderProgress) render(force bool) {
	now := time.Now()
	window := now.Sub(p.last)
	if !force && window < time.Second {
		return
	}
	if window <= 0 {
		window = time.Nanosecond
	}

	captureFPS := float64(p.captured-p.lastCaptured) / window.Seconds()
	decodeFPS := float64(p.valid-p.lastValid) / window.Seconds()
	recovered := p.recoveredBytes()
	currentKB := float64(recovered-p.lastRecoveredByte) / window.Seconds() / 1024
	elapsed := now.Sub(p.start)
	averageKB := 0.0
	if elapsed > 0 {
		averageKB = float64(recovered) / elapsed.Seconds() / 1024
	}

	if p.fileSize < 0 {
		fmt.Fprintf(p.out, "\rwaiting for valid frame capture_fps=%5.1f invalid=%d elapsed=%s%s",
			captureFPS, p.invalid, shortDuration(elapsed), clearLine())
	} else {
		progress := 1.0
		if p.fileSize > 0 {
			progress = float64(recovered) / float64(p.fileSize)
			if progress > 1 {
				progress = 1
			}
		}
		eta := "unknown"
		if averageKB > 0 && recovered < p.fileSize {
			remaining := float64(p.fileSize-recovered) / 1024 / averageKB
			eta = shortDuration(time.Duration(remaining * float64(time.Second)))
		} else if recovered >= p.fileSize {
			eta = "0s"
		}
		fmt.Fprintf(p.out, "\r%s capture_fps=%5.1f decode_fps=%5.1f speed=%7.1f KB/s avg=%7.1f KB/s data=%s/%s rank=%d/%d elapsed=%s eta=%s%s",
			progressBar(progress, 24), captureFPS, decodeFPS, currentKB, averageKB,
			formatBytes(recovered), formatBytes(p.fileSize), p.rank, p.blockCount,
			shortDuration(elapsed), eta, clearLine())
	}

	p.last = now
	p.lastCaptured = p.captured
	p.lastValid = p.valid
	p.lastRecoveredByte = recovered
}

func (p *screenDecoderProgress) recoveredBytes() int {
	if p.fileSize < 0 {
		return 0
	}
	recovered := p.rank * p.blockSize
	if recovered > p.fileSize {
		recovered = p.fileSize
	}
	return recovered
}

func progressBar(progress float64, width int) string {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	filled := int(progress * float64(width))
	if filled > width {
		filled = width
	}
	return fmt.Sprintf("[%s%s] %5.1f%%", strings.Repeat("#", filled), strings.Repeat(".", width-filled), progress*100)
}

func formatBytes(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.2f MB", float64(n)/(1024*1024))
}

func shortDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(d/time.Minute), int(d%time.Minute/time.Second))
	}
	return fmt.Sprintf("%dh%02dm%02ds", int(d/time.Hour), int(d%time.Hour/time.Minute), int(d%time.Minute/time.Second))
}

func clearLine() string {
	return "\x1b[K"
}
