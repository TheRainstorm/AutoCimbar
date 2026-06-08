# Decoder Pipeline and Windows Timer Analysis

本文说明当前 screen decoder 的实际执行流程，以及 `cap`、`dec`、`valid/useful` 等指标分别表示什么。结论先行：

- `runScreenCaptureLoop` 中的 timer 只直接影响截图调度，也就是 `cap`。
- `dec` 是 cell decode 成功的帧率，受截图供给速率和单帧解码耗时共同限制。
- 如果 `cap` 已经能稳定到 120 fps，则当前场景没有被 Windows 默认 15.6 ms timer 精度卡在 64 fps。
- 如果目标是 `-f 160`，但 `cap` 在无明显截图耗时瓶颈时卡在约 60-70 fps，才需要优先怀疑 timer resolution。
- `cap` 高不代表 `dec` 一定高；`dec` 还会被 tile/cell 解码成本、颜色位数、shape 位数、RQ 区域大小、decode worker 数量、内存复制和 packet 校验成本影响。

## 相关代码位置

- 主入口：`pkg/app/screen.go` 的 `DecodeScreenToPath`
- 截图循环：`runScreenCaptureLoop`
- 解码 worker：`runScreenDecodeWorkers`
- 单帧 cell decode：`decodeCapturedFrame`
- 进度统计：`pkg/app/progress.go` 的 `screenDecoderProgress`

## 指标定义

进度行目前打印：

```text
cap=... dec=... pkt v/r/u=.../.../... bad=... spd=... ema=...
```

这些指标不是同一个阶段的不同名字，而是 pipeline 中不同计数器的窗口速率。

| 指标 | 计数位置 | 含义 |
| --- | --- | --- |
| `cap` | `progress.noteCaptured()` | `capturer.CaptureFrame` 成功返回后的截图帧率 |
| `dec` | `progress.noteDecoded()` | `decodeCapturedFrame` 成功完成 cell decode 的帧率 |
| `bad` | `progress.noteInvalid()` | cell decode 失败、ECC/CRC/packet parse 失败等无效 packet/帧的速率 |
| `pkt valid` | `progress.noteValid(...)` | packet 通过 ECC/CRC/header 解析后的速率 |
| `pkt repeat` | `noteValid(... duplicate=true)` | frame id 已接收过的重复 packet 速率 |
| `pkt useful` | `noteValid(... added=true)` | fountain rank 实际增长的 packet 速率 |
| `spd` | `rank * blockSize` 的窗口增量 | 最近窗口内恢复出的源数据速度 |
| `ema` | `spd` 的指数滑动平均 | 平滑后的近期速度 |

因此：

- `cap` 看的是“截图是否拿到了图像”。
- `dec` 看的是“截图图像是否成功识别成一帧 codec payload”。
- `valid/useful` 看的是“payload 是否能通过 packet/ECC/CRC，并对 fountain 恢复有贡献”。

## 当前伪代码

### DecodeScreenToPath 初始化

```go
func DecodeScreenToPath(cfg) {
    normalize fps, backend, cell spec, color bits, packets

    frameCapacity = GridCapacityBytes(...)
    payloadCapacity = PayloadCapacityBytesWithECCAndPackets(...)
    blockSize = payloadCapacity

    decoders = make N frameDecoder
    capturer = newScreenCapturer(rect, captureBackend)

    interval = time.Second / fps

    frames = chan capturedScreenFrame, size=N
    decodedFrames = chan decodedScreenFrame, size=N
    freeBuffers = chan []byte

    go runScreenCaptureLoop(...)
    go runScreenDecodeWorkers(...)

    for {
        decoded := <-decodedFrames

        if decoded.Err != nil {
            continue
        }

        for each packet in decoded frame {
            packet = packetCodec.DecodeInto(...) // ECC/CRC
            frame = ParsePacket(packet)

            if first valid frame {
                create fountain decoder from fileSize/blockSize
            }

            if frameID is duplicate {
                note repeat
                continue
            }

            added = fountainDec.AddFrame(frameID, payload)
            note valid/useful

            if fountainDec.Complete() {
                result = fountainDec.Decode()
                WriteSourceDataToPath(result)
                return
            }
        }
    }
}
```

### 截图循环

