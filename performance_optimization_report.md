# AutoCambar 性能优化报告

本文总结近期为屏幕单向传输链路做的性能和可靠性优化，重点解释 encoder 从达不到 60 fps 到可以冲高帧率、decoder 截屏/解码帧率提升，以及最终吞吐受 `valid_fps`、ECC、颜色通道、`-packets` 等因素影响的原因。

## 1. 总体瓶颈变化

早期链路的主要瓶颈在屏幕 encoder：

- encoder 屏幕刷新和编码耦合，受 Windows 消息/定时器限制。
- 每帧重复分配 block、packet、图像缓冲。
- 每个 cell 渲染时反复做模板/颜色组合。
- Windows 显示路径存在额外拷贝和等待。

优化后，encoder 不再是主要瓶颈。用户实测中，`-fps 120` 时 encoder 已可稳定接近 120 fps；继续提高目标帧率时可以轻松超过原先 60 fps 上限。之后主要瓶颈转移到 decoder 的截图帧率、视频压缩后单帧有效率，以及大 `Q` 下整帧失败概率。

## 2. Encoder 为什么从不到 60 fps 到可冲高帧率

### 2.1 去掉 WM_TIMER 帧率限制

提交：`a37d4f5 perf: 避免 Windows encoder 受 WM_TIMER 限速`

Windows 原生窗口早期依赖 `WM_TIMER` 驱动刷新，实际调度精度和消息泵行为会让帧率卡在较低水平。优化后改为更主动的刷新节奏，并配合提高 timer resolution，使目标 fps 不再被默认 Windows timer 精度强行限制。

效果：encoder 从 30/60 fps 目标下只能跑到 30-40 fps，提升到可以接近目标刷新率。

### 2.2 原生窗口双缓冲显示

提交：`7a4d946 perf: Windows encoder 使用双缓冲显示`

编码和显示使用前后缓冲，减少绘制过程中读写同一 buffer 的等待和 tearing 风险。窗口绘制只消费已经完成的 buffer，编码线程可以继续准备下一帧。

效果：降低显示路径对编码路径的阻塞，刷新 fps 更接近 encode fps。

### 2.3 直接生成 BGRA，减少像素格式转换

相关实现：`pkg/codec.EncodeBGRA`、`pkg/app/screen_windows.go`

Windows GDI 显示天然使用 BGRA/DIB 方向更合适。优化后 encoder 可以直接生成 BGRA buffer，避免先生成 RGBA 再转换或复制。

效果：每帧像素处理少一次全图转换，尤其在 `Q=120/160` 这种大图下收益明显。

### 2.4 Tile cache：预渲染颜色 × 形状组合

相关实现：`pkg/codec.Encoder.tileCache`、`tileCacheBGRA`

每个 cell 只有“颜色 ID + 形状 ID”组合。当前默认 4 色是 4 × 16 = 64 种 tile，16 色是 16 × 16 = 256 种 tile。优化后初始化 encoder 时一次性构建 tile cache，运行时每个 cell 只做：

1. 从 payload 读取 cell bits。
2. 查表得到 tile。
3. 复制 tile 到目标图像位置。

效果：把每个像素的条件判断和模板映射从热路径移到初始化阶段，编码每帧变成内存顺序复制。

### 2.5 复用喷泉码、packet、图像缓冲

提交：`ebcbf39 perf: 复用屏幕编码数据缓冲`

优化包括：

- `fountain.EncodeInto` 复用 block buffer。
- `BuildPacketInto` 复用 packet buffer。
- ECC `EncodeInto` 复用 encoded packet buffer。
- `EncodeBGRA` 复用屏幕像素输出 buffer。

效果：减少高 fps 下 GC 压力和分配抖动，让 encoder 长时间运行时帧率更稳定。

### 2.6 进度指标拆分

提交：`1464f04 perf: 支持单帧多独立 packet`

encoder 进度从单一 `encode_fps` 改为：

- `frame_fps`：实际生成屏幕图的速率。
- `packet_fps`：实际生成传输 packet 的速率。
- `refresh_fps`：窗口上屏刷新速率。

这样开启 `-packets N` 后，能区分“屏幕图刷新速度”和“实际数据 packet 速度”。

## 3. Decoder fps 提升

### 3.1 Windows 截屏使用 DIB section

提交：`eeb4435 perf: Windows 截屏使用 DIB section`

早期截图路径会产生额外图像对象和拷贝。改成 DIB section 后，`BitBlt` 直接写入可访问的内存 buffer。

效果：降低 capture 端每帧开销，是 `capture_fps` 提升的基础。

### 3.2 直接解码 BGRA 截屏缓冲

提交：`23aa022 perf: Windows decoder 直接解码 BGRA 截屏缓冲`

Windows 截屏拿到的是 BGRA。优化后 decoder 不再把截图转换成 Go `image.RGBA` 后再解码，而是直接对 BGRA memory 做 cell hash 和颜色识别。

