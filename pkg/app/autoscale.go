package app

import (
	"fmt"
	"image"
	"io"
	"math"
	"sort"
	"time"

	"github.com/autocambar/autocambar/pkg/codec"
	"github.com/autocambar/autocambar/pkg/ecc"
	"github.com/autocambar/autocambar/pkg/symbol"
)

// Auto-scale owns its probe decoder and buffers; decode workers never share them.
type autoScaleCapturer struct {
	source                         screenFrameCapturer
	symbols                        *symbol.Recognizer
	decoder                        frameDecoder
	packetCodec                    *ecc.PacketCodec
	grid, size, blockSize, packets int
	desktop                        image.Point
	region                         image.Rectangle
	sampler                        *scaledFrameSampler
	fullBuffer, payload, packet    []byte
	nextSearch, nextCheck          time.Time
	pendingRect                    image.Rectangle
	pendingSampler                 *scaledFrameSampler
	nextDiagnostic                 time.Time
	nextProbe                      time.Time
	failedChecks                   int
	debugDir, debugCell            string
	debugCount                     int
	log                            io.Writer
	stop                           <-chan struct{}
}

func newAutoScaleCapturer(source screenFrameCapturer, cfg ScreenDecodeConfig, spec symbol.Spec, size, capacity, blockSize int, stop <-chan struct{}) (*autoScaleCapturer, error) {
	syms, err := LoadSymbols(cfg.SymbolDir, spec)
	if err != nil {
		return nil, err
	}
	colors, err := colorRecognizerForBits(normalizeColorBits(cfg.ColorBits))
	if err != nil {
		return nil, err
	}
	dec, err := codec.NewDecoderWithColorBits(syms, colors, CellSizeForSpec(cfg.Scale, spec), cfg.GridSize, normalizeColorBits(cfg.ColorBits))
	if err != nil {
		return nil, err
	}
	packets := normalizePacketsPerFrame(cfg.PacketsPerFrame)
	pc, err := NewFramePacketCodecWithFrameCapacityAndPackets(capacity, cfg.ECCPercent, blockSize, packets)
	if err != nil {
		return nil, err
	}
	return &autoScaleCapturer{source: source, symbols: syms, decoder: dec, packetCodec: pc,
		grid: cfg.GridSize, size: size, blockSize: blockSize, packets: packets, log: cfg.Progress, stop: stop,
		debugDir: cfg.DebugCapturePath, debugCell: CellSpecName(cfg.Tile, spec.ShapeBits, cfg.ColorBits)}, nil
}

func (a *autoScaleCapturer) CaptureFrame(dst []byte) (*capturedScreenFrame, error) {
	full, err := a.source.CaptureFrame(a.fullBuffer)
	if err != nil || full == nil {
		return nil, err
	}
	if err := validateScaleFrame(full); err != nil {
		return nil, err
	}
	desktop := image.Pt(full.Width, full.Height)
	if desktop != a.desktop {
		a.desktop = desktop
		a.region = image.Rectangle{}
		a.sampler = nil
		a.pendingSampler = nil
		a.nextSearch = time.Time{}
	}
	a.fullBuffer = full.Pix
	if a.debugDir != "" && a.debugCount < 60 {
		path := debugCaptureFramePath(a.debugDir, a.debugCell, a.debugCount)
		if err := saveCapturedFramePNG(path, full); err != nil {
			return nil, fmt.Errorf("save auto-scale desktop capture: %w", err)
		}
		a.debugCount++
		if a.log != nil {
			fmt.Fprintf(a.log, "debug capture saved: %s\n", path)
		}
	}
	now := time.Now()
	if !a.region.Empty() {
		if !a.region.In(image.Rect(0, 0, full.Width, full.Height)) {
			a.region = image.Rectangle{}
		} else {
			frame := a.sampler.normalize(full, dst)
			if !now.Before(a.nextCheck) {
				a.nextCheck = now.Add(time.Second)
				if a.valid(frame) {
					a.failedChecks = 0
				} else {
					a.failedChecks++
				}
				if a.failedChecks >= 3 {
					score := scaledTemplateScore(full, a.region, a.grid, a.symbols)
					if score < 0.10 {
						// Mixed frames can retain perfectly aligned symbols while failing
						// packet CRC. Do not confuse transport damage with scale changes.
						if a.log != nil && a.failedChecks%5 == 3 {
							fmt.Fprintf(a.log, "auto-scale: geometry stable (score=%.3f), packet checks failing; possible remote compression/mixed frames, lower sender FPS\n", score)
						}
					} else {
						a.region = image.Rectangle{}
						a.pendingSampler = nil
						if a.log != nil {
							fmt.Fprintf(a.log, "auto-scale: geometry changed or degraded (score=%.3f); searching again\n", score)
						}
						return nil, nil
					}
				}
			}
			return frame, nil
		}
	}
	if now.Before(a.nextSearch) {
		// Retry promising geometry on fresh captures, rather than waiting for
		// the next full search to happen to coincide with an intact packet.
		if a.pendingSampler != nil && !now.Before(a.nextProbe) {
			a.nextProbe = now.Add(100 * time.Millisecond)
			frame := a.pendingSampler.normalize(full, dst)
			if a.valid(frame) {
				a.lock(a.pendingRect, a.pendingSampler)
				return frame, nil
			}
		}
		return nil, nil
	}
	defer func() { a.nextSearch = time.Now().Add(time.Second) }()
	candidates := centeredScaleCandidates(full, a.grid, a.symbols, a.stop)
	a.pendingSampler = nil
	if len(candidates) > 0 && candidates[0].score < 0.10 {
		a.pendingRect = candidates[0].rect
		a.pendingSampler = newScaledFrameSampler(full, a.pendingRect, a.size, a.grid, a.symbols.Spec())
	}
	for _, candidate := range candidates {
		select {
		case <-a.stop:
			return nil, nil
		default:
		}
		sampler := newScaledFrameSampler(full, candidate.rect, a.size, a.grid, a.symbols.Spec())
		frame := sampler.normalize(full, dst)
		dst = frame.Pix
		if !a.valid(frame) {
			continue
		}
		a.lock(candidate.rect, sampler)
		return frame, nil
	}
	if a.log != nil && a.pendingSampler != nil && !now.Before(a.nextDiagnostic) {
		a.nextDiagnostic = now.Add(5 * time.Second)
		fmt.Fprintf(a.log, "auto-scale: symbol geometry candidate rect=%v score=%.3f, no valid packet yet; retrying fresh captures (check frame format or lower sender FPS)\n", a.pendingRect, candidates[0].score)
	}
	return nil, nil
}

