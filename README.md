# AutoCamBar

AutoCamBar 是一个面向远程桌面截屏场景的文件传输工具。发送端把文件编码成彩色符号二维码帧，接收端从 PNG 帧目录或指定屏幕区域解码并恢复文件。

当前实现使用 Go：

- 4 色 × 16 符号，每个 cell 编码 6 bits
- 默认内置 16 个 libcimbar bitmap 符号，Windows 上单个 exe 可直接运行
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
```

### Windows PowerShell

```powershell
go test ./...
go build -o bin\encoder.exe .\cmd\encoder
go build -o bin\decoder.exe .\cmd\decoder
```

### 交叉编译 Windows 程序

在 Linux/macOS 上交叉编译：

```bash
GOOS=windows GOARCH=amd64 go build -o bin/encoder.exe ./cmd/encoder
GOOS=windows GOARCH=amd64 go build -o bin/decoder.exe ./cmd/decoder
```

## 快速验证：PNG 帧模式

PNG 帧模式不需要真实屏幕，适合先确认程序能完整恢复文件。

发送端生成帧：

```bash
./bin/encoder -i input.bin -o frames -Q 50 -B 1 -redundancy 100
```

接收端恢复文件：

```bash
./bin/decoder -i frames -o output.bin -Q 50 -B 1
```

Windows PowerShell：

```powershell
.\bin\encoder.exe -i input.bin -o frames -Q 50 -B 1 -redundancy 100
.\bin\decoder.exe -i frames -o output.bin -Q 50 -B 1
```

启用单帧 ECC 时，encoder 和 decoder 必须使用相同 `-ecc`：

```bash
./bin/encoder -i input.bin -o frames -Q 80 -B 1 -redundancy 100 -ecc 20
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

## Windows 屏幕传输

### 1. 发送端

发送端直接打开原生无边框置顶窗口，不需要浏览器。

如果有多块屏幕，先查看程序识别到的屏幕索引和坐标：

```powershell
.\bin\encoder.exe -list-displays
```

```powershell
.\bin\encoder.exe -screen -i input.bin -Q 50 -B 1 -R 0:-0:-0 -fps 30
```

参数说明：

- `-screen`：使用屏幕播放模式，不写 PNG 文件
- `-R X:Y` 或 `-R SCREEN:X:Y`：播放窗口位置；省略 `SCREEN` 时默认主屏 0
- 负数从该屏幕右/下边缘定位；`0:-0:-0` 表示主屏右下角贴边
- `-fps`：窗口刷新帧率
- `-block-size`：可选，喷泉码 block 大小；默认使用当前 `Q` 可承载的最大 payload

按 `Esc` 可以关闭发送窗口。非 Windows 平台当前仍使用 HTTP/浏览器 fallback。

### 2. 接收端

接收端从指定屏幕区域截图并解码：

```powershell
.\bin\decoder.exe -screen -o output.bin -Q 50 -B 1 -R 0:-0:-0 -fps 30 -timeout 5m
```

参数说明：

- `-screen`：使用截图解码模式
- `-R SCREEN:X:Y`：截图区域
- `SCREEN`：屏幕编号，从 0 开始
- `X:Y`：截图区域左上角；负数从该屏幕右/下边缘定位
- `-timeout`：等待足够喷泉帧的最长时间

也可以用 decoder 查看屏幕索引：

```powershell
.\bin\decoder.exe -list-displays
```

示例：

- `-R 0:0:0`：主屏左上角
- `-R 0:-0:-0`：主屏右下角
- `-R 1:100:200`：第 2 块屏幕，偏移 `(100, 200)`

两端必须使用一致的 `Q`、`B` 和 `block-size`。文件大小会在当前线性喷泉码帧头中发送，decoder 不需要手动指定。

## 参数参考

### encoder

```text
-i             输入文件
-o             PNG 帧输出目录，默认 frames
-Q             grid cell 数，默认 50
-B             cell 缩放倍数，实际 cell 像素为 8 * B
-redundancy    PNG 模式额外冗余帧百分比，默认 10
-block-size    喷泉码 block 大小，0 表示使用最大 payload
-ecc           单帧 Reed-Solomon ECC 百分比，默认 0；必须与 decoder 一致
-screen        启用屏幕播放模式
-R             屏幕播放位置，格式 X:Y 或 SCREEN:X:Y
-fps           屏幕播放帧率
-addr          非 Windows HTTP fallback 的播放器地址
-open          非 Windows HTTP fallback 是否自动打开浏览器
-list-displays 列出程序识别到的屏幕索引和坐标
-symbols       可选 libcimbar bitmap 符号目录；为空时使用内置符号
-list-displays 列出程序识别到的屏幕索引和坐标
```

### decoder

```text
-i             PNG 帧输入文件或目录，默认 frames
-o             输出文件，默认 decoded.out
-Q             grid cell 数，必须与 encoder 一致
-B             cell 缩放倍数，必须与 encoder 一致
-block-size    喷泉码 block 大小，必须与 encoder 一致
-ecc           单帧 Reed-Solomon ECC 百分比，默认 0；必须与 encoder 一致
-screen        启用截图解码模式
-R             截图区域，格式 SCREEN:X:Y
-fps           截图频率
-timeout       截图解码超时
-symbols       可选 libcimbar bitmap 符号目录；为空时使用内置符号
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
- 启用 ECC 的 PNG 帧端到端恢复
- 删除部分帧后的喷泉码恢复
- 屏幕帧输出的可解码性

## 当前限制

- 当前喷泉码是纯 Go 线性 XOR 喷泉码，decoder 需要知道源块数量；实现方式是在每帧头携带文件大小，用于推导 `blockCount`。
- 每帧头当前包含 `fileSize(8 bytes) + frameID(4 bytes)`，其余为喷泉编码块。
- `-ecc` 是运行时约定参数，不写入帧头；开启后会减少每帧 fountain payload，并在单帧内添加交织后的 RS 校验字节。
- Windows 屏幕模式使用 Win32 原生窗口；非 Windows 平台暂时使用 HTTP/浏览器 fallback。
- 暂未实现 GPU 加速和真正的 Wirehair/Raptor 类喷泉码。

## 项目结构

```text
cmd/
  encoder/       编码器 CLI
  decoder/       解码器 CLI
pkg/
  app/           CLI 应用层、内置符号、PNG 帧、屏幕播放和截图
  codec/         彩色符号二维码编解码
  color/         颜色识别
  ecc/           Reed-Solomon 实验模块
  fountain/      纯 Go 线性喷泉码
  symbol/        image hash 符号识别
third-party/
  libcimbar/bitmap/4/  内置符号的来源和可选覆盖资源
```

## 许可证

MIT
