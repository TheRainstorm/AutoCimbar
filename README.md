# AutoCamBar

已经实现了梦寐以求的 1MB/s 大关

AutoCamBar 是一个面向远程桌面、串流画面和单向屏幕信道的文件传输工具。encoder 把文件编码成屏幕上的高密度彩色符号帧，decoder 从指定屏幕区域截图并恢复文件。运行参数由两端提前约定，帧里只放真正传输数据。

[English README](README.en.md)

## 实验结果

以下为当前 RDP 远程实测结果：

- `-c 4t4s8c` （`-Q 130` 时，`-packets 8`）
- 有效解码 fps 只有最高 30 fps（远程桌面限制）

| -Q  | speed       |
| --- | ----------- |
| 14  | 33 KB/s     |
| 20  | 61 KB/s     |
| 26  | 114 KB/s    |
| 130 | 1310.4 KB/s |

## 功能概览

- 默认 zstd 压缩源文件，decoder 流式解压并校验文件 MD5
- 每个 packet 内置 CRC，能尽早丢弃无 ECC 或低 ECC 下的错误 packet
- 单帧 Reed-Solomon ECC，支持交织，适合 GPU 编码/串流压缩造成的局部错误
- 线性喷泉码，支持丢帧、重复帧和冗余帧
- Windows encoder 使用原生无边框置顶窗口，不需要浏览器
- 常用 tile 符号集已编译进程序，单 exe 分发即可运行
- 新增 `-backend qr`，可用标准 QR code backend 和 symbols backend 做速度对比

## 编译

Linux/macOS:

```bash
export PATH=/usr/local/go/bin:$PATH
go test ./...
go build -o bin/encoder ./cmd/encoder
go build -o bin/decoder ./cmd/decoder
go build -o bin/tilegen ./cmd/tilegen
```

Linux/macOS 交叉编译 Windows:

```bash
GOOS=windows GOARCH=amd64 go build -o bin/encoder.exe ./cmd/encoder
GOOS=windows GOARCH=amd64 go build -o bin/decoder.exe ./cmd/decoder
```

## 快速使用

屏幕传输命令：

```bash
./bin/encoder -i input.bin -RQ 120 -r 0
./bin/decoder -o output.bin -RQ 120 -r 1
```

PNG 离线验证：

```bash
./bin/encoder -png -i input.bin -o frames -RQ 80
./bin/decoder -png -i frames -o output.bin -RQ 80
```

## 关键参数

常用参数：

```text
-i              输入文件；decoder PNG 模式下为帧目录
-o              输出路径；encoder PNG 模式下为帧目录
-Q              grid/cell 数；QR backend 下映射为 QR version
-RQ             以 8x8 tile 为基准的参考 Q，tile 变小时自动增大实际 Q
-B              cell 缩放倍数
-c, -cell       紧凑 cell 规格，默认 4t4s8c
-p, -packets    每帧独立 packet 数
-ecc            单帧 Reed-Solomon ECC 百分比，默认 3
-f, -fps        播放或截图帧率，默认 120
-r, -R          屏幕区域，格式 SCREEN、X:Y 或 SCREEN:X:Y
-png            使用 PNG 帧模式；默认是屏幕模式
-backend        symbols 或 qr，默认 symbols
-no-zstd        关闭默认 zstd 压缩
-symbols        外部 symbol PNG 目录；为空时使用内置符号
```

区域参数：

- `-r 0`：屏幕 0，默认右下角
- `-r 1`：屏幕 1，默认右下角
- `-r 1:c:c`：屏幕 1 居中
- `-r c:c`：屏幕 0 居中
- `-r 1:100:200`：屏幕 1 的 `(100, 200)`

查看屏幕编号：

```bash
./bin/encoder -list-displays
./bin/decoder -list-displays
```

## Cell 和 Tile

默认 `-c 4t4s8c` 等价于：

```text
-tile 4x4 -shape-bits 4 -color-bits 8
```

`-cell` 语法：

- `4t` 表示 `-tile 4x4`
- `4s` 表示 `-shape-bits 4`
- `8c` 表示 `-color-bits 8`

生成新符号：

```bash
./bin/tilegen -tile 8x8 -shape-bits 6 -o generated-tiles/8x8_6bit_custom -seed 123 -attempts 20000
```

使用外部符号目录时，encoder 和 decoder 必须指定相同 `-symbols`、`-tile` 和 `-shape-bits`。

## decode 输出

decoder 每秒追加一行日志，方便保存后分析：

```text
fields: cap=capture fps, dec=cell decode fps, pkt v/r/u=valid/repeat/useful packet fps, bad=invalid packet fps, spd=current KB/s, ema=smoothed KB/s
```

含义：

- `cap`：截图 FPS
- `dec`：完成 cell decode 的 FPS
- `pkt v/r/u`：有效 packet / 重复 packet / 实际提升喷泉码 rank 的 useful packet
- `bad`：CRC、ECC、参数或解析失败的 packet
- `spd`：当前窗口速度
- `ema`：近期滑动速度

完成后 decoder 会输出 summary，总时间从首个有效帧开始计算，不包含等待 encoder 启动的时间。

## 自动测试

```bash
go test ./...
```

测试覆盖 PNG 端到端、QR backend PNG 往返、ECC、packet CRC、喷泉码、zstd/MD5、动态 tile 符号集和 screen frame source。

## 当前限制

- 当前喷泉码是线性 XOR 方案，decoder 需要通过每帧中的 transfer size 推导 blockCount。
- `Q/RQ`、`B`、`-ecc`、`-c`、`-packets`、`-backend` 等参数是运行时约定，不写入每帧数据。
- `-backend qr` 用于对比，不是当前最高吞吐路径；最高吞吐路径仍是 symbols backend。
- 非 Windows 平台 encoder screen 模式使用 HTTP/浏览器 fallback。

## License

MIT
