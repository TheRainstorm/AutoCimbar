# AutoCamBar

AutoCamBar 是一个面向远程桌面截屏场景的文件传输工具。发送端把文件编码成彩色符号二维码帧，接收端从 PNG 帧目录或指定屏幕区域解码并恢复文件。

当前实现使用 Go：

- 默认 4 色 × 16 个 8x8 形状符号，每个 cell 编码 6 bits；可用 `-cell`、`-color-bits` 和 `-shape-bits` 调整 cell 规格
- 常用 `generated-tiles/` 符号集已编译进程序，Windows 上单个 exe 可直接运行；仍可用 `-symbols` 覆盖为外部符号目录
- 原始文件传输前默认使用 zstd 压缩，恢复时流式解压并校验 MD5；可用 `-no-zstd` 关闭
- 纯 Go 线性喷泉码，支持冗余帧和丢帧恢复
- PNG 帧模式：便于测试和离线验证
- 屏幕模式：Windows 下 encoder 直接打开原生无边框置顶窗口，decoder 截图指定区域恢复文件

## 环境要求

- Go 1.24 或更高版本；当前开发验证使用 Go 1.26.4
- Windows 运行时建议使用 PowerShell
- 屏幕截图功能依赖 `github.com/kbinani/screenshot`
- 默认不需要外部 symbol 文件；只有使用 `-symbols` 覆盖内置符号时才需要提供 PNG 目录

如果本机安装了新版 Go 到 `/usr/local/go`，Linux/macOS 下可这样使用：

```bash
export PATH=/usr/local/go/bin:$PATH
go version
```

## 编译

### Linux/macOS

```bash
go test ./...
go build -o bin/encoder ./cmd/encoder
go build -o bin/decoder ./cmd/decoder
go build -o bin/tilegen ./cmd/tilegen
```

### Windows PowerShell

```powershell
go test ./...
go build -o bin\encoder.exe .\cmd\encoder
go build -o bin\decoder.exe .\cmd\decoder
go build -o bin\tilegen.exe .\cmd\tilegen
```

### 交叉编译 Windows 程序

在 Linux/macOS 上交叉编译：

```bash
GOOS=windows GOARCH=amd64 go build -o bin/encoder.exe ./cmd/encoder
GOOS=windows GOARCH=amd64 go build -o bin/decoder.exe ./cmd/decoder
GOOS=windows GOARCH=amd64 go build -o bin/tilegen.exe ./cmd/tilegen
```

## Tile 符号规格

默认 `-tile 8x8 -shape-bits 4` 使用内置 libcimbar 16 符号，不需要 `-symbols`。下列 `generated-tiles/` 规格也已编译进程序，运行时只需要指定 `-tile/-shape-bits` 或 `-cell`，不需要分发符号目录。

仓库提供了几组已生成符号，可直接用于实验：

```text
generated-tiles/8x8_5bit   32 symbols, min hamming distance 22
generated-tiles/8x8_6bit   64 symbols, min hamming distance 20
generated-tiles/6x6_4bit   16 symbols, min hamming distance 12
generated-tiles/4x4_3bit    8 symbols, min hamming distance 6
generated-tiles/4x4_2bit    4 symbols, min hamming distance 8
generated-tiles/4x4_4bit   16 symbols, min hamming distance 4
```

使用示例：

```bash
./bin/encoder -png -i input.bin -o frames -tile 8x8 -shape-bits 5
./bin/decoder -png -i frames -o output.bin -tile 8x8 -shape-bits 5
```

生成新的符号集合：

```bash
./bin/tilegen -tile 8x8 -shape-bits 6 -o generated-tiles/8x8_6bit_custom -seed 123 -attempts 20000
```

输出目录包含 `00.png..` 和 `manifest.json`。encoder 和 decoder 必须使用相同的 `-tile`、`-shape-bits` 和 `-symbols`，这些参数不写入每帧数据。`-symbols` 非空时优先使用外部目录。

`-cell` 是常用 cell 参数的紧凑写法，格式为 `数字+字母` 的组合：

- `t`：tile 宽高，例如 `4t` 表示 `-tile 4x4`
- `c`：color bits，例如 `7c` 表示 `-color-bits 7`
- `s`：shape bits，例如 `4s` 表示 `-shape-bits 4`

示例：`-cell 4t7c4s` 等价于 `-tile 4x4 -color-bits 7 -shape-bits 4`。

如果希望不同 tile size 下显示面积大致一致，可以用 `-RQ` 代替手工换算 `-Q`。`-RQ` 表示以 `8x8` tile 为基准的参考 Q，程序会按 `actual_Q = ceil(RQ * 8 / tile_width)` 自动计算实际 Q。例如 `-RQ 120 -tile 4x4` 会使用实际 `Q=240`，显示宽度仍约等于 `120 * 8 * B` 像素。

## 快速验证：PNG 帧模式

PNG 帧模式不需要真实屏幕，适合先确认程序能完整恢复文件。

发送端生成帧：

```bash
./bin/encoder -i input.bin -o frames
```