func (a *autoScaleCapturer) lock(rect image.Rectangle, sampler *scaledFrameSampler) {
	a.region, a.sampler = rect, sampler
	a.pendingSampler = nil
	a.failedChecks = 0
	a.nextCheck = time.Now().Add(time.Second)
	if a.log != nil {
		fmt.Fprintf(a.log, "auto-scale: locked rect=%v (display-local) scale=%.4f size=%d -> %d pixels\n", rect, float64(rect.Dx())/float64(a.size), rect.Dx(), a.size)
	}
}

// Validate capture layout before any template or bilinear sampling indexes it.
func validateScaleFrame(frame *capturedScreenFrame) error {
	pix, stride := frame.Pix, frame.Stride
	if pix == nil && frame.Img != nil {
		pix, stride = frame.Img.Pix, frame.Img.Stride
	}
	if frame.Width <= 0 || frame.Height <= 0 || stride <= 0 || frame.Width > stride/4 ||
		frame.Height-1 > len(pix)/stride ||
		frame.Width*4 > len(pix)-(frame.Height-1)*stride {
		return fmt.Errorf("auto-scale: invalid capture layout %dx%d stride=%d bytes=%d", frame.Width, frame.Height, stride, len(pix))
	}
	return nil
}

func (a *autoScaleCapturer) valid(frame *capturedScreenFrame) bool {
	payload, err := decodeCapturedFrame(a.decoder, frame, a.payload)
	if err != nil {
		return false
	}
	a.payload = payload
	n := a.packetCodec.EncodedSize()
	for i := 0; i < a.packets && (i+1)*n <= len(payload); i++ {
		packet, err := a.packetCodec.DecodeInto(payload[i*n:(i+1)*n], a.packet)
		if err != nil {
			continue
		}
		a.packet = packet
		if _, err := ParsePacket(packet, a.blockSize); err == nil {
			return true
		}
	}
	return false
}

type scaleCandidate struct {
	rect  image.Rectangle
	score float64
}

