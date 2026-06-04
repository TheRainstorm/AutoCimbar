# AutoCambar 实现要点总结

基于对 libcimbar 的研究和 technical_design.md 的详细方案，这里总结最关键的实现要点。

## 1. 核心技术：Image Hash 符号识别

### 原理
libcimbar 使用的方法非常简单但有效：
- 每个 8×8 tile 通过阈值化转为 64-bit 二进制数
- 16 个预定义符号，每对之间汉明距离约 20 bits
- 解码时计算汉明距离，选择最接近的符号

### 为什么有效
- 即使符号模糊、噪声干扰，汉明距离关系基本保持
- 只需要简单的位运算，性能极高
- libcimbar 在物理摄像头场景已验证可达 106 KB/s

## 2. 实现步骤建议

### Phase 1: MVP（4-6周）

#### Step 1: 符号准备
**选项 A（推荐）**：直接使用 libcimbar 的符号
```bash
# 从 libcimbar 仓库获取符号文件
git clone https://github.com/sz3/libcimbar
# 符号位置：libcimbar/bitmap/
# 共 16 个 8×8 PNG 文件
```

**选项 B**：自己设计符号
- 手工绘制约 40 个候选符号
- 计算每个符号的 image hash
- 选择汉明距离最大的 16 个

#### Step 2: Image Hash 实现
```go
// 核心函数
func computeImageHash(img image.Image) uint64 {
    // 1. 缩放到 8×8
    // 2. 转灰度
    // 3. 计算平均亮度作为阈值
    // 4. 每个像素 > 阈值 = 1，否则 = 0
    // 5. 生成 64-bit 数字
}

func hammingDistance(a, b uint64) int {
    return popcount(a ^ b)
}

func recognizeShape(cellImg image.Image) Shape {
    hash := computeImageHash(cellImg)
    minDist := 64
    bestShape := Shape(0)
    
    for i, refHash := range reference16Hashes {
        dist := hammingDistance(hash, refHash)
        if dist < minDist {
            minDist = dist
            bestShape = Shape(i)
        }
    }
    return bestShape
}
```

// __CONTINUE_HERE__