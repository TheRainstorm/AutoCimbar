# AutoCambar 详细技术方案

## 关键设计决策

### 与 libcimbar 对比
- **libcimbar**: 颜色 2-3 bit + 形状 4 bit (16种形状)，每 cell 4×4 像素，需要摄像头扫描
- **AutoCambar**: 同样使用颜色 2-3 bit + 形状 4 bit (16种形状)，每 cell 4×4 像素，B 参数控制屏幕缩放
- **核心区别**: 
  - 传输信道不同：屏幕截图 vs 物理摄像头
  - **无需定位标记**：encoder 和 decoder 的显示/截图区域手动指定，节省空间提高数据密度
  - 数字信道更可靠，可以追求更高的信息密度和帧率

### 核心技术原理：Image Hashing
参考 libcimbar 的设计，使用 **image hash** 进行符号识别：

1. **符号设计**：每个 8×8 tile 转换为 64-bit image hash（阈值化：亮=1，暗=0）
2. **汉明距离**：16 个符号彼此间保持约 20 bits 汉明距离，确保即使模糊/损坏也能正确识别
3. **解码方法**：计算 tile 的 image hash，与 16 个参考符号比较汉明距离，选择最接近的

**优势**：
- 简单高效，适合实时处理
- 对模糊、噪声有良好的容错能力
- libcimbar 已证明在物理摄像头场景下可达 106 KB/s

## 1. 项目概述

### 1.1 项目目标
开发一个高速文件传输系统，通过远程桌面截屏场景，将文件转换成高信息密度的二维码序列进行传输。

### 1.2 核心指标
- **传输速度**: > 106 KB/s
- **目标文件大小**: MB 级别
- **传输信道**: 远程桌面截屏（非物理摄像头）
- **平台支持**: Windows（优先）
- **部署要求**: 易于分发，避免 DLL 兼容性问题

### 1.3 与 libcimbar 的区别
- libcimbar: 物理摄像头扫描，受光照、角度、距离影响
- AutoCambar: 屏幕截图，数字信道，可实现更高信息密度

## 2. 系统架构

### 2.1 整体架构
```
┌─────────────────┐         ┌─────────────────┐
│   Source File   │         │  Decoded File   │
└────────┬────────┘         └────────▲────────┘
         │                           │
         ▼                           │
┌─────────────────┐         ┌─────────────────┐
│    Encoder      │         │    Decoder      │
│  - 文件分块     │         │  - 屏幕截图     │
│  - 喷泉码编码   │         │  - 图像识别     │
│  - 二维码生成   │         │  - 喷泉码解码   │
│  - 屏幕显示     │         │  - 文件重组     │
└────────┬────────┘         └────────▲────────┘
         │                           │
         │     屏幕显示/远程桌面      │
         └───────────────────────────┘
```


### 2.2 编程语言选择

#### Encoder
- **推荐**: Go
  - 原因: 易于编译成单一可执行文件，无需运行时依赖
  - 跨平台编译简单
  - 性能优秀，并发模型成熟
- **备选**: Python (开发阶段快速原型)

#### Decoder
- **推荐**: Go
  - 原因同 Encoder
  - 需要高性能截屏和图像处理
- **备选**: Python + PyInstaller (但需注意依赖打包问题)

#### 浏览器版 Encoder (Phase 2)
- JavaScript + WebAssembly
  - 前端: React/Vue + Canvas API
  - 核心编码逻辑: Go 编译为 WASM

## 3. 核心技术模块

### 3.1 高信息密度二维码设计

#### 3.1.1 编码方案
参考 libcimbar，采用多维度编码:

**颜色通道 (Color Channel)**
- 使用 4/8 色彩编码 (2-3 bits per cell)
- 颜色选择: 高对比度、易区分
  - 4 色 (2 bits): 黑、白、红、蓝
  - 8 色 (3 bits): 黑、白、红、蓝、绿、黄、青、品红

**形状通道 (Symbol Channel)**
- 16 种形状 (4 bits per cell)
- **选择标准**：汉明距离最远的 16 种形状（参考 libcimbar）
  - 每对符号的 image hash 汉明距离约 20 bits
  - 即使符号模糊或损坏，距离关系仍基本保持
- **符号来源**：
  - 方案 A：直接使用 libcimbar 的 16 个符号（如果许可允许）
  - 方案 B：使用遗传算法生成新符号集（参考 cimbar-tiles-generator）
  - 方案 C：手工设计 + 实验筛选（libcimbar 原始方法）

**Image Hash 原理**
- 8×8 tile → 64-bit 二进制数
- 阈值化：每个像素 > 阈值则为 1，否则为 0
- 从左到右，从上到下编码为 64-bit 数字
- 解码时计算汉明距离，选择最接近的符号

**Cell 物理尺寸**
- 基础 cell 大小: 4×4 像素（逻辑单位）
- B 参数控制缩放: 实际屏幕像素 = 4 × B
  - B=1: 4×4 屏幕像素
  - B=2: 8×8 屏幕像素
  - B=3: 12×12 屏幕像素

**位置编码**
- Grid 大小可配置 (Q 参数)
- 整个二维码尺寸: (Q × 4 × B) 像素

**信息密度计算**
- 每个 cell: color_bits + symbol_bits = 3 + 4 = 7 bits (8色+16形状)
- Q=20 (假设 400 cells), 每个 cell 7 bits: 400 × 7 = 3500 bits ≈ 437 bytes
- 实际可用数据量需减去 ECC 开销