接收端恢复文件：

```bash
./bin/decoder -i frames -o output.bin
```

Windows PowerShell：

```powershell
.\bin\encoder.exe -i input.bin -o frames
.\bin\decoder.exe -i frames -o output.bin
```

启用单帧 ECC 时，encoder 和 decoder 必须使用相同 `-ecc`：

```bash
./bin/encoder -i input.bin -o frames -Q 80 -B 1 -ecc 20
./bin/decoder -i frames -o output.bin -Q 80 -B 1 -ecc 20
```

校验：

```bash
sha256sum input.bin output.bin
```

PowerShell：

```powershell
Get-FileHash .\input.bin
Get-FileHash .\output.bin
```

decoder 也会在喷泉码恢复完成后校验内置的文件级 MD5；校验失败会报错，不会静默接受错误输出。

## Windows 屏幕传输

### 1. 发送端

发送端直接打开原生无边框置顶窗口，不需要浏览器。

如果有多块屏幕，先查看程序识别到的屏幕索引和坐标：

```powershell
.\bin\encoder.exe -list-displays
```

```powershell
.\bin\encoder.exe -i input.bin
```

参数说明：

- 默认使用屏幕播放模式，不写 PNG 文件；需要 PNG 模式时使用 `-png`
- `-R SCREEN`、`-R X:Y` 或 `-R SCREEN:X:Y`：播放窗口位置；省略 `SCREEN` 时默认主屏 0
- `SCREEN` 单独出现时默认右下角；负数从该屏幕右/下边缘定位；`c` 表示该轴居中
- `-fps`：窗口刷新帧率
- `-color-bits`：颜色通道位数，`0..8` 对应 1..256 色；encoder 和 decoder 必须一致
- `-shape-bits`：形状通道位数；默认 4，5/6 需要使用匹配的 `-symbols` 目录
- `-tile`：逻辑符号 tile 尺寸，如 `8x8`、`6x6`、`4x4`；encoder 和 decoder 必须一致
- `-symbols`：符号目录，文件名为 `00.png`、`01.png` ...；默认空值仅支持内置 `8x8/4bit`
- `-packets`：每张屏幕图携带的独立 packet 数，默认 1；提高后局部错误只丢对应 packet，encoder 和 decoder 必须一致
- `-no-zstd`：关闭默认 zstd 压缩，直接传输原始文件数据

按 `Esc` 可以关闭发送窗口。非 Windows 平台当前仍使用 HTTP/浏览器 fallback。

### 2. 接收端

接收端从指定屏幕区域截图并解码：

```powershell
.\bin\decoder.exe -o output.bin -timeout 5m
```

参数说明：

- 默认使用截图解码模式；需要 PNG 模式时使用 `-png`
- `-R SCREEN`、`-R X:Y` 或 `-R SCREEN:X:Y`：截图区域
- `SCREEN`：屏幕编号，从 0 开始
- `X:Y`：截图区域左上角；负数从该屏幕右/下边缘定位
- `-timeout`：等待足够喷泉帧的最长时间

也可以用 decoder 查看屏幕索引：

```powershell
.\bin\decoder.exe -list-displays
```

示例：

- `-R 0:0:0`：主屏左上角
- `-R 0` 或 `-R 0:-0:-0`：主屏右下角
- `-R c:c`：主屏居中
- `-R 1:c:c`：第 2 块屏幕居中
- `-R 1:100:200`：第 2 块屏幕，偏移 `(100, 200)`

两端必须使用一致的 `Q`、`B`、`-ecc`、`-color-bits`、`-shape-bits`、`-tile`、`-symbols` 和 `-packets`。文件大小会在当前线性喷泉码帧头中发送，decoder 不需要手动指定。

decoder 进度中的 `cap`、`dec`、`pkt v/r/u`、`bad`、`spd`、`ema` 含义：

- `cap`：截图帧率
- `dec`：完成 cell decode 的帧率
- `pkt v/r/u`：有效 packet / 重复 packet / 让喷泉码 rank 增加的 useful packet 速率
- `bad`：截图已解码但 packet 校验失败、参数不匹配或内容错误的速率
- `spd`：当前窗口瞬时速度
- `ema`：近期滑动平滑速度，使用 EMA，避免等待首帧时间拉低显示值
- decoder 每秒追加一行进度日志，便于保存后分析，不会覆盖清除上一秒记录
- `-decode-workers` 可开启/调整并行 cell decode；默认自动选择，设为 `1` 可回到串行 decode
- 完成后会打印 `summary`，其中总时间从首个有效帧开始计算，不包含等待 encoder 启动的时间

## 参数参考

### encoder

