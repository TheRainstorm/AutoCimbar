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

## Quick Start

Screen transfer:

```bash
./bin/encoder -i input.bin -RQ 120 -r 0
./bin/decoder -o output.bin -RQ 120 -r 1
```

Offline PNG verification:

```bash
./bin/encoder -png -i input.bin -o frames -RQ 80
./bin/decoder -png -i frames -o output.bin -RQ 80
```

## Common Options

```text
-i              input file; in decoder PNG mode, input frame directory
-o              output path; in encoder PNG mode, output frame directory
-Q              grid/cell count; QR backend maps it to a QR version
-RQ             reference Q for 8x8 tiles; smaller tiles auto-scale actual Q
-B              cell scale
-c, -cell       compact cell spec, default 4t4s8c
-p, -packets    independent packets per frame
-ecc            per-frame Reed-Solomon ECC percentage, default 3
-f, -fps        display or capture FPS, default 120
-r, -R          screen region: SCREEN, X:Y, or SCREEN:X:Y
-png            PNG frame mode; default is screen mode
-backend        symbols or qr, default symbols
-no-zstd        disable default zstd compression
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

Default `-c 4t4s8c` means:

```text
-tile 4x4 -shape-bits 4 -color-bits 8
```

`-cell` syntax:

- `4t` means `-tile 4x4`
- `4s` means `-shape-bits 4`
- `8c` means `-color-bits 8`

Generate a new symbol set:

```bash
./bin/tilegen -tile 8x8 -shape-bits 6 -o generated-tiles/8x8_6bit_custom -seed 123 -attempts 20000
```

When using an external symbol directory, both sides must use the same `-symbols`, `-tile`, and `-shape-bits`.

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