#### 3.1.2 纠错码 (ECC)
- 使用 Reed-Solomon 码
- 纠错率可配置: 10%-30%
- 平衡: 信息密度 vs 容错能力
- 允许部分识别错误而不丢弃整张图


### 3.2 喷泉码 (Fountain Code)

#### 3.2.1 选择 Wirehair 库
- **优点**:
  - Rateless 编码，不需要预知丢包率
  - 接收约 N/b + O(sqrt(N/b)) 个包即可解码
  - C/C++ 实现，有 Go binding
- **Go 集成**: 使用 CGo 或纯 Go 实现

- 平衡: 信息密度 vs 容错能力
- 允许部分识别错误而不丢弃整张图

#### 3.1.3 同步与定位
- 四角定位标记 (类似 QR Code 的定位符)
- 用于图像旋转、缩放、透视矫正
- 可选: 时序标记用于帧序列识别

### 3.2 喷泉码 (Fountain Code)

#### 3.2.1 选择 Wirehair 库
- **优点**:
  - Rateless 编码，不需要预知丢包率
  - 接收约 N/b + O(sqrt(N/b)) 个包即可解码
  - C/C++ 实现，有 Go binding
- **Go 集成**: 使用 CGo 或纯 Go 实现

#### 3.2.2 参数设计
- **Block Size (b)**: 每个二维码承载的数据量
  - 建议: 100-500 bytes (取决于 Q 和 ECC 参数)
- **Max Message Size**: 可配置的最大文件大小
  - 设计为运行时参数，不硬编码
  - 例如: 1MB, 10MB, 100MB
- **Overhead**: 通常需要接收 N/b × 1.05 ~ 1.10 个包

#### 3.2.3 编码流程
```
Input File (N bytes)
    ↓
Split into blocks (size b)
    ↓
Wirehair Encode → Generate infinite fountain-coded blocks
    ↓
Each block → Add ECC → Generate QR Code
    ↓
Display sequence
```

### 3.3 Encoder 设计

#### 3.3.1 模块划分
```
encoder/
├── main.go              # 入口，参数解析
├── file_reader.go       # 文件读取和分块
├── fountain_encoder.go  # Wirehair 封装
├── qr_generator.go      # 高密度二维码生成
├── display.go           # 屏幕显示控制
└── config.go           # 配置管理
```


#### 3.3.2 工作流程
```go
// 伪代码
func Encode(inputFile string, config Config) {
    // 1. 读取文件
    data := readFile(inputFile)
    
    // 2. 喷泉码编码初始化
    encoder := wirehair.NewEncoder(data, config.BlockSize)
    
    // 3. 生成器 goroutine
    blockChan := make(chan []byte, 10)
    go func() {
        for {
            block := encoder.NextBlock()
            blockChan <- block
        }
    }()
    
    // 4. 二维码生成 goroutine pool (多进程)
    qrChan := make(chan image.Image, 5)
    for i := 0; i < runtime.NumCPU(); i++ {
        go func() {
            for block := range blockChan {
                qr := generateHighDensityQR(block, config.Q, config.B)
                qrChan <- qr
            }
        }()
    }
    
    // 5. 显示 goroutine
    display := NewDisplay(config.Position, config.Size)
    for qr := range qrChan {
        display.Show(qr)
        time.Sleep(config.FrameInterval) // 控制帧率
    }
}
```

#### 3.3.3 性能优化
- **多进程并行**: 生成、编码、显示流水线
- **GPU 加速** (Phase 2): 使用 CUDA/OpenCL 加速图像生成
- **预生成缓冲**: 提前生成 5-10 帧，避免显示卡顿


#### 3.3.4 屏幕显示控制
- **窗口管理**:
  - Go: 使用 `fyne` 或 `go-gl` 创建窗口
  - 无边框窗口，指定位置和大小
- **位置参数** (-R flag):
  - 格式: `X:Y` 或 `-X:-Y` (负数表示从右下角计算)
  - 例如: `-0:-0` → 右下角贴边
- **刷新率**: 目标 30-60 FPS，平衡速度和识别率

### 3.4 Decoder 设计

#### 3.4.1 模块划分
```
decoder/
├── main.go              # 入口，参数解析
├── screen_capture.go    # 高性能截屏
├── qr_decoder.go        # 高密度二维码识别
├── fountain_decoder.go  # Wirehair 解码
├── file_writer.go       # 文件写入
└── config.go           # 配置管理
```

#### 3.4.2 工作流程
```go
// 伪代码
func Decode(outputFile string, config Config) {
    // 1. 喷泉码解码器初始化
    decoder := wirehair.NewDecoder(config.BlockSize)
    
    // 2. 截屏 goroutine
    frameChan := make(chan image.Image, 10)
    go func() {
        capture := NewScreenCapture(config.ScreenID, config.Position, config.Size)
        for {
            frame := capture.Capture()
            frameChan <- frame
        }
    }()
    
    // 3. 解码 goroutine pool
    blockChan := make(chan []byte, 10)
    for i := 0; i < runtime.NumCPU(); i++ {
        go func() {
            for frame := range frameChan {
                block, err := decodeHighDensityQR(frame, config.Q, config.B)
                if err == nil {
                    blockChan <- block
                }
            }
        }()
    }
    
    // 4. 喷泉码解码
    for block := range blockChan {
        complete := decoder.AddBlock(block)
        if complete {
            data := decoder.GetData()
            writeFile(outputFile, data)
            return
        }
    }
}
```