```text
-i             输入文件
-o             PNG 帧输出目录，默认 frames
-Q             grid cell 数，默认 120
-RQ            以 8x8 tile 为基准的参考 Q；设置后自动按 tile 宽度换算实际 Q
-B             cell 缩放倍数，实际 cell 像素为 tile_width * B
-ecc           单帧 Reed-Solomon ECC 百分比，默认 3；必须与 decoder 一致
-color-bits    颜色通道位数，0..8 表示 1..256 色；默认 2
-shape-bits    形状通道位数，默认 4；必须与 decoder 一致
-tile          逻辑符号 tile 尺寸，默认 8x8；必须与 decoder 一致
-cell          紧凑 cell 规格，如 4t7c4s
-c             -cell 的短参数
-packets       屏幕模式每张图携带的独立 packet 数，默认 1；必须与 decoder 一致
-p             -packets 的短参数
-no-zstd       关闭默认 zstd 压缩
-png           启用 PNG 帧模式；默认是屏幕播放模式
-R             屏幕播放位置，格式 SCREEN、X:Y 或 SCREEN:X:Y；c 表示居中
-r             -R 的短参数
-fps           屏幕播放帧率，默认 120
-f             -fps 的短参数
-addr          非 Windows HTTP fallback 的播放器地址
-open          非 Windows HTTP fallback 是否自动打开浏览器
-list-displays 列出程序识别到的屏幕索引和坐标
-symbols       可选符号目录；为空时使用内置符号
-s             -symbols 的短参数
-list-displays 列出程序识别到的屏幕索引和坐标
```

### decoder

```text
-i             PNG 帧输入文件或目录，默认 frames
-o             输出文件，默认 decoded.out
-Q             grid cell 数，默认 120；必须与 encoder 一致
-RQ            以 8x8 tile 为基准的参考 Q；设置后自动按 tile 宽度换算实际 Q
-B             cell 缩放倍数，必须与 encoder 一致
-ecc           单帧 Reed-Solomon ECC 百分比，默认 3；必须与 encoder 一致
-color-bits    颜色通道位数，0..8 表示 1..256 色；默认 2
-shape-bits    形状通道位数，默认 4；必须与 encoder 一致
-tile          逻辑符号 tile 尺寸，默认 8x8；必须与 encoder 一致
-cell          紧凑 cell 规格，如 4t7c4s
-c             -cell 的短参数
-packets       屏幕模式每张图携带的独立 packet 数，默认 1；必须与 encoder 一致
-p             -packets 的短参数
-png           启用 PNG 帧模式；默认是截图解码模式
-R             截图区域，格式 SCREEN、X:Y 或 SCREEN:X:Y；c 表示居中
-r             -R 的短参数
-fps           截图频率，默认 120
-f             -fps 的短参数
-decode-workers 并行截图 decode worker 数，0 表示自动选择
-timeout       截图解码超时
-symbols       可选符号目录；为空时使用内置符号
-s             -symbols 的短参数
```

## 自动测试

```bash
go test ./...
```

测试覆盖：

- image hash 和符号识别
- 颜色识别
- codec 往返
- Reed-Solomon 模块
- 单帧 ECC 字节错误修复
- 喷泉码恢复
- PNG 帧端到端恢复
- 8x8 5/6bit、6x6 4bit、4x4 2/3/4bit 动态 tile 符号集 PNG 端到端恢复
- 启用 ECC 的 PNG 帧端到端恢复
- 删除部分帧后的喷泉码恢复
- zstd 源数据压缩、流式解压和 MD5 校验
- 屏幕帧输出的可解码性

## 当前限制

- 当前喷泉码是纯 Go 线性 XOR 喷泉码，decoder 需要知道源块数量；实现方式是在每帧头携带文件大小，用于推导 `blockCount`。
- 每个 packet 头当前包含 `fileSize(8 bytes) + frameID(4 bytes) + crc32(4 bytes)`，其余为喷泉编码块；`fileSize` 指喷泉传输数据大小，`crc32` 用于尽早丢弃错误 packet。
- 原始文件默认会先进行 zstd 压缩，再加一个一次性源数据头，包含原始文件大小、压缩方式和 MD5，用于最终完整性校验；它不是每帧参数。使用 `-no-zstd` 时源数据头会标记为未压缩。
- `-ecc`、`-color-bits`、`-shape-bits`、`-tile`、`-symbols` 和 `-packets` 是运行时约定参数，不写入帧头；开启 ECC 后会减少每个 packet 的 fountain payload，并在 packet 内添加交织后的 RS 校验字节。
- Windows 屏幕模式使用 Win32 原生窗口；非 Windows 平台暂时使用 HTTP/浏览器 fallback。
- 暂未实现 GPU 加速和真正的 Wirehair/Raptor 类喷泉码。

## 项目结构

```text
cmd/
  encoder/       编码器 CLI
  decoder/       解码器 CLI
  tilegen/       tile 符号搜索/导出工具
generated-tiles/ 已生成的实验符号集
pkg/
  app/           CLI 应用层、内置符号、PNG 帧、屏幕播放和截图
  codec/         彩色符号二维码编解码
  color/         颜色识别
  ecc/           Reed-Solomon 实验模块
  fountain/      纯 Go 线性喷泉码
  symbol/        image hash 符号识别
  tilegen/       tile 搜索生成逻辑
third-party/
  libcimbar/bitmap/4/  内置符号的来源和可选覆盖资源
```

## 许可证

MIT