```go
func runScreenCaptureLoop(interval) {
    nextCapture := time.Now()

    for {
        wait while paused

        now := time.Now()
        if now.Before(nextCapture) {
            wait time.Until(nextCapture)       // time.After
        } else if now.Sub(nextCapture) > interval {
            nextCapture = now                 // 严重落后时重置节奏
        }

        nextCapture += interval

        buf = try reuse free buffer
        frame = capturer.CaptureFrame(buf)    // GDI BitBlt 或 DXGI capture/crop
        noteCaptured()                        // cap 在这里加 1

        if debug enabled {
            save png
        }

        try send frame to frames channel
        if channel full {
            drop old frame and keep latest
        }
    }
}
```

这里有两个重要点：

1. `cap` 在 `CaptureFrame` 成功后才增加，所以截图 API 本身慢也会降低 `cap`。
2. `frames` channel 满时会丢旧帧，这保证 decoder 跟不上时优先处理最新帧，但也意味着 `cap` 可以高于 `dec`。

### 解码 worker

```go
func runScreenDecodeWorkers(decoders) {
    for each decoder {
        go func() {
            for {
                captured := <-frames

                encodedFrame, err := decodeCapturedFrame(dec, captured, frameBuf)
                recycle capture buffer

                if err != nil {
                    noteInvalid()
                    decoded <- {Err: err}
                    continue
                }

                noteDecoded()                 // dec 在这里加 1
                decoded <- copy(encodedFrame)
            }
        }()
    }
}
```

`dec` 的上限是：

```text
min(capture 供给速率, 所有 decode workers 的处理能力, decodedFrames 消费能力)
```

所以你的理解是对的：`dec` 一方面受截图速率影响，另一方面受解码时间影响。除此之外还有通道背压、内存复制、packet 处理等次要因素。

## Windows timer 精度到底影响哪里

`runScreenCaptureLoop` 用 `time.After(time.Until(nextCapture))` 控制下一次截图时间。

当目标 fps 是 160：

```text
interval = 1000ms / 160 = 6.25ms
```

Windows 默认 timer resolution 常见约 15.6 ms。如果进程没有通过 `timeBeginPeriod(1)` 或类似方式提高系统 timer resolution，那么一个“睡 6.25 ms”的请求可能被实际唤醒为约 15.6 ms。

这会直接影响：

```text
截图调度频率 -> CaptureFrame 调用频率 -> cap
```

它不会直接影响：

```text
单次 decodeCapturedFrame 的 CPU 耗时
```

但是因为 decode worker 没有截图输入就不能工作，所以当 `cap` 被 timer 卡住时，`dec` 也会间接受限：

```text
timer 粒度太粗 -> cap 上不去 -> dec 没有足够输入 -> dec 也上不去
```

这类表现通常是：

```text
cap ~= dec ~= 60-70 fps
```

并且 CPU 并不满、DXGI/GDI 单帧截图耗时也没有达到瓶颈。

## 为什么你能看到 cap 到 120，没有被限制在 64

如果实测 `cap` 已经能稳定到 120 fps，则说明当前运行环境下至少不符合“timer 固定 15.6 ms 且每次都严格睡满”的最坏情况。可能原因包括：

1. 进程或系统中已有其它组件提高了 timer resolution。
2. Windows 版本、Go runtime、硬件/电源策略让 timer coalescing 表现不是固定 15.6 ms。
3. 截图循环在某些情况下没有每帧都睡满 interval，例如落后后会重置 `nextCapture = now`。
4. DXGI capture 有时可能被桌面刷新/AcquireNextFrame 行为驱动，而不是只由 Go timer 决定。

因此，timer resolution 是一个潜在上限问题，不是解释所有低 fps 的唯一原因。判断它是否是当前瓶颈，需要看：

```text
目标 fps 高于 120/160
cap 是否贴近 64 fps
单帧 CaptureFrame 是否明显小于 interval
decode workers 是否空闲或 dec ~= cap
```

如果 `cap=120` 且目标就是 `-f 120`，timer 精度不是当前瓶颈。

## 影响 cap 的因素

`cap` 主要受这些因素影响：

1. 截图调度等待：`time.After(time.Until(nextCapture))`
2. 截图 backend：
   - GDI `BitBlt`：大区域时受 CPU/GDI 内存带宽限制明显
   - DXGI：通常更快，但受 display rotation、HDR/color management、整屏 capture 后 crop copy 影响
3. 截图区域大小：
   - `RQ` 越大，区域像素越多
   - `B` 越大，区域像素也越多
4. buffer 分配和 copy：
   - GDI/DXGI 到 BGRA buffer
   - DXGI 整屏到 region crop