#### 3.4.3 高性能截屏
**Windows 平台**:
- 使用 Windows API: BitBlt / PrintWindow
- Go 库: `github.com/kbinani/screenshot`
- 优化: 只截取指定区域，不截全屏

**Linux 平台**:
- 使用 X11: XGetImage
- 或 Wayland: wlr-screencopy

**截屏频率**:
- 目标: 60+ FPS
- 异步模式: 截屏和解码并行，延迟隐藏

#### 3.4.4 图像识别优化
- **预处理**:
  - 灰度化 (如果只用位置，不用颜色)
  - 二值化 / 颜色量化
  - 透视矫正 (基于定位标记)
- **去重**:
  - 使用帧哈希，避免重复解码相同帧
  - 喷泉码本身允许重复，但浪费计算资源
- **多进程解码**:
  - 解码是 CPU 密集型，充分利用多核
- **GPU 加速** (Phase 2):
  - 使用 OpenCV CUDA 模块
  - 或自定义 CUDA kernel

### 3.5 配置参数系统

#### 3.5.1 参数列表
```
-i, --input         输入文件路径 (encoder only)
-o, --output        输出文件路径 (decoder only)
-Q, --quality       二维码版本/大小 (10-40, 影响 grid 大小)
-B, --block-scale   像素缩放因子 (1-5)
-R, --region        显示/截图区域
                    encoder: X:Y
                    decoder: ScreenID:X:Y
-E, --ecc           纠错率 (0.1-0.3)
-F, --fps           帧率 (encoder: 显示帧率, decoder: 截屏帧率)
-M, --max-size      最大文件大小 (MB)
--colors            颜色数 (4/8)
--symbols           形状数 (4)
```

#### 3.5.2 配置文件支持
- 使用 TOML/YAML 配置文件
- 命令行参数优先级高于配置文件
- 预设配置: `fast`, `balanced`, `reliable`


## 4. 性能分析与优化

### 4.1 速度瓶颈分析

#### 瓶颈 1: 单个二维码数据容量
- **目标**: 每个 QR 至少 500-1500 bytes 有效数据
- **计算** (8色+16形状，每 cell 7 bits):
  - Q=20, Grid 20×20 = 400 cells
  - 每 cell 7 bits = 3500 bits = 437.5 bytes raw
  - 减去 ECC 20% = 350 bytes 有效数据
  
  - Q=30, Grid 30×30 = 900 cells
  - 每 cell 7 bits = 7875 bits = 984 bytes raw
  - 减去 ECC 20% = 787 bytes 有效数据
  
  - Q=40, Grid 40×40 = 1600 cells
  - 每 cell 7 bits = 14000 bits = 1750 bytes raw
  - 减去 ECC 20% = 1400 bytes 有效数据

- **帧率**: 30-60 FPS
- **理论速度**: 
  - Q=30: 787 bytes × 30 FPS = 23.6 KB/s
  - Q=40: 1400 bytes × 30 FPS = 42 KB/s
  - Q=40: 1400 bytes × 60 FPS = 84 KB/s

**优化方向**:
- 提高 Q 参数（更大的 grid）
- 提高帧率
- 降低 ECC 比例（牺牲容错率）
- 多 QR 并行显示

#### 瓶颈 2: 帧率限制
- **显示器刷新率**: 60 Hz
- **识别延迟**: 每帧解码需要 10-30ms
- **目标帧率**: 30-60 FPS 可行

#### 瓶颈 3: 喷泉码开销
- **理论**: N/b 个包
- **实际**: N/b × 1.05 ~ 1.10 个包
- **开销**: 5%-10%

### 4.2 达成 106 KB/s 的路径

**方案 1: 大 Grid + 中等帧率**
- Q=50, Grid 50×50 = 2500 cells
- 每 cell 7 bits = 21875 bits = 2734 bytes raw
- 减去 ECC 20% = 2187 bytes 有效数据
- 30 FPS → 65.6 KB/s (不够)
- 60 FPS → 131 KB/s ✓ 达成目标

**方案 2: 超大 Grid + 低帧率**
- Q=60, Grid 60×60 = 3600 cells
- 每 cell 7 bits = 31500 bits = 3937 bytes raw
- 减去 ECC 20% = 3150 bytes 有效数据
- 30 FPS → 94.5 KB/s (接近)
- 40 FPS → 126 KB/s ✓ 达成目标

**方案 3: 多 QR 并行显示**
- 2 个 QR，Q=40 每个
- 每个 1400 bytes × 30 FPS = 42 KB/s
- 总计: 2 × 42 = 84 KB/s (不够)
- 
- 3 个 QR，Q=40 每个
- 总计: 3 × 42 = 126 KB/s ✓ 达成目标

**方案 4: 降低 ECC，提高数据密度**
- Q=50, ECC 10%（更激进）
- 2734 bytes raw × 0.9 = 2460 bytes 有效数据
- 30 FPS → 73.8 KB/s
- 50 FPS → 123 KB/s ✓ 达成目标


### 4.3 推荐方案
**Phase 1**: 单 QR，大 Grid
- Q=50, B=2, 8色+16形状
- 目标: 2187 bytes/frame, 60 FPS
- 速度: 131 KB/s
- 整个二维码尺寸: 50 × 4 × 2 = 400 像素（适合大多数屏幕）

**Phase 2**: 优化与备选
- 备选 A: Q=60, 40 FPS, 速度 126 KB/s（更大 QR，降低帧率）
- 备选 B: 3 个 QR（Q=40），总计 126 KB/s（并行传输，降低单个识别难度）
- 根据实际测试的识别成功率选择最优方案

