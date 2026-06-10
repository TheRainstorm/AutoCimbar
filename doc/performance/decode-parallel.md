# Decoder Cell Decode 并行优化记录

日期：2026-06-08

## 目标

`decoder` 在高 `RQ` 下主要瓶颈已经从截图转为 cell decode。单帧中每个 cell 的 shape/color 判决相互独立，适合按 cell 区间并行。

这次实现只保留通用优化，不绑定 `4t4s8c` 或其它特定 cell 格式。

## 实现

- `decodeBGRAInto` 和 `decodeRGBAInto` 按 cell 区间并行。
- 小帧（`numCells < 1600`）仍走单线程，避免 goroutine 调度开销。
- worker 数取 `GOMAXPROCS`，上限 8。
- 每个 worker 使用独立的 64-byte sample buffer，避免共享 `Decoder.sampleBuf`。
- 分片边界按 `cellBits` 对齐到 byte 边界，避免 bit-packed 输出中相邻 worker 写同一个 byte。

这个分片边界是对原始 `fe1855` 思路的修正：直接按 cell 数均分在 6bit/12bit 这类格式下可能跨 byte 写入，存在数据竞争和偶发错包风险。

## 验证

```bash
go test ./pkg/codec
go test -race ./pkg/codec
go test ./pkg/app ./pkg/color ./pkg/symbol
GOOS=windows GOARCH=amd64 go build -trimpath -o bin/decoder.exe ./cmd/decoder
```

## Benchmark

机器：AMD Ryzen 5 5600G，Linux amd64，Go benchmark 默认 `GOMAXPROCS=12`。

### 默认 full-frame benchmark

`BenchmarkDecodeBGRAIntoFullFrame`，默认 `8t4s2c`、`Q=50`：

| 版本 | ns/op |
| --- | ---: |
| 优化前 | 832,332 |
| 优化后 | 292,516 |
| 加速比 | 2.85x |

优化后单次波动较大，5 次结果范围约 `251,660 - 366,438 ns/op`，低延迟样本对应约 `3.31x`。

### 多 cell 格式 benchmark

新增 `BenchmarkDecodeBGRAIntoCellFormats`，覆盖不同 tile/color 配置：

| cell | grid | 优化前 ns/op | 优化后 ns/op | 加速比 |
| --- | ---: | ---: | ---: | ---: |
| `8t4s2c` | 120 | 4,719,417 | 1,195,306 | 3.95x |
| `6t4s4c` | 120 | 3,153,855 | 769,374 | 4.10x |
| `4t4s8c` | 240 | 4,770,798 | 1,095,133 | 4.36x |

结论：收益不是 `4t4s8c` 专用优化，而是通用的 cell decode 并行化。

## 后续方向

- 如果 app 层 `decode-workers` 与 codec 内部 worker 叠加造成过度并发，可以后续增加 codec 内部并行开关或 worker 上限配置。
- 下一步热点仍会集中在 `cellHash*` 和 `recognizeCellColor*`，但应优先做通用数据结构优化，避免重新引入特定 cell 格式专用分支。