效果：

- 减少一次全图像素转换。
- 减少临时对象分配。
- 对大 `Q` 图像收益明显。

### 3.3 capture 与 decode 解耦

提交：`3e65334 feat: 支持关闭 zstd 并拆分截图统计`

截图循环独立运行，decode 消费最新帧。队列满时丢旧帧，避免 decoder 暂时变慢时阻塞截图线程。

效果：

- `capture_fps` 更真实，表示截图能力。
- `decode_fps` 表示解码处理能力。
- 当 decoder 慢于 capture 时，不积压过期帧。

### 3.4 解码输出缓冲复用

提交：`17cb5cc perf: 复用屏幕解码输出缓冲`

`DecodeInto`、`DecodeBGRAInto` 复用输出 buffer，不再每帧分配完整 codec payload。

效果：降低 GC 压力，减少持续解码时的抖动。

### 3.5 RGB 快路径、强度 hash、YCbCr 颜色识别

提交：

- `35d1b8e perf: 使用 RGB 快路径识别解码颜色`
- `c7ee570 perf: 提升视频压缩下的符号判定稳定性`
- `de26d4e perf: 使用 YCbCr 距离识别颜色`

具体变化：

- 颜色识别热路径从通用 `image.At()`/LAB 距离切到直接 RGB/YCbCr 数值路径。
- 符号 hash 从灰度亮度改为 `max(R,G,B)` 前景强度。黑底 + 高饱和前景下，红/蓝/紫不再因为灰度偏暗而更容易受背景噪声影响。
- 颜色匹配改用近似 YCbCr 距离，更贴近 Moonlight/GPU 编码后的 YUV 压缩失真。

效果：提升 decoder CPU 速度，同时提高视频压缩后的单 cell 判定稳定性。

## 4. 最终吞吐不只看 fps

端到端速度大致取决于：

```text
吞吐 ~= 新增独立 packet fps * 每 packet fountain payload
```

因此仅有 `capture_fps` 或 `decode_fps` 高还不够，还必须提高：

- `valid_fps`：通过单帧 ECC/帧头解析的 packet 数。
- `valid_fps` 后半部分：真正新增喷泉码秩的 packet 数。
- 每个 packet 的有效 payload。
- decoder 最终 MD5 校验通过率。

## 5. valid_fps 与新增帧速率

提交：`f6ca32f feat: 支持多档颜色通道并区分新增帧速率`

decoder 现在显示：

```text
valid_fps=通过校验packet/新增独立packet
```

原因：

- 当 decoder fps 高于 encoder fps 时，会重复截到同一张图。
- 旧的 valid fps 会把重复数据也算进去，导致高估实际吞吐。
- 现在同一个 `frame_id` 重复出现时，仍可统计为 valid，但不会进入喷泉码消元，也不会算作新增独立 packet。

效果：进度显示更接近真实传输速度，也减少重复帧进入喷泉矩阵消元的 CPU 开销。

## 6. ECC 对正确率和速度的影响

提交：`3e531bf feat: 接入单帧 ECC 纠错`

当前 ECC 是 packet 内 Reed-Solomon，并做了交织：

- `-ecc 0`：没有单 packet 纠错，速度和容量最高，但任何 cell/byte 错误都可能污染喷泉码。
- `-ecc 2/3/10/20`：牺牲部分 payload，换取视频压缩/截图误差下更高 valid fps。
- 交织后，局部图像错误会分散到多个 RS codeword，避免局部遮挡或压缩块集中打坏同一个 codeword。

实践现象：

- 无 ECC 时可能最终 MD5 错误，decoder 直到源数据头 MD5 校验阶段才能确认结果是否正确。
- 低 ECC 有时反而整体更快，因为 valid fps 提升超过了 payload 损失。
- 高 ECC 适合高 `Q`、高 fps、视频压缩较重的场景。

## 7. 颜色通道优化

提交：

- `5951136 feat: 增加可配置颜色通道`
- `f6ca32f feat: 支持多档颜色通道并区分新增帧速率`
- `6199577 perf: 优化高阶颜色通道调色板`

当前支持：

| 参数 | 颜色数 | cell bits | 特点 |
| --- | ---: | ---: | --- |
| `-color-bits 1` | 2 | 5 | 最稳，容量最低 |
| `-color-bits 2` | 4 | 6 | 默认，稳定性/容量折中 |
| `-color-bits 3` | 8 | 7 | 值得在 Moonlight 下重点测试 |
| `-color-bits 4` | 16 | 8 | 容量最高，但对视频压缩最敏感 |

优化内容：

- 支持 1/2/3/4 bit 四档颜色通道。
- 高阶调色板改为更大 RGB/近似 YCbCr 间距。
- 默认 4 色前 4 个颜色 ID 保持不变，避免破坏默认链路。