## 5. 技术栈与依赖库

### 5.1 Go 依赖

#### 核心库
```
// 喷泉码
github.com/google/gopacket/wirehair  // 或自己实现 RaptorQ

// 图像处理
github.com/fogleman/gg              // 2D 图形绘制
image/color                          // 标准库
image/draw

// GUI / 显示
github.com/fyne-io/fyne/v2          // 跨平台 GUI
// 或
github.com/go-gl/glfw/v3.3/glfw     // OpenGL 窗口

// 截屏
github.com/kbinani/screenshot       // 跨平台截屏

// 图像识别
github.com/makiuchi-d/gozxing       // QR 解码 (需扩展)
// 或自实现解码器

// 纠错码
github.com/klauspost/reedsolomon    // Reed-Solomon

// 配置
github.com/spf13/cobra              // CLI 框架
github.com/spf13/viper              // 配置管理
```

#### 性能库 (可选)
```
runtime                              // CPU 核心数
sync                                 // 并发控制
github.com/panjf2000/ants/v2        // goroutine 池
```


### 5.2 Python 依赖 (备选方案)

```python
# 喷泉码
pywirehair  # Wirehair Python binding

# 图像处理
Pillow      # 图像生成和处理
numpy       # 数组运算
opencv-python  # 高级图像处理

# 纠错码
reedsolo    # Reed-Solomon

# 截屏
mss         # 快速截屏
pyautogui   # 跨平台截屏 (备选)

# GUI
tkinter     # 标准库 (简单窗口)
PyQt5       # 功能强大 (但体积大)

# CLI
click       # 命令行框架
```

**打包方案**:
- PyInstaller: 打包成 .exe
- 问题: 依赖项多，体积大 (50-100 MB)
- 建议: 仅用于快速原型，最终用 Go

### 5.3 浏览器版 (Phase 2)

```
// 前端框架
React / Vue / Vanilla JS

// 文件处理
File API, FileReader

// Canvas 绘图
Canvas API

// WASM 核心
Go → WASM (编码逻辑)
```

## 6. 开发路线图

### 6.1 Phase 1: MVP (4-6 周)

#### Week 1-2: 基础设施
- [ ] 项目框架搭建 (Go)
- [ ] 高密度二维码编码器 (简化版: 颜色 + 位置)
- [ ] 高密度二维码解码器
- [ ] 单元测试: 编码→解码正确性


#### Week 3-4: 喷泉码集成
- [ ] 集成 Wirehair 或实现 RaptorQ
- [ ] Encoder: 文件 → 喷泉码块 → QR 序列
- [ ] Decoder: QR 序列 → 喷泉码解码 → 文件
- [ ] 端到端测试: 小文件 (100KB)

#### Week 5-6: 显示与截屏
- [ ] Encoder: 窗口显示 QR 序列，支持位置参数
- [ ] Decoder: 截屏指定区域
- [ ] 性能测试: 测量实际传输速率
- [ ] 调优: 达到 50+ KB/s

**交付物**:
- `encoder` 和 `decoder` 可执行文件 (Windows)
- 支持命令行参数
- 能传输 1MB 文件

### 6.2 Phase 2: 优化 (4-6 周)

#### Week 7-8: 高信息密度
- [ ] 添加形状通道 (4 种形状)
- [ ] 优化颜色识别算法
- [ ] 提升单帧数据量至 1000+ bytes

#### Week 9-10: 性能优化
- [ ] 多进程并行编码/解码
- [ ] 帧缓冲优化
- [ ] 识别算法优化 (去重、预处理)
- [ ] 达到 106 KB/s 目标

#### Week 11-12: 稳定性与易用性
- [ ] 配置文件支持
- [ ] 预设模式 (fast/balanced/reliable)
- [ ] 进度显示和统计信息
- [ ] 错误处理和日志

**交付物**:
- 优化版 encoder/decoder
- 传输速率 > 106 KB/s
- 用户文档

### 6.3 Phase 3: 扩展功能 (8-10 周)

#### Week 13-16: 浏览器版 Encoder
- [ ] Go → WASM 编译
- [ ] Web UI (文件上传、参数配置)
- [ ] Canvas 实时显示 QR

#### Week 17-20: GPU 加速
- [ ] CUDA/OpenCL 集成
- [ ] GPU 加速 QR 生成
- [ ] GPU 加速图像识别

#### Week 21-22: 高级功能
- [ ] 多 QR 并行模式
- [ ] 压缩集成 (zstd)
- [ ] 加密传输 (可选)


**交付物**:
- 浏览器版 encoder
- GPU 加速版本
- 完整功能集

## 7. 测试策略

### 7.1 单元测试
- **QR 编码/解码**: 随机数据往返测试
- **喷泉码**: 不同丢包率下的解码成功率
- **ECC**: 不同错误率下的纠错能力

### 7.2 集成测试
- **端到端**: 不同大小文件 (1KB - 100MB)
- **帧丢失**: 模拟识别失败率 (10%, 20%, 30%)
- **性能基准**: 测量实际传输速率

### 7.3 跨平台测试
- Windows 10/11
- Linux (Ubuntu, Arch)
- macOS (如果支持)

### 7.4 压力测试
- 长时间运行 (1 小时+)
- 大文件传输 (100MB+)
- 内存和 CPU 使用监控

## 8. 风险与挑战