5. debug capture：
   - 保存 PNG 会显著降低 cap，只用于临时调试

## 影响 dec 的因素

`dec` 主要受这些因素影响：

1. `cap` 供给速率：没有截图输入，decode worker 无法产生 `dec`。
2. `decodeCapturedFrame` 单帧成本：
   - grid cell 数量约随 `Q^2` 增长
   - tile 越小，cell 数越多
   - shape bits 越高，符号识别候选更多或判别成本更高
   - color bits 越高，颜色判决更敏感，错误率也可能上升
3. decode worker 数量：
   - 当前默认 `runtime.NumCPU()/2`，最多 4
   - worker 太少会让 `dec < cap`
   - worker 太多可能引入内存带宽和调度开销
4. 内存复制：
   - decode 成功后会 `append([]byte(nil), encodedFrame...)` 复制 payload
   - 大 `RQ`、多 packet 时 payload 更大
5. 后续主循环消费速度：
   - ECC decode
   - packet CRC/header parse
   - fountain AddFrame
   - seen frame id map 查询

注意：`bad` 过高时，`dec` 仍可能很高。因为 `dec` 只说明 cell payload 识别成功；packet 之后 CRC/ECC 失败会统计到 `bad`，不会回退 `dec`。

## 如何判断当前瓶颈

可以按下面几种形态判断：

### 1. cap 低，dec 接近 cap

```text
cap=60 dec=58
```

优先看截图侧：

- GDI 是否被大区域 BitBlt 卡住
- DXGI 是否 fallback
- timer 是否卡住高 fps 调度
- debug capture 是否开启

### 2. cap 高，dec 明显低

```text
cap=160 dec=80
```

优先看 decode 侧：

- cell decode CPU 成本
- decode workers 数量
- tile/cell 配置是否太重
- 内存带宽和复制

### 3. cap 和 dec 都高，但 useful 低

```text
cap=120 dec=118 pkt valid=80 repeat=40 useful=40 bad=0
```

优先看传输效率：

- decoder fps 高于 encoder fps，重复帧多
- fountain 有重复 frame id
- packets/frame、encoder fps、decoder fps 不匹配

### 4. cap 和 dec 高，但 bad 高

```text
cap=120 dec=115 bad=80 useful=20
```

优先看图像质量和帧格式：

- HDR/color management
- color bits 太高
- ECC 太低
- encoder/decoder cell/ecc/packet/zstd 配置不一致
- 截图区域没对齐或窗口没完整覆盖

## 对 timeBeginPeriod 的判断

给 decoder 加 `timeBeginPeriod(1)` 是合理的防御性优化，因为：

- encoder 已经做了。
- decoder 的高 fps capture pacing 确实依赖 Go timer。
- 它可以避免某些 Windows 环境下 `-f 160` 被粗 timer resolution 限制。

但它不是万能优化：

- 如果 `cap` 已经等于目标 fps，它不会提升速度。
- 如果瓶颈是 GDI BitBlt 或 DXGI crop copy，它不会提升速度。
- 如果瓶颈是 cell decode CPU，它不会提升 `dec`。
- 如果瓶颈是颜色错误导致 `bad` 高，它也不会提升 `valid/useful`。

因此应把它看作“解除截图调度上限”的修复，而不是“提升解码算法速度”的修复。

当前实现已经在 `DecodeScreenToPath` 启动时调用 timer resolution helper：Windows 下进入 `timeBeginPeriod(1)`，退出时 `timeEndPeriod(1)`；非 Windows 平台是 no-op。

## 建议后续观测项

为了更明确地区分瓶颈，decoder 增加了 `-v` 详细诊断输出。默认不启用这些耗时采样，避免在高 fps 热路径中额外调用 `time.Now()` 影响性能。

`-v` 会显示：

1. `capture_ms`：`CaptureFrame` 单次耗时的窗口平均值。
2. `decode_ms`：`decodeCapturedFrame` 单次耗时的窗口平均值。
3. `queue_drop_fps`：frames channel 满导致丢帧的速率。
4. `packet_ms`：ECC/CRC/Parse/Fountain 主循环耗时。
5. `worker_count`：当前 decode worker 数量。

有了这些指标后，可以直接判断：

```text
cap 低是 timer wait、CaptureFrame 慢，还是 queue/drop 行为导致；
dec 低是 worker 算不过来，还是主循环消费慢。
```
