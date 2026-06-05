# AutoCamBar

Cheers: AutoCamBar has reached the long-awaited 1 MB/s milestone.

AutoCamBar transfers files over a one-way screen channel. The encoder renders high-density colored symbol frames on screen, and the decoder captures a configured screen region to reconstruct the file. Runtime parameters are agreed out of band, so frames carry only real transfer data.

[中文 README](README.md)

## Current Results

The recommended default cell is now `-c 4t4s8c`.

`-c 4t4s8c` (`-packets 8` when `-Q 130`)

| -Q  | speed       |
| --- | ----------- |
| 14  | 33 KB/s     |
| 20  | 61 KB/s     |
| 26  | 114 KB/s    |
| 130 | 1310.4 KB/s |

## Features

- Screen mode by default, with `-Q 120 -fps 120 -ecc 3 -c 4t4s8c`
- zstd source compression by default, with streaming decompression and MD5 verification
- Per-packet CRC to reject bad packets early when ECC is disabled or weak
- Per-frame Reed-Solomon ECC with interleaving
- Linear fountain code for dropped, repeated, and redundant frames
- Native borderless topmost encoder window on Windows
- Common tile sets embedded in the executable
- `-backend qr` for speed comparison against a standard QR code backend

## Build

Linux/macOS:

```bash
export PATH=/usr/local/go/bin:$PATH
go test ./...
go build -o bin/encoder ./cmd/encoder
go build -o bin/decoder ./cmd/decoder
go build -o bin/tilegen ./cmd/tilegen
```

Windows PowerShell:

```powershell
go test ./...
go build -o bin\encoder.exe .\cmd\encoder
go build -o bin\decoder.exe .\cmd\decoder
go build -o bin\tilegen.exe .\cmd\tilegen
```

Cross-compile Windows binaries:

```bash
GOOS=windows GOARCH=amd64 go build -o bin/encoder.exe ./cmd/encoder
GOOS=windows GOARCH=amd64 go build -o bin/decoder.exe ./cmd/decoder
```

## Quick Start

Recommended screen transfer:

```bash
./bin/encoder -i input.bin -RQ 120 -p 3 -r 0
./bin/decoder -o output.bin -RQ 120 -p 3 -r 1
```

High-throughput experiment:

```bash
./bin/encoder -i 10M.bin -f 200 -RQ 130 -p 8 -r 0
./bin/decoder -o out.bin -f 120 -RQ 130 -p 8 -r 1
```

Offline PNG test:

```bash
./bin/encoder -png -i input.bin -o frames -RQ 80
./bin/decoder -png -i frames -o output.bin -RQ 80
```

The decoder prints the output MD5 after completion. A source MD5 mismatch is treated as an error.

## QR Backend

Default high-speed symbols backend:

```bash
./bin/encoder -i input.bin -RQ 120
./bin/decoder -o output.bin -RQ 120
```

Standard QR code backend:

```bash
./bin/encoder -backend qr -i input.bin -Q 33 -B 4
./bin/decoder -backend qr -o output.bin -Q 33 -B 4
```

PNG comparison:

```bash
./bin/encoder -png -backend qr -i input.bin -o qr_frames -Q 33 -B 4
./bin/decoder -png -backend qr -i qr_frames -o output.bin -Q 33 -B 4
```

`-backend qr` reuses the existing zstd, packet CRC, ECC, fountain code, and MD5 verification layers. Only the frame renderer/reader changes. QR `-Q` maps to the closest QR version.

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

- `-r 0`: screen 0, bottom-right
- `-r 1`: screen 1, bottom-right
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

Embedded tile sets:

```text
8x8_5bit   32 symbols
8x8_6bit   64 symbols
6x6_4bit   16 symbols
4x4_4bit   16 symbols
4x4_3bit    8 symbols
4x4_2bit    4 symbols
```

Generate a new tile set:

```bash
./bin/tilegen -tile 8x8 -shape-bits 6 -o generated-tiles/8x8_6bit_custom -seed 123 -attempts 20000
```

When using an external symbol directory, both sides must use the same `-symbols`, `-tile`, and `-shape-bits`.

## Progress Log

The decoder appends one line per second:

```text
fields: cap=capture fps, dec=cell decode fps, pkt v/r/u=valid/repeat/useful packet fps, bad=invalid packet fps, spd=current KB/s, ema=smoothed KB/s
```

- `cap`: capture FPS
- `dec`: cell decode FPS
- `pkt v/r/u`: valid, repeated, and useful packets per second
- `bad`: invalid packets per second
- `spd`: current-window speed
- `ema`: smoothed recent speed

The final summary excludes time spent waiting for the first valid encoder frame.

## Tests

```bash
go test ./...
```

Tests cover PNG round trips, QR backend PNG round trips, ECC, packet CRC, fountain recovery, zstd/MD5, generated tile specs, and screen frame source decoding.

## Limitations

- The current fountain code is a linear XOR fountain. The decoder derives block count from transfer size carried in the packet header.
- `Q/RQ`, `B`, `-ecc`, `-c`, `-packets`, and `-backend` are runtime agreements and are not written into every frame.
- `-backend qr` is for comparison. The fastest path is still the symbols backend.
- Non-Windows screen encoder mode currently uses the HTTP/browser fallback.

## License

MIT