### 8.1 技术风险

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 高密度 QR 识别准确率低 | 传输速度达不到目标 | 分阶段提升密度，充分测试 |
| 喷泉码库不成熟 | 解码失败或性能差 | 考虑自实现 RaptorQ |
| 截屏性能不足 | 帧率低，速度受限 | 使用底层 API，优化截屏区域 |
| 跨平台兼容性问题 | Windows 外平台不可用 | 优先 Windows，其他平台 Phase 2 |

### 8.2 实现难点

#### 难点 1: 颜色和形状识别
- **挑战**: 远程桌面可能有压缩损失
- **方案**:
  - 选择高对比度颜色
  - 添加容错机制
  - 提供多种识别算法切换

#### 难点 2: 达到 106 KB/s
- **挑战**: 需要极高的单帧数据量或帧率
- **方案**:
  - 优先尝试单 QR 高密度
  - 备选多 QR 并行
  - 逐步提升，设立阶段性目标 (50, 80, 106 KB/s)

#### 难点 3: 同步和去重
- **挑战**: 避免重复解码相同帧
- **方案**:
  - 帧内嵌入序列号或时间戳
  - 使用感知哈希去重


## 9. 详细实现细节

### 9.1 高密度二维码格式

#### 9.1.1 Grid 结构
```
+---+---+---+---+---+---+---+
| D | D | D | D | D | D | D |  D = 数据单元 (Data cell)
+---+---+---+---+---+---+---+  所有 cell 都用于数据编码
| D | D | D | D | D | D | D |  无需定位标记（区域手动指定）
+---+---+---+---+---+---+---+
| D | D | D | D | D | D | D |
+---+---+---+---+---+---+---+
| D | D | D | D | D | D | D |
+---+---+---+---+---+---+---+
```

**说明**:
- 由于 encoder 和 decoder 的显示/截图区域是手动指定的，无需定位标记
- 所有 cell 均可用于数据编码，提高信息密度
- Grid 尺寸: Q × Q cells
- 每个 cell 物理尺寸: 4 × B 像素
- 整个二维码屏幕尺寸: (Q × 4 × B) × (Q × 4 × B) 像素

#### 9.1.2 数据单元编码
```
每个 Cell 编码（参考 libcimbar）:
- 颜色: 2-3 bits
  · 4 色 (2 bits): 0:黑 1:白 2:红 3:蓝
  · 8 色 (3 bits): 0:黑 1:白 2:红 3:蓝 4:绿 5:黄 6:青 7:品红
  
- 形状: 4 bits (16 种形状)
  · 选取汉明距离最远的 16 种形状
  · 具体形状设计参考 libcimbar 的最优形状集
  
总计: 6-7 bits/cell (4色+16形状=6bits, 8色+16形状=7bits)
```

#### 9.1.3 数据包结构
```
[ Header | Payload | ECC ]

Header (10 bytes):
- Magic number: 0xACAB (2 bytes)
- Block index: uint32 (4 bytes) // 喷泉码块索引
- Payload size: uint16 (2 bytes)
- Checksum: uint16 (2 bytes) // CRC16

Payload: N bytes (实际数据)

ECC: M bytes (Reed-Solomon 纠错)
```


### 9.2 喷泉码实现

#### 9.2.1 编码器接口
```go
type FountainEncoder struct {
    data      []byte
    blockSize int
    seed      uint64
}

func NewFountainEncoder(data []byte, blockSize int) *FountainEncoder

// 生成第 i 个编码块
func (e *FountainEncoder) GenerateBlock(index uint32) []byte

// 无限生成
func (e *FountainEncoder) NextBlock() []byte
```

#### 9.2.2 解码器接口
```go
type FountainDecoder struct {
    blockSize    int
    receivedBlocks map[uint32][]byte
    totalSize    int
}

func NewFountainDecoder(blockSize int, totalSize int) *FountainDecoder

// 添加接收到的块
func (d *FountainDecoder) AddBlock(index uint32, data []byte) (complete bool, err error)

// 获取解码后的完整数据
func (d *FountainDecoder) GetData() ([]byte, error)

// 获取进度
func (d *FountainDecoder) Progress() float64
```

#### 9.2.3 Wirehair 集成
```go
// 使用 CGo 调用 C 库
/*
#cgo CFLAGS: -I./wirehair/include
#cgo LDFLAGS: -L./wirehair/lib -lwirehair
#include <wirehair/wirehair.h>
*/
import "C"

func wirehairEncode(data []byte, blockSize int, blockID uint32) []byte {
    // 调用 C 函数
}
```


### 9.3 图像生成优化

#### 9.3.1 QR 生成流程
```go
func GenerateHighDensityQR(data []byte, Q int, B int) image.Image {
    // 1. 添加 ECC
    encoded := addReedSolomon(data, eccRate)
    
    // 2. 转换为 bits
    bits := bytesToBits(encoded)
    
    // 3. 映射到颜色和形状
    cells := bitsToColorShape(bits)
    
    // 4. 渲染图像
    img := renderGrid(cells, Q, B)
    
    return img
}

func renderGrid(cells []Cell, Q int, B int) image.Image {
    cellSize := 4 * B  // 每个 cell 是 4×4 逻辑像素，乘以 B 得到实际屏幕像素
    size := Q * cellSize
    img := image.NewRGBA(image.Rect(0, 0, size, size))
    
    // 绘制所有数据单元（无需定位标记）
    for i, cell := range cells {
        x, y := indexToCoord(i, Q)
        drawCell(img, x*cellSize, y*cellSize, cellSize, cell.Color, cell.Shape)
    }
    
    return img
}

func drawCell(img *image.RGBA, x, y, size int, color Color, shape Shape) {
    // 绘制 16 种形状中的一种
    // shape 取值 0-15，对应汉明距离最远的形状集
    
    // 使用预定义的形状模板（参考 libcimbar）
    shapeTemplate := getShapeTemplate(shape, size)
    drawColoredShape(img, x, y, shapeTemplate, color)
}

func getShapeTemplate(shape Shape, size int) ShapeTemplate {
    // 返回预定义的形状模板
    // 这些形状是 libcimbar 选择的汉明距离最远的 16 种形状
    // 可以是：不同角度的线条组合、不同形状的多边形等
    return precomputedShapes16[shape]
}

// 预计算的 16 种形状模板
var precomputedShapes16 [16]ShapeTemplate

func init() {
    // 初始化 16 种形状
    // 设计原则：汉明距离最大化，确保识别容错率
}
```

