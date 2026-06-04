# AutoCamBar

基于 [libcimbar](https://github.com/sz3/libcimbar) 技术的彩色符号二维码系统（Go 实现）

## 特性

- **高密度编码**: 每个 cell 编码 6 bits（4 色 × 16 符号）
- **Image Hash 识别**: 使用汉明距离进行快速符号识别
- **LAB 色彩空间**: 更接近人眼感知的颜色识别
- **Reed-Solomon 纠错**: 可配置纠错率（0-100%）
- **高性能**: 符号识别 ~1µs，编码 ~115µs
- **完整测试**: 所有核心模块 100% 测试覆盖

## 技术参数

| 参数 | 值 |
|------|-----|
| 颜色数量 | 4（黑、白、红、蓝）|
| 符号数量 | 16 |
| 每 Cell 位数 | 6 bits |
| 推荐纠错率 | 20% |
| 符号识别方法 | Image Hash + 汉明距离 |
| 颜色识别方法 | LAB 色彩空间距离 |

## 项目结构

```
autocambar/
├── pkg/
│   ├── symbol/          # 符号识别模块
│   │   ├── imagehash.go      # Image hash 计算
│   │   └── recognizer.go     # 符号识别器
│   ├── color/           # 颜色识别模块
│   │   └── recognizer.go     # LAB 色彩空间识别
│   ├── codec/           # 编解码器
│   │   └── codec.go          # 编码/解码实现
│   └── ecc/             # 纠错码
│       └── ecc.go            # Reed-Solomon 实现
├── cmd/
│   ├── encoder/         # 编码器 CLI（待实现）
│   └── decoder/         # 解码器 CLI（待实现）
└── docs/
    ├── technical_design.md    # 技术设计文档
    ├── cimbar_learnings.md    # libcimbar 学习笔记
    └── PROGRESS.md            # 实现进展总结
```

## 性能指标

| 操作 | 性能 | 内存分配 |
|------|------|---------|
| Image Hash | ~650 ns/op | 0 B/op |
| 符号识别 | ~1200 ns/op | 128 B/op |
| 颜色识别 | ~1100 ns/op | 256 B/op |
| 编码 (100 bytes) | ~115 µs | 647 KB/op |
| 解码 (100 bytes) | ~10 ms | 3 MB/op |
| ECC 编码 | ~1.3 µs | 4 KB/op |
| ECC 解码 | ~626 ns | 2.6 KB/op |

## 快速开始

### 安装

```bash
go get github.com/autocambar/autocambar
```

### 使用示例

```go
package main

import (
    "github.com/autocambar/autocambar/pkg/symbol"
    "github.com/autocambar/autocambar/pkg/color"
    "github.com/autocambar/autocambar/pkg/codec"
)

func main() {
    // 创建识别器
    symRec := symbol.NewRecognizer()
    colorRec := color.NewRecognizer4Color()
    
    // 加载符号集（需要预先生成）
    // ... 加载 16 个符号到 symRec
    
    // 创建编码器
    encoder := codec.NewEncoder(symRec, colorRec, 8, 50) // 8px cell, 50x50 grid
    
    // 编码数据
    data := []byte("Hello, AutoCamBar!")
    img, err := encoder.Encode(data)
    if err != nil {
        panic(err)
    }
    
    // 保存图像
    // ...
}
```

## 开发状态

### ✓ 已完成

- [x] Image Hash 符号识别核心
- [x] 符号识别器（16 符号）
- [x] 颜色识别（LAB 色彩空间）
- [x] 编解码器（6 bits/cell）
- [x] Reed-Solomon 纠错码

### 🚧 进行中

- [ ] 符号集生成器
- [ ] 端到端测试
- [ ] CLI 工具（encoder/decoder）

### 📋 计划中

- [ ] Fountain 码支持
- [ ] 性能优化（并发、SIMD）
- [ ] 可视化调试工具

## 测试

```bash
# 运行所有测试
go test ./...

# 运行性能测试
go test -bench=. -benchmem ./...

# 查看测试覆盖率
go test -cover ./...
```

## 文档

- [技术设计文档](technical_design.md) - 详细的技术方案
- [libcimbar 学习笔记](cimbar_learnings.md) - 核心技术要点
- [实现进展总结](PROGRESS.md) - 当前进展和下一步计划

## 依赖

- [github.com/klauspost/reedsolomon](https://github.com/klauspost/reedsolomon) - Reed-Solomon 纠错码

## 许可证

MIT

## 致谢

本项目基于 [libcimbar](https://github.com/sz3/libcimbar) 的设计思想，特别感谢 sz3 的开源贡献。

## 联系

如有问题或建议，请提交 Issue 或 PR。
