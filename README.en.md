# AutoCimBar

AutoCimBar is a one-way file transfer tool that uses the screen channel of a remote desktop or game-streaming session. The encoder renders a file as high-density colored symbol frames on screen, inspired by [cimbar](https://github.com/sz3/cimbar), and the decoder captures a configured screen region to recover the file. It combines fountain coding, ECC, packet CRC, compression, and file-level MD5 verification. Current real-world tests have already broken the long-awaited 1 MB/s barrier.

[中文 README](README.md)

## Experimental Results

Current measurements over an RDP remote session:

- `-c 4t4s8c` (`-packets 8` when `-Q 130`)
- Effective decode FPS is capped at about 30 FPS due to remote desktop limits

| -Q  | speed       |
| --- | ----------- |
| 14  | 33 KB/s     |
| 20  | 61 KB/s     |
| 26  | 114 KB/s    |
| 130 | 1310.4 KB/s |

Moonlight behaves differently from locally rendered RDP frames because it transports a video stream. Extracting data from the compressed video image is harder; the current 60 FPS Moonlight result is 243 KB/s.

## Features

- zstd source compression by default, with streaming decompression and file MD5 verification
- Per-packet CRC to reject bad packets early when ECC is disabled or weak
- Per-frame Reed-Solomon ECC with interleaving, useful for local damage from GPU/video compression
- Linear fountain code for dropped, repeated, and redundant frames
- Native borderless topmost encoder window on Windows, no browser required
- Common tile symbol sets embedded in the executable, so a single exe can run directly
- `-backend qr` for comparing the standard QR code backend with the symbols backend
- Reads INI configuration from `~/.autocimbar`; explicit command-line options override it

## Build

Linux/macOS:

```bash
export PATH=/usr/local/go/bin:$PATH
go test ./...
go build -o bin/encoder ./cmd/encoder
go build -o bin/decoder ./cmd/decoder
go build -o bin/tilegen ./cmd/tilegen
```

Cross-compile Windows binaries on Linux/macOS:

```bash
GOOS=windows GOARCH=amd64 go build -o bin/encoder.exe ./cmd/encoder
GOOS=windows GOARCH=amd64 go build -o bin/decoder.exe ./cmd/decoder
```

GUI application for Windows:

```bash
cd cmd/gui/frontend
npm install
npm run build
cd ../../..
GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" -o bin/gui.exe ./cmd/gui
```

## Quick Start

Screen transfer:

```bash
./bin/encoder -i input.bin -RQ 120 -r 0
./bin/decoder -RQ 120 -r 1
```

Offline PNG verification:

```bash
./bin/encoder -png -i input.bin -o frames -RQ 80
./bin/decoder -png -i frames -RQ 80
```

The encoder sends the original file name as one-time source metadata. The decoder defaults to the current directory and saves using the sender file name. If `-o` points to a directory, the received file is saved in that directory with the sender file name. If `-o` points to a concrete file path, that path is used exactly.

Windows GUI:

```bash
./bin/gui.exe
```

The GUI provides independent Sender and Receiver panels that can run at the same time. The QR/symbol display still uses the native high-performance Windows topmost window. The main UI exposes `RQ`, screen selection, and capture backend; the Advanced panel highlights the frame-format settings that must match on both sides. The GUI reads `[default]` and `[gui]` from `~/.autocimbar`; when `RQ` is absent, old `Q` values are treated as the reference Q. Clicking the window `X` exits the app; clicking `To Tray` hides the dashboard in the system tray, whose menu can show or quit the app.

## Key Options

Frame format options, which must match on encoder and decoder:

```text
-backend        symbols or qr, default symbols
-c, -cell       compact cell spec, default 8t4s2c
-ecc            per-frame Reed-Solomon ECC percentage, default 3
-p, -packets    independent packets per frame
-no-zstd        encoder disables default zstd compression; decoder detects compression from source metadata
```

Runtime options:

```text
-i              input file; in decoder PNG mode, input frame directory
-o              output path; in encoder PNG mode, output frame directory; decoder defaults to current directory and uses the sender file name for directory output
-RQ             reference Q for 8x8 tiles; smaller tiles auto-scale actual Q
-Q              raw grid/cell count; RQ takes precedence when set
-B              cell scale
-f, -fps        display or capture FPS, default 120
-r, -R          screen region: SCREEN, X:Y, or SCREEN:X:Y
-png            PNG frame mode; default is screen mode
-list-displays  list display indexes and bounds
```

Decoder capture backend options:

```text
-capture-backend auto|dxgi|gdi, default auto. DXGI is fastest, but HDR/color-managed displays can break high color-bit modes; use an SDR display or gdi when colors do not decode.
-debug-capture   output directory for the first 60 captured frames, named <cell>_NNN.png; missing directories are created
-symbols        external symbol PNG directory; empty uses embedded symbols
```

Region examples:

- `-r 0`: screen 0, bottom-right by default
- `-r 1`: screen 1, bottom-right by default
- `-r 1:c:c`: center on screen 1
- `-r c:c`: center on screen 0
- `-r 1:100:200`: position `(100, 200)` on screen 1

List displays:

```bash
./bin/encoder -list-displays
./bin/decoder -list-displays
```

## Cell And Tile

Default `-c 8t4s2c` means:

```text
-tile 8x8 -shape-bits 4 -color-bits 2
```

`-cell` syntax:

- `8t` means `-tile 8x8`
- `4s` means `-shape-bits 4`
- `2c` means `-color-bits 2`

High-throughput experiments can still specify `-c 4t4s8c` explicitly.

Generate a new symbol set:

```bash
./bin/tilegen -tile 8x8 -shape-bits 6 -o generated-tiles/8x8_6bit_custom -seed 123 -attempts 20000
```

When using an external symbol directory, both sides must use the same `-symbols`, `-tile`, and `-shape-bits`.

## Configuration File

On startup, the programs read `~/.autocimbar`. The file uses INI syntax and supports no section, `[default]`, `[encoder]`, and `[decoder]`. Explicit command-line options override config values.

Example:

```ini
[default]
RQ = 120
B = 1
ecc = 3
cell = 8t4s2c
fps = 120

[encoder]
r = 0

[decoder]
r = 1
capture-backend = auto
```

Config keys use command-line option names such as `RQ`, `cell`, `packets`, and `capture-backend`; underscores are treated as dashes.

## QR Backend `-Q`

With the symbols backend, `-Q` is the number of cells per side. With the default `8x8` tile, image width is roughly `Q * 8 * B` pixels.

With the QR backend, `-Q` maps to the closest QR version. Real QR module count must follow the standard formula `17 + 4 * version`, and the image also includes a quiet zone. Therefore it is not the same cell count as the symbols backend. The current implementation scales QR modules by the `8x8` reference tile so the visible area is closer for the same `-Q`, but version rounding and quiet zone still make the exact size different.

## Decode Output

The decoder appends one progress line per second so logs can be saved and analyzed later:

```text
fields: cap=capture fps, dec=cell decode fps, pkt v/r/u=valid/repeat/useful packet fps, bad=invalid packet fps, spd=current KB/s, ema=smoothed KB/s
```

Meaning:

- `cap`: capture FPS
- `dec`: completed cell decode FPS
- `pkt v/r/u`: valid packets / repeated packets / useful packets that increase fountain rank
- `bad`: packets rejected by CRC, ECC, parameter mismatch, or parse failure
- `spd`: current-window speed
- `ema`: smoothed recent speed

The final decoder summary measures transfer time from the first valid frame, excluding time spent waiting for the encoder to start.

## Automated Tests

```bash
go test ./...
```

Tests cover PNG end-to-end transfer, QR backend PNG round trip, ECC, packet CRC, fountain recovery, zstd/MD5, dynamic tile symbol sets, and screen frame source decoding.

## Current Limitations

- The current fountain code is a linear XOR scheme. The decoder derives `blockCount` from the transfer size carried in each packet.
- `Q/RQ`, `B`, `-ecc`, `-c`, `-packets`, `-backend`, and other runtime parameters are agreed out of band and are not written into every frame.
- `-backend qr` is for comparison and is not the highest-throughput path. The fastest path is still the symbols backend.
- Non-Windows encoder screen mode currently uses the HTTP/browser fallback.

## Credits

- https://github.com/sz3/libcimbar

## License

MIT
