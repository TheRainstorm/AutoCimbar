# 从 libcimbar 学到的关键技术

## 1. 符号设计原理

### 1.1 核心思想：Image Hashing
- cimbar 使用 **阈值化 image hash** 作为符号识别基础
- 8×8 的 tile 转换为 64-bit 二进制数（左到右，上到下）
- 每个像素：亮 = 1，暗 = 0

### 1.2 符号集选择标准
- **汉明距离**：每个符号的 image hash 需要与其他所有符号保持足够的汉明距离
- **cimbar 实现**：16 个符号，彼此间约 20 bits 汉明距离
- **关键特性**：即使符号模糊或损坏，汉明距离关系仍然基本保持

### 1.3 符号生成方法
根据 ABOUT.md：
- 作者手工在 kolourpaint 绘制了约 40 个候选符号
- 通过实验筛选出最终的 16 个符号
- 选择标准：image hash 汉明距离最大化

**改进空间**：
- 使用遗传算法自动生成（参考 cimbar-tiles-generator 项目）
- 理论上 8×8 tile 可以支持 32 个不同符号（5 bits）

## 2. 编码与解码流程

### 2.1 编码流程（伪代码）
```python
for bits in error_correction(file):
    for x, y in next_position():
        img.paste(cimbar_tile(bits), x, y)
```

### 2.2 解码流程（伪代码）
```python
for i, bits, distance, drift in next_decode():
    results[deinterleave(i)] = bits
    position_tracker.update(i, drift, distance)

decoded_data = error_correct(results)
```

**关键概念**：
- **distance**：image hash 距离，越小越可信（用作置信度）
- **drift**：(x, y) 偏移，跟踪局部畸变（最大 ±7px）
- **优先级解码**：先解码高置信度的 cell，用其 drift 指导低置信度 cell
- **去交织（deinterleave）**：恢复数据原始顺序

### 2.3 Image Hash 识别原理
解码时：
1. 对每个 tile 计算 image hash（64-bit）
2. 与 16 个参考符号的 hash 计算汉明距离
3. 选择距离最小的符号
4. 如果最小距离明显优于其他候选，则解码成功

## 3. 性能数据

### 3.1 cimbar 规格
- **图像尺寸**：1024×1024 像素
- **Tile 尺寸**：8×8 像素，在 9×9 网格中（两侧各留 1 行/列间距）
- **数据容量**：
  - 原始：9300 bytes（4 色模式）
  - ECC 30/155 后：7500 bytes
- **实测速度**：
  - mode B (8×8 4-color): **852 kbps (~106 KB/s)**
  - mode 8C (8×8 8-color, 已弃用): 943 kbps (~118 KB/s)

### 3.2 关键性能因素
- **瓶颈通常是摄像头**，不是 CPU
- **光照很重要**：白色背景 + 良好环境光
- **图像质量**：最低 700×700，推荐更高
- **角度**：正面拍摄优于斜角

## 4. 错误处理机制

### 4.1 Reed-Solomon 纠错
- 默认 ECC 30/155：每 125 bytes 真实数据 + 30 bytes 纠错
- **不完美**：RS 纠正字节错误，但 cimbar 错误通常是 1-3 bits
- 改进空间：使用 bit-level 纠错（如 LDPC）

### 4.2 交织（Interleaving）
- ECC 应用在相邻字节，但图像错误倾向于聚集
- 解决：将 ECC 块分散到图像各处（跳过 N 个 cells）
- 目的：局部遮挡不会破坏整个 ECC 块

### 4.3 喷泉码（Fountain Codes - Wirehair）
- 支持大文件（>7500 bytes）传输
- **无序接收**：帧可以乱序到达
- **容错**：只需接收 N+1 帧（N = file_size / bytes_per_frame）
- **限制**：cimbar 限制文件 ≤ 33.55 MB（需全部在 RAM）

## 5. 颜色编码

### 5.1 实践经验
- **4 色（2 bits）**：完全可行，dark 和 light mode 都可以
- **8 色（3 bits）**：可能，至少在 dark mode 可以
- **16 色（4 bits）**：当前颜色解码逻辑无法实现

### 5.2 颜色约束
- 颜色必须与背景色有足够对比度
- 例如：蓝色（0x0000FF）在 dark mode 会与黑色混淆
- 颜色混淆 → 符号解码失败 → 错误率暴增

### 5.3 颜色解码
- **非常简单**的逻辑，几乎没有色彩校正
- 依赖相机自动曝光、自动白平衡

## 6. 对 AutoCambar 的启示

### 6.1 可以直接借鉴的
1. **Image hash 方法**：简单有效，适合我们的数字信道
2. **优先级解码 + drift tracking**：即使我们没有定位标记，局部畸变仍可能存在
3. **交织策略**：防止局部错误聚集
4. **喷泉码**：支持大文件和无序接收

### 6.2 我们的优势
1. **数字信道**：无需担心光照、角度、抖动
2. **无需定位**：手动指定区域，100% 空间用于数据
3. **更高帧率**：不受摄像头限制，可以 60+ FPS
4. **更大 Grid**：屏幕分辨率允许，可以用 Q=50-60

### 6.3 需要实现的关键点
1. **符号集生成**：
   - 可以参考 cimbar 的 16 个符号
   - 或使用遗传算法生成新的符号集
   - 确保汉明距离 ≥ 20 bits
   
2. **Image Hash 解码器**：
   - 实现 8×8 阈值化 hash
   - 预计算 16 个参考符号的 hash
   - 解码时计算汉明距离选择最佳匹配
   
3. **颜色识别**：
   - 从 4 色（2 bits）开始
   - 选择高对比度颜色：黑、白、红、蓝
   - 避免与背景色混淆
   
4. **优先级解码**（可选，Phase 2）：
   - 按置信度排序解码
   - Drift tracking 用于局部畸变修正

### 6.4 符号存储格式
- 存储为 8×8 的二值图像（64 bits）
- 或存储为 8×8 的模板图像（PNG/二进制）
- 预计算所有符号的 image hash

## 7. 实现建议

### Phase 1: 简化实现
- 直接使用 cimbar 的 16 个符号（如果开源可用）
- 4 色模式（2 bits）
- 简单的逐 cell 解码（不需要优先级）
- 标准 Reed-Solomon ECC

### Phase 2: 优化
- 实现 drift tracking
- 优化颜色识别算法
- 考虑 8 色模式（3 bits）

### Phase 3: 进阶
- 遗传算法生成新的符号集
- 尝试 5 bits 符号（32 个符号）
- 实现 bit-level ECC

## 8. 参考资源

- **libcimbar 源码**：https://github.com/sz3/libcimbar
- **cimbar 研究版**：https://github.com/sz3/cimbar
- **Image hash 库**：https://github.com/JohannesBuchner/imagehash
- **Tile 生成器**：https://github.com/mihaigalos/cimbar-tiles-generator
- **Android 接收端**：https://github.com/sz3/cfc

## 9. 符号 Tile 文件位置

在 libcimbar 源码中：
- `bitmap/` 目录：存储 tile PNG 文件
- 可以直接复用这些符号定义