#### 9.3.2 性能优化
- **预计算**: 缓存 16 种形状路径和模板
- **并行渲染**: 分块并行绘制
- **内存池**: 复用 image buffer


### 9.4 图像识别优化

#### 9.4.1 解码流程
```go
func DecodeHighDensityQR(img image.Image, Q int, B int) ([]byte, error) {
    // 1. 直接提取每个 cell（无需定位标记检测）
    // 截图区域已经手动指定，图像边界即为二维码边界
    cellSize := 4 * B
    cells := extractCells(img, Q, cellSize)
    
    // 2. 识别颜色和形状
    data := make([]byte, 0)
    for _, cell := range cells {
        color := recognizeColor(cell)
        shape := recognizeShape(cell)
        bits := colorShapeToBits(color, shape)
        data = append(data, bits)
    }
    
    // 3. 去除 ECC，恢复原始数据
    decoded, err := removeReedSolomon(data)
    if err != nil {
        return nil, err
    }
    
    return decoded, nil
}

func extractCells(img image.Image, Q int, cellSize int) []image.Image {
    cells := make([]image.Image, Q*Q)
    for i := 0; i < Q; i++ {
        for j := 0; j < Q; j++ {
            x := j * cellSize
            y := i * cellSize
            cells[i*Q+j] = subImage(img, x, y, cellSize, cellSize)
        }
    }
    return cells
}
```

#### 9.4.2 颜色识别
```go
func recognizeColor(cellImg image.Image) Color {
    // 计算平均颜色
    avgR, avgG, avgB := computeAverageColor(cellImg)
    
    // K 近邻分类
    minDist := math.MaxFloat64
    bestColor := Color(0)
    
    for i, refColor := range referenceColors {
        dist := colorDistance(avgR, avgG, avgB, refColor)
        if dist < minDist {
            minDist = dist
            bestColor = Color(i)
        }
    }
    
    return bestColor
}

func colorDistance(r1, g1, b1 uint8, ref RGBColor) float64 {
    // 使用 LAB 色彩空间距离
    lab1 := rgbToLab(r1, g1, b1)
    lab2 := rgbToLab(ref.R, ref.G, ref.B)
    return deltaE(lab1, lab2)
}
```


#### 9.4.3 形状识别（Image Hash 方法）
```go
// 参考 libcimbar 的 image hash 识别方法
func recognizeShape(cellImg image.Image) Shape {
    // 1. 计算 cell 的 image hash
    hash := computeImageHash(cellImg)
    
    // 2. 与 16 个参考符号计算汉明距离
    minDist := 64
    bestShape := Shape(0)
    
    for i, refHash := range reference16ShapeHashes {
        dist := hammingDistance(hash, refHash)
        if dist < minDist {
            minDist = dist
            bestShape = Shape(i)
        }
    }
    
    // 3. 可选：检查是否有明确的最佳匹配
    // 如果最小距离与次小距离太接近，可能是识别错误
    
    return bestShape
}

func computeImageHash(img image.Image) uint64 {
    // libcimbar 使用的阈值化 image hash
    // 1. 确保图像是 8×8（如果不是则缩放）
    resized := resize(img, 8, 8)
    
    // 2. 转灰度
    gray := toGrayscale(resized)
    
    // 3. 计算平均亮度作为阈值
    threshold := computeAverageBrightness(gray)
    
    // 4. 生成 64-bit hash
    var hash uint64
    for y := 0; y < 8; y++ {
        for x := 0; x < 8; x++ {
            bit := 0
            if getBrightness(gray, x, y) > threshold {
                bit = 1
            }
            hash = (hash << 1) | uint64(bit)
        }
    }
    
    return hash
}

func hammingDistance(a, b uint64) int {
    // XOR 后计算 1 的个数
    xor := a ^ b
    return popcount(xor)
}

func popcount(x uint64) int {
    // 计算 64-bit 数字中 1 的个数
    // 可以使用 math/bits.OnesCount64
    count := 0
    for x != 0 {
        count += int(x & 1)
        x >>= 1
    }
    return count
}

// 16 种参考符号的 image hash（启动时预计算或硬编码）
var reference16ShapeHashes = [16]uint64{
    // 从 libcimbar 符号文件计算得到
    // 或自己设计符号后计算
    // 每个 hash 之间汉明距离应该 >= 15-20 bits
    0x0000000000000000,  // Shape 0
    0x0000000000000001,  // Shape 1
    // ... 其余 14 个
}

// 符号初始化：从文件加载或生成
func init() {
    // 方案 A：从 libcimbar 的 bitmap/ 目录加载 PNG
    loadSymbolsFromLibcimbar("path/to/libcimbar/bitmap/")
    
    // 方案 B：使用内置的符号定义
    // ...
}

func loadSymbolsFromLibcimbar(path string) {
    for i := 0; i < 16; i++ {
        filename := fmt.Sprintf("%s/symbol_%d.png", path, i)
        img := loadImage(filename)
        reference16ShapeHashes[i] = computeImageHash(img)
    }
}
```

