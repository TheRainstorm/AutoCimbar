# DXGI Screenshot Backend Analysis

## 背景

decoder 在高 `RQ` 下的主要瓶颈已经从 cell decode 变成屏幕截图：

- `RQ=80` 时 capture 约 `120-150 fps`
- `RQ=120` 时 capture 约 `82 fps`
- `RQ=160` 时 capture 约 `55 fps`

`RQ=160` 默认 8x8 tile、`B=1` 时，截图区域约为 `1280x1280 = 1.64 Mpixel`。当前 GDI `BitBlt` 路径实测吞吐约 `109 Mpixel/s`，所以单帧读回耗时约 `15 ms`，理论上限约 `54 fps`，与实测 `55 fps` 基本一致。

这说明继续优化 timer 精度、decode worker 数量或日志输出无法突破 `RQ=160` 的 capture 上限。

## 方案选择

Windows 8+ 提供 DXGI Desktop Duplication：

- 通过 `IDXGIOutputDuplication` 从 GPU 桌面纹理获取帧。
- OBS、ShareX、Windows 录屏等常见录屏/截图工具都采用同类路径。
- 相比 GDI `BitBlt`，它避开了传统 CPU GDI 截图路径，适合高分辨率、高 fps 的连续捕获。

本项目接入本地第三方库：

```text
third-party/screenshot
```

该库提供：

- GDI 和 DXGI 两种 capture provider。
- 多显示器枚举。
- DXGI Desktop Duplication 封装。
- BGRA/RGBA swizzle 辅助。

## 集成方式

### 统一显示器编号

为了保证 encoder 和 decoder 的 `-r SCREEN:X:Y` 编号一致，项目不再让 decoder 单独使用一套显示器枚举。

现在 `pkg/app` 内部通过平台函数统一显示器列表：

- Windows: `third-party/screenshot` 的 `NumActiveDisplays` / `GetDisplayBounds`
- 非 Windows: 原 `github.com/kbinani/screenshot`

`DisplayBounds()`、`ResolveEncoderRegion()` 和 `ResolveDecoderRegion()` 都走同一套函数，因此 encoder 和 decoder 的屏幕编号保持一致。

### Windows capture backend

`pkg/app/capture_windows.go` 现在按如下顺序初始化：

1. 根据 decoder region 找到其所属 display。
2. 优先初始化 `ProviderDXGI`。
3. 如果 DXGI 初始化失败，自动回退到原 GDI `BitBlt` capturer。

DXGI backend 按 display 捕获整屏，再裁剪出 decoder region。这样可以保留现有 `-r SCREEN:X:Y` 语义，同时避免重写 region 解析和 decode pipeline。

### BGRA 直解

原有 decoder 已支持 BGRA 直解：

```go
DecodeBGRAInto(pix, width, height, stride, dst)
```

因此本次补强了 `third-party/screenshot` 的 `CaptureBGRA()`：

- 直接从 DXGI mapped surface 拷贝 `DXGI_FORMAT_B8G8R8A8_UNORM` 数据。
- 不再走颜色语义不明确的 `Capture()`。
- decoder 继续沿用 BGRA path，避免额外 RGBA 转换。

### 静止画面处理

DXGI Desktop Duplication 只在桌面有新帧时返回图像；画面静止时可能返回 `ErrNoImageYet`。

decoder 需要在 encoder 尚未启动时持续等待，因此 Windows DXGI capturer 做了兼容：

- 有上一帧时复用上一帧。
- 首次无帧时返回一帧空 BGRA 数据，让 decoder 继续等待，而不是退出。

## 当前预期

这次改动消除了 GDI `BitBlt` 的硬性吞吐上限，`RQ=160` 不再应该被 `~55 fps` 卡死。实际速度仍取决于：

- GPU 桌面复制和 CPU readback 成本。
- Moonlight/串流画面更新频率。
- display 实际刷新率。
- decoder region 大小。
- DXGI 整屏捕获后裁剪的内存 copy 成本。

## 后续优化点

1. 暴露 DXGI mapped rect 的 region view，减少整屏到 region 的中间 copy。
2. 用 `CopySubresourceRegion` 只从 GPU 复制 decoder region，而不是整屏。
3. 在 capture loop 中区分 `no image yet`，这类重复帧可以不进入 decode worker。
4. 记录 capture backend 名称和 per-frame capture cost，方便对比 GDI/DXGI。
5. 对高 fps 场景考虑固定大小 buffer pool，减少 DXGI crop 的内存带宽和 GC 压力。
