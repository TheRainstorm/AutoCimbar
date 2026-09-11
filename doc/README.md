# AutoCimBar 文档索引

本文档是 `doc/` 目录的入口。面向日常使用时优先读根目录的 [README.md](../README.md)；需要理解实现、调参或继续开发时，再按下面的主题进入。

## 目录结构

```text
doc/
├── README.md                         # 本索引
├── demo.png                          # README 使用的 GUI 截图
├── performance/                      # 当前性能、截图和指标文档
├── implementation/                   # GUI/实现记录
├── research/                         # 外部项目和技术学习笔记
└── archive/                          # 早期规划和历史进展，保留参考，不保证完全反映当前实现
```

## 用户入口

- [中文 README](../README.md)：项目简介、编译、快速使用、关键参数和当前限制。
- [English README](../README.en.md)：英文版 README，内容与中文 README 对齐。

如果只是想使用程序，通常读 README 就够了。GUI 用户优先使用 `gui.exe` 或 `guilite.exe`；命令行用户重点关注 `-RQ`、`-r`、`-c`、`-ecc`、`-p`、`-capture-backend`。

## 性能与调参

- [性能优化总览](performance/overview.md)
  - 汇总 encoder、decoder、ECC、packets、zstd、MD5 和吞吐相关优化。
  - 适合想理解“为什么能到 1MB/s”以及下一步性能方向时阅读。
- [Decoder pipeline 与 Windows timer 分析](performance/decoder-pipeline.md)
  - 解释 `cap`、`dec`、`pkt v/r/u`、`bad`、`spd`、`ema` 和 `-v` 诊断指标。
  - 适合判断瓶颈在 capture、cell decode、packet/ECC/fountain 还是图像质量时阅读。
- [DXGI 截图后端分析](performance/dxgi-capture.md)
  - 说明 GDI/DXGI 后端、显示器编号一致性、旋转屏映射和 HDR/color management 限制。
  - 适合排查 Windows decoder 截图速度或颜色不一致问题时阅读。
- [Decoder cell decode 并行优化](performance/decode-parallel.md)
  - 记录通用 cell decode 并行化实现、race 风险规避和 benchmark 加速比。
  - 适合继续优化 decoder CPU 热路径时阅读。

## 实现记录

- [居中图案自动缩放识别](implementation/auto-scale.md)：远程桌面缩放的定位、CRC 锁定、重采样、支持范围与测试。

- [Wails GUI 实现记录](implementation/gui-wails.md)
  - 记录 GUI/full GUI/Lite GUI 的后端结构、前端结构、托盘、配置读取和 release automation。
  - 适合继续维护 GUI 或发布流程时阅读。

## 研究资料

- [从 libcimbar 学到的关键技术](research/cimbar-learnings.md)
  - 记录 image hash、符号设计、ECC/interleaving、fountain code、颜色编码等背景知识。
  - 适合理解 AutoCimBar 的设计来源和和 cimbar/libcimbar 的关系。

## 历史归档

这些文档保留原始上下文，方便追溯项目早期想法和阶段性记录。它们可能包含旧命令、旧参数或未实现方案，不应作为当前使用说明。

- [早期项目笔记](archive/project-notes.md)
- [早期实现进展](archive/progress-legacy.md)
- [早期详细技术方案](archive/technical-design-legacy.md)

## 维护规则

- README 只保留安装、快速使用、关键参数和当前限制。
- 性能实验、调试方法和指标解释放在 `doc/performance/`。
- GUI、发布、平台集成类实现记录放在 `doc/implementation/`。
- 外部项目学习和算法背景放在 `doc/research/`。
- 过期但仍有参考价值的长文档移动到 `doc/archive/`，并在文件名中标注 `legacy` 或 `notes`。
- 修改 decoder 进度输出或命令行参数时，同步更新 README 和 `doc/performance/decoder-pipeline.md`。