**优势**：
- 简单高效：只需要位运算
- libcimbar 证明可达 106 KB/s
- 对模糊和噪声有良好容错

**优化方向**（Phase 2）**：
- 实现 libcimbar 的优先级解码（按汉明距离排序）
- Drift tracking：跟踪局部像素偏移
- 置信度阈值：汉明距离太大时拒绝解码

#### 9.4.4 去重机制
```go
type FrameDeduplicator struct {
    recentHashes map[uint64]time.Time
    mutex        sync.Mutex
}

func (fd *FrameDeduplicator) IsDuplicate(img image.Image) bool {
    hash := perceptualHash(img)
    
    fd.mutex.Lock()
    defer fd.mutex.Unlock()
    
    if lastSeen, exists := fd.recentHashes[hash]; exists {
        // 如果 100ms 内见过，认为是重复
        if time.Since(lastSeen) < 100*time.Millisecond {
            return true
        }
    }
    
    fd.recentHashes[hash] = time.Now()
    
    // 清理过期哈希
    fd.cleanup()
    
    return false
}

func perceptualHash(img image.Image) uint64 {
    // 使用 pHash 算法
    // 1. 缩放到 8x8
    // 2. 转灰度
    // 3. DCT 变换
    // 4. 提取低频
    // 5. 生成 64-bit hash
    // ...
}
```

### 9.5 并发架构

#### 9.5.1 Encoder 流水线
```go
func RunEncoder(config Config) {
    // Pipeline stages
    blockChan := make(chan []byte, 10)
    qrChan := make(chan image.Image, 5)
    
    // Stage 1: 喷泉码生成
    go fountainGenerator(config.InputFile, blockChan)
    
    // Stage 2: QR 生成池 (多 worker)
    for i := 0; i < runtime.NumCPU(); i++ {
        go qrGeneratorWorker(blockChan, qrChan, config)
    }
    
    // Stage 3: 显示
    displayQRSequence(qrChan, config)
}
```


#### 9.5.2 Decoder 流水线
```go
func RunDecoder(config Config) {
    frameChan := make(chan image.Image, 10)
    blockChan := make(chan DecodedBlock, 10)
    
    // Stage 1: 截屏
    go screenCaptureLoop(frameChan, config)
    
    // Stage 2: 去重和解码池
    dedup := NewFrameDeduplicator()
    for i := 0; i < runtime.NumCPU(); i++ {
        go decoderWorker(frameChan, blockChan, dedup, config)
    }
    
    // Stage 3: 喷泉码解码和文件写入
    fountainDecodeAndWrite(blockChan, config.OutputFile)
}

func decoderWorker(in <-chan image.Image, out chan<- DecodedBlock, 
                   dedup *FrameDeduplicator, config Config) {
    for frame := range in {
        // 去重
        if dedup.IsDuplicate(frame) {
            continue
        }
        
        // 解码
        data, err := DecodeHighDensityQR(frame, config.Q, config.B)
        if err != nil {
            // 解码失败，跳过
            continue
        }
        
        // 解析 header
        block, err := parseBlock(data)
        if err != nil {
            continue
        }
        
        out <- block
    }
}
```

### 9.6 命令行界面

#### 9.6.1 Encoder 用法
```bash
# 基础用法
autocambar encode -i input.txt -o display

# 完整参数
autocambar encode \
  -i input.pdf \
  -Q 20 \              # QR 版本/大小
  -B 2 \               # 像素缩放
  -R -0:-0 \           # 显示位置（右下角）
  -E 0.2 \             # ECC 纠错率 20%
  -F 30 \              # 帧率 30 FPS
  --colors 8 \         # 8 色
  --symbols 4 \        # 4 形状
  -M 100               # 最大文件大小 100MB

# 使用配置文件
autocambar encode -i input.zip -c config.toml
```


#### 9.6.2 Decoder 用法
```bash
# 基础用法
autocambar decode -o output.txt

# 完整参数
autocambar decode \
  -o output.pdf \
  -Q 20 \              # QR 版本（需与 encoder 一致）
  -B 2 \               # 像素缩放（需与 encoder 一致）
  -R 0:-0:-0 \         # 屏幕 0，右下角截图
  -F 60 \              # 截屏帧率 60 FPS
  --colors 8 \         # 8 色
  --symbols 4          # 4 形状

# 使用配置文件
autocambar decode -o output.zip -c config.toml
```

#### 9.6.3 配置文件示例 (TOML)
```toml
# config.toml

[encoder]
quality = 20           # Q 参数
block_scale = 2        # B 参数
position = "-0:-0"     # 右下角
ecc_rate = 0.2         # 20% 纠错
fps = 30
colors = 8
symbols = 4
max_file_size_mb = 100

[decoder]
quality = 20
block_scale = 2
screen_id = 0
position = "-0:-0"
fps = 60
colors = 8
symbols = 4

# 预设模式
[presets.fast]
quality = 15
ecc_rate = 0.1
fps = 60

[presets.balanced]
quality = 20
ecc_rate = 0.2
fps = 30

[presets.reliable]
quality = 25
ecc_rate = 0.3
fps = 20
```