// Search square frames centered on the selected display. A sparse template score
// ranks sizes; only ECC/CRC validation of a complete packet may lock a candidate.
func centeredScaleCandidates(frame *capturedScreenFrame, grid int, syms *symbol.Recognizer, stop <-chan struct{}) []scaleCandidate {
	spec := syms.Spec()
	minimum := grid * max(spec.Width, spec.Height)
	maximum := min(frame.Width, frame.Height)
	candidates := make([]scaleCandidate, 0, max(0, maximum-minimum+1))
	for side := minimum; side <= maximum; side++ {
		select {
		case <-stop:
			return nil
		default:
		}
		x, y := (frame.Width-side)/2, (frame.Height-side)/2
		r := image.Rect(x, y, x+side, y+side)
		score := scaledTemplateScore(frame, r, grid, syms)
		if score < 0.22 {
			candidates = append(candidates, scaleCandidate{r, score})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score < candidates[j].score })
	// Adjacent sizes often describe the same peak. Keep a few distinct peaks,
	// then try their immediate neighbours to tolerate rounded scaling edges.
	var selected []scaleCandidate
	for _, c := range candidates {
		near := false
		for _, other := range selected {
			if absInt(c.rect.Dx()-other.rect.Dx()) <= 2 {
				near = true
				break
			}
		}
		if !near {
			selected = append(selected, c)
		}
		if len(selected) == 8 {
			break
		}
	}
	var refined []scaleCandidate
	seen := make(map[int]bool)
	for _, c := range selected {
		for _, delta := range []int{0, -1, 1, -2, 2} {
			side := c.rect.Dx() + delta
			if side < minimum || side > maximum || seen[side] {
				continue
			}
			seen[side] = true
			x, y := (frame.Width-side)/2, (frame.Height-side)/2
			refined = append(refined, scaleCandidate{image.Rect(x, y, x+side, y+side), c.score})
		}
	}
	return refined
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func scaledTemplateScore(frame *capturedScreenFrame, rect image.Rectangle, grid int, syms *symbol.Recognizer) float64 {
	spec := syms.Spec()
	var total float64
	// Distributed cells avoid inferring scale from a single central symbol.
	for i := 0; i < 16; i++ {
		cx := (grid/8 + i*37) % grid
		cy := (grid/7 + i*53) % grid
		var samples [64]uint8
		sum, low, high := 0, 255, 0
		for ty := 0; ty < spec.Height; ty++ {
			for tx := 0; tx < spec.Width; tx++ {
				x := float64(rect.Min.X) + (float64(cx)+(float64(tx)+0.5)/float64(spec.Width))*float64(rect.Dx())/float64(grid)
				y := float64(rect.Min.Y) + (float64(cy)+(float64(ty)+0.5)/float64(spec.Height))*float64(rect.Dy())/float64(grid)
				p := framePixel(frame, int(x), int(y))
				v := int(max(p[0], p[1], p[2]))
				samples[ty*spec.Width+tx] = uint8(v)
				sum += v
				low = min(low, v)
				high = max(high, v)
			}
		}
		if high-low < 48 {
			total += 1
			continue
		}
		threshold := sum / spec.TileBits()
		var hash uint64
		for _, v := range samples[:spec.TileBits()] {
			hash <<= 1
			if int(v) > threshold {
				hash |= 1
			}
		}
		_, distance := syms.RecognizeHash(hash)
		total += float64(distance) / float64(spec.TileBits())
	}
	return total / 16
}

func framePixel(frame *capturedScreenFrame, x, y int) []byte {
	x = max(0, min(x, frame.Width-1))
	y = max(0, min(y, frame.Height-1))
	if frame.Pix != nil {
		return frame.Pix[y*frame.Stride+x*4:][:4]
	}
	return frame.Img.Pix[y*frame.Img.Stride+x*4:][:4]
}

type scaleSample struct {
	lo, hi int
	weight uint32
}

type scaledFrameSampler struct {
	size int
	x, y []scaleSample
}

func scaleAxisSamples(origin, extent, limit, size, grid, tile int) []scaleSample {
	axis := make([]scaleSample, size)
	cell := size / grid
	for i := range axis {
		pixel := (i % cell) * tile / cell
		position := float64(origin) + (float64(i/cell)+(float64(pixel)+0.5)/float64(tile))*float64(extent)/float64(grid) - 0.5
		lo := int(math.Floor(position))
		axis[i] = scaleSample{max(0, min(lo, limit-1)), max(0, min(lo+1, limit-1)), uint32((position-float64(lo))*256 + 0.5)}
	}
	return axis
}

// Sample logical tile pixel centres. Mapping is reused while the region is
// locked; B only changes output replication, never the sampling phase.
func newScaledFrameSampler(src *capturedScreenFrame, rect image.Rectangle, size, grid int, spec symbol.Spec) *scaledFrameSampler {
	return &scaledFrameSampler{size: size,
		x: scaleAxisSamples(rect.Min.X, rect.Dx(), src.Width, size, grid, spec.Width),
		y: scaleAxisSamples(rect.Min.Y, rect.Dy(), src.Height, size, grid, spec.Height)}
}

func (s *scaledFrameSampler) normalize(src *capturedScreenFrame, dst []byte) *capturedScreenFrame {
	size := s.size
	need := size * size * 4
	if cap(dst) < need {
		dst = make([]byte, need)
	} else {
		dst = dst[:need]
	}
	pix, stride := src.Pix, src.Stride
	if pix == nil {
		pix, stride = src.Img.Pix, src.Img.Stride
	}
	for y, ys := range s.y {
		row0 := pix[ys.lo*stride:][:src.Width*4]
		row1 := pix[ys.hi*stride:][:src.Width*4]
		for x, xs := range s.x {
			p00, p10 := row0[xs.lo*4:][:3], row0[xs.hi*4:][:3]
			p01, p11 := row1[xs.lo*4:][:3], row1[xs.hi*4:][:3]
			i := (y*size + x) * 4
			for c := 0; c < 3; c++ {
				top := uint32(p00[c])*(256-xs.weight) + uint32(p10[c])*xs.weight
				bottom := uint32(p01[c])*(256-xs.weight) + uint32(p11[c])*xs.weight
				dst[i+c] = uint8((top*(256-ys.weight) + bottom*ys.weight + 32768) >> 16)
			}
			dst[i+3] = 255
		}
	}
	frame := &capturedScreenFrame{Pix: dst, Width: size, Height: size, Stride: size * 4, BGRA: src.BGRA}
	if !src.BGRA {
		frame.Img = &image.RGBA{Pix: dst, Stride: size * 4, Rect: image.Rect(0, 0, size, size)}
	}
	return frame
}
