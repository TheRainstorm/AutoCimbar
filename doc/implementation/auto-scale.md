# 居中图案自动缩放识别

实现日期：2026-09-11。命令行 `-auto-scale`，GUI 完整版 `Auto scale (receiver)`，均默认关闭。

## 参数与使用范围

`Q/RQ` 决定逻辑 cell 数和 packet 容量，不应因接收端显示器分辨率不同而修改。新增功能自动确定接收画面中的像素尺寸，把图像还原到现有 decoder 的网格后再解码。encoder 无需更改帧格式，也不添加定位标记。

```bash
# 云桌面：居中发送，屏幕编号按该机器调整
./bin/encoder.exe -i input.bin -RQ 80 -r 0:c:c
# 本地：在屏幕 1 搜索，自动适配例如 2x 的放大
./bin/decoder.exe -RQ 80 -r 1 -auto-scale
```

两端逻辑 `Q/RQ`、cell、ECC、packets、符号集必须一致。B 在自动模式中用于 decoder 的规范化输出尺寸；允许发送端和接收端不同，不再需要通过接收端 B 猜测远程缩放比例。压缩方式依旧从源数据头识别。

- 仅支持 symbols screen backend；与 QR 或 PNG 同用会明确报错。
- `-r` 只选择显示器，忽略 X:Y；搜索图案中心固定在该显示器中心。
- 支持等比例缩放，允许整数像素取整。远程客户端应全屏或使传输图案恰好位于所选屏幕中心。
- 不做任意位置、透视、非等比例拉伸或 RQ 自动推断。
- 搜索下限为 `grid * max(tileWidth, tileHeight)` 像素，上限为屏幕短边。小于一个屏幕像素的逻辑 tile 像素已经丢失信息，不保证可还原；先提高发送端 B。
- 插值和视频压缩仍可能破坏颜色，建议先用默认 2bit color。此功能不修复 HDR 色彩变换。

GUI 完整版开关只影响接收任务。GUI Lite 保持原有固定设置。`~/.autocambar.ini` 的 `[decoder]` 或 `[gui]` 支持 `auto-scale = true`，CLI 可用 `-auto-scale=false` 覆盖配置。

## 实现流程

1. 使用现有 GDI/DXGI 后端截取所选整屏，继承已有显示器编号、旋转和 BGRA 约定。
2. 未锁定时遍历居中正方形尺寸，在 16 个分散 cell 中采样逻辑 tile 像素。用符号 hash 的汉明距离排序，并排除低对比度的平坦区域。
3. 取最多 8 个尺寸峰值，细查附近的 1～2 像素尺寸。模板匹配仅用于排序，候选必须经过完整 cell decode、packet ECC 和 ParsePacket CRC 才能锁定。任何一个 packet 成功即可锁定，但其它 packet 仍按原流程独立校验。
4. 按每个逻辑 tile 像素的中心重采样，再展开为 decoder 所需的 B。保持原始通道顺序，RGBA 与 BGRA 共用几何逻辑。
5. 锁定期间复用采样坐标表、截图和输出缓冲，规范化帧进入原有去重、并行解码和 fountain 流程。
6. 每秒额外校验一次当前区域，连续 3 次失败则解锁。一次搜索结束后至少等待 1 秒才再次搜索；搜索中响应停止信号。接收任务和 fountain 状态保留。

自动定位使用独立的 probe decoder 和 ECC buffer，不与解码 worker 共享可变状态。关闭功能时继续使用原来的指定区域截图路径。

## 日志与诊断

```text
auto-scale: searching centered symbol frame on selected display (packet CRC required)
auto-scale: locked rect=(1280,440)-(2560,1720) (display-local) scale=2.0000 size=1280 -> 640 pixels
auto-scale: lost valid packets; searching again
```

坐标相对选定显示器左上角。`scale` 是实际区域边长 / decoder 规范化边长；B 不同时不等于远程客户端的纯缩放比例。

`-debug-capture DIR` 在自动模式中保存前 60 次原始整屏截图到 `DIR/<cell>_NNN.png`，便于诊断未锁定的问题；关闭自动模式时仍按原来的区域截图行为保存。保存 PNG 会降低截图速度。

`cap` 包含正在搜索时的截图；未找到有效候选时不向 decoder 队列提交图像。`-v` 的 `cap_ms` 此时覆盖截图、自动定位/重采样以及周期性 probe 校验，所以搜索期高耗时不等于 DXGI API 慢。`dec_ms` 仍是下游 decoder 的实际解码耗时，不含自动定位 probe。

## 验证与性能

`pkg/app/autoscale_test.go` 用真实内置符号、随机 packet 数据和独立的双线性远程缩放模拟验证：

- 1080p 到 4K 的 2x 缩放，含 RQ120 大网格。
- 1.25x、1.5x 缩放，以及 B=2 后 0.75x 缩小。
- 8x8、6x6、4x4 tile，单 packet 和多 packet，RGBA 和 BGRA。
- 普通桌面不锁定，错误逻辑网格不锁定，缩放改变后释放旧区域并重新定位。
- 多帧 fountain 恢复、zstd 解压、原始文件名和 MD5 一致。

在 Linux / Ryzen 5 5600G 上，`BenchmarkAutoScaleLocked` 使用静态 4K 截图，把 1920x1920 图案还原到 RQ120 的 960x960 输出：初版逐像素坐标换算约 23.9 ms，预计算采样坐标后约 9.1 ms。这个数字包含周期性的 probe 校验，不包含真实屏幕 API、下游持续解码或磁盘保存，不能直接当作 Windows 端到端帧率。

本模式持续整屏截图以便缩放变化后重定位，因此额外消耗显存/内存带宽，GDI 尤其明显。若客户端可以保持原始像素尺寸，关闭自动识别仍然最快。真实 Windows 云桌面效果需要在具体客户端插值、刷新率和显示器配置下实测。