## 10. 性能监控与调试

### 10.1 统计信息
```go
type EncoderStats struct {
    TotalFrames      int64
    BytesSent        int64
    ElapsedTime      time.Duration
    CurrentFPS       float64
    AvgGenerationTime time.Duration
}

type DecoderStats struct {
    FramesCaptured   int64
    FramesDecoded    int64
    FramesDuplicate  int64
    FramesFailed     int64
    BytesReceived    int64
    Progress         float64  // 0.0 - 1.0
    CurrentFPS       float64
    AvgDecodeTime    time.Duration
}
```


### 10.2 实时监控界面
```
=== AutoCambar Encoder ===
File: input.pdf (2.5 MB)
Progress: [████████████░░░░░░░] 65% (1625/2500 blocks)
Speed: 112 KB/s
FPS: 28.3
Elapsed: 00:01:23
ETA: 00:00:42

=== AutoCambar Decoder ===
Output: output.pdf
Progress: [████████████░░░░░░░] 62% (1550/2500 blocks needed)
Speed: 108 KB/s
FPS: 56.7 (capture) / 27.1 (decode success)
Success rate: 95.2%
Elapsed: 00:01:26
ETA: 00:00:45
```

### 10.3 调试模式
```bash
# 保存调试信息
autocambar encode -i input.txt --debug --save-frames ./debug/

# 生成的文件:
# - frame_0000.png, frame_0001.png, ...
# - encode.log (详细日志)
# - stats.json (统计信息)

autocambar decode -o output.txt --debug --save-frames ./debug/

# 生成的文件:
# - capture_0000.png, capture_0001.png, ...
# - decode.log
# - stats.json
```

## 11. 部署和分发

### 11.1 编译
```bash
# Windows (在 Linux/Mac 上交叉编译)
GOOS=windows GOARCH=amd64 go build -o autocambar.exe ./cmd/autocambar

# Linux
GOOS=linux GOARCH=amd64 go build -o autocambar ./cmd/autocambar

# Mac
GOOS=darwin GOARCH=amd64 go build -o autocambar ./cmd/autocambar
```

### 11.2 静态链接
```bash
# 完全静态编译，无需任何依赖
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
  -ldflags="-s -w" \
  -o autocambar.exe ./cmd/autocambar

# 如果使用 CGO (Wirehair C 库)，需要静态链接
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 go build \
  -ldflags="-s -w -extldflags '-static'" \
  -o autocambar.exe ./cmd/autocambar
```

### 11.3 发布包
```
autocambar-v1.0.0-windows-amd64.zip
├── autocambar.exe
├── README.md
├── config.example.toml
└── LICENSE

autocambar-v1.0.0-linux-amd64.tar.gz
├── autocambar
├── README.md
├── config.example.toml
└── LICENSE
```


## 12. 未来扩展

### 12.1 双向通信 (Phase 3+)
- Decoder 显示小型状态 QR（接收进度、ACK）
- Encoder 定期截屏解析状态
- 实现选择性重传，提高效率

### 12.2 自适应传输 (Phase 3+)
- 实时监测解码成功率
- 动态调整 Q、ECC、FPS 参数
- 在速度和可靠性之间自动平衡

### 12.3 多文件传输 (Phase 3+)
- 支持文件列表或目录传输
- 文件元数据传输（文件名、大小、权限）
- 断点续传

### 12.4 安全性增强 (Phase 4+)
- 端到端加密 (AES-256)
- 密钥交换 (ECDH)
- 数字签名验证

### 12.5 压缩集成 (Phase 3+)
- 自动压缩（zstd、lz4）
- 针对不同文件类型选择最优压缩算法

## 13. 参考资料

### 13.1 相关项目
- **libcimbar**: https://github.com/sz3/libcimbar
  - 颜色和形状编码参考
- **Wirehair**: https://github.com/catid/wirehair
  - 喷泉码实现
- **QR Code**: ISO/IEC 18004
  - 定位标记设计参考

### 13.2 技术论文
- Fountain Codes: *"A Digital Fountain Approach to Reliable Distribution of Bulk Data"* (Byers et al., 1998)
- Reed-Solomon: *"Polynomial Codes Over Certain Finite Fields"* (Reed & Solomon, 1960)
- RaptorQ: RFC 6330

### 13.3 Go 库文档
- Image processing: https://pkg.go.dev/image
- Concurrency: https://go.dev/doc/effective_go#concurrency

## 14. 总结

### 14.1 核心优势
1. **高信息密度**: 颜色 + 形状编码，远超标准 QR Code
2. **容错能力**: ECC + 喷泉码，丢帧不影响传输
3. **高性能**: 多进程并行，目标 > 106 KB/s
4. **易部署**: Go 单文件编译，无依赖问题
5. **跨平台**: Windows / Linux / macOS

### 14.2 关键里程碑
- **MVP** (Week 6): 基础传输，50+ KB/s
- **Phase 2** (Week 12): 优化版本，106+ KB/s
- **Phase 3** (Week 22): 完整功能，浏览器支持

### 14.3 成功标准
- ✓ 传输速度 > 106 KB/s
- ✓ 支持 MB 级文件传输
- ✓ Windows 平台稳定运行
- ✓ 单文件部署，无依赖问题
- ✓ 错误率 < 5% 时正常传输

---

**文档版本**: v1.0  
**最后更新**: 2026-06-04  
**作者**: AutoCambar Team