结论：

- `color-bits` 增大能提升理论容量，但也增加颜色误判概率。
- 在 Moonlight/GPU 编码链路中，`3 bit` 可能比 `4 bit` 更实用。
- `4 bit` 如果 valid fps 低，实际速度会低于 `2/3 bit`。

## 8. 单帧多独立 packet：解决大 Q 下整帧失败

提交：`1464f04 perf: 支持单帧多独立 packet`

问题：

`Q` 越大，一张图里的 cell 越多。即使单 cell 错误率不变，整张图完全正确的概率也会下降。旧模式下一张图只有一个 ECC packet，一处局部错误可能导致整张图的 packet 被丢弃，`Q=160` 以后 valid fps 急剧下降。

方案：

新增运行时参数：

```text
-packets N
```

开启后，一张屏幕图顺序承载 N 个独立 ECC packet：

- encoder 一次生成一张图，但图内包含多个独立 packet。
- decoder 对每个 packet 单独 ECC 解码、单独解析帧头、单独加入喷泉码。
- 某个 packet 失败，不影响同一张图里的其他 packet。

约束：

- `-packets` 不写入帧头，encoder/decoder 必须运行时指定一致。
- 默认 `-packets 1`，保持旧行为。
- packet 变多后，单个 packet payload 会变小，但大 Q 下有效 packet 数可能明显增加。

为什么这有助于冲 1 MB/s：

- 旧公式接近：`吞吐 = 整图 valid fps * 大 payload`
- 新公式接近：`吞吐 = packet valid fps * 中等 payload`
- 在大 Q 且局部错误多时，后者更不容易因为单点错误归零。

建议测试：

```bash
./bin/encoder.exe -screen -i input.bin -Q 160 -B 1 -R 3:-0:-0 -fps 120 -ecc 10 -packets 2
./bin/decoder.exe -screen -o out.bin -Q 160 -B 1 -R 3:-0:-0 -fps 120 -ecc 10 -packets 2
```

如果 `valid_fps` 稳定，再试：

```bash
-packets 4
```

## 9. zstd 与文件级 MD5

提交：

- `9f5a0ee feat: 默认启用高容量参数和 zstd 压缩`
- `dabcc8d feat: 增加文件级 MD5 校验`
- `3e65334 feat: 支持关闭 zstd 并拆分截图统计`

优化点：

- 默认 zstd 压缩源文件，减少需要传输的总字节数。
- 源数据头只出现一次，不在每帧重复传输。
- 源数据头包含原始文件大小、压缩方式、MD5。
- decoder 写出文件前做大小和 MD5 校验，防止无 ECC 或错误喷泉块导致 silent corruption。

效果：

- 可压缩文件速度提升明显。
- 不可压缩文件也有 MD5 校验兜底。
- `-no-zstd` 可用于已经压缩过的数据或性能对比。

## 10. 当前推荐测试矩阵

先用默认稳态确认链路：

```bash
-Q 120 -fps 120 -ecc 3 -color-bits 2 -packets 1
```

冲更高吞吐时建议按顺序测试：

```bash
-Q 140 -fps 120 -ecc 10 -color-bits 2 -packets 1
-Q 160 -fps 120 -ecc 10 -color-bits 2 -packets 2
-Q 160 -fps 120 -ecc 10 -color-bits 3 -packets 2
-Q 160 -fps 120 -ecc 20 -color-bits 3 -packets 4
```

观察指标：

- encoder：`frame_fps`、`packet_fps`、`refresh_fps`
- decoder：`capture_fps`、`decode_fps`、`valid_fps=a/b`
- 端到端：`speed KB/s`、最终 MD5

## 11. 后续优化方向

还有几类可能继续推进到 1 MB/s 以上：

1. 对 `-packets` 做自适应推荐，根据 `Q/ecc/color-bits` 自动给出合理 packet 数。
2. 增加 packet 内 CRC，尽早丢弃无 ECC 或低 ECC 下的错误 packet。
3. 继续优化高阶颜色模式，尤其是 `color-bits 3` 的调色板和判定阈值。
4. 将 cell 解码并行化，尤其是 `Q >= 160` 时可按行块并发。
5. 更换喷泉码实现，减少线性消元成本和对 blockCount 的依赖。

## 12. 小结

这轮优化的核心路线是：

1. 先让 encoder 摆脱 Windows timer、拷贝和分配瓶颈，达到高 fps。
2. 再让 decoder 截屏和解码走 BGRA 快路径，拆分 capture/decode 指标。
3. 最后处理真正决定吞吐的有效率：ECC 交织、颜色判定、有效/新增帧统计，以及 `-packets` 多独立 packet。

因此现在的性能分析不能只看 fps。最终速度要看：

```text
新增独立 packet fps × 每 packet payload × MD5 正确率
```

这也是后续继续冲 1 MB/s 时最应该盯住的指标。
