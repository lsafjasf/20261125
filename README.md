# ICC matrix/TRC 子集解析与对比工具

纯 Go 标准库实现（不依赖任何第三方包），解析并对比两个声称同型号的 ICC
显示器描述文件（一个 v2、一个 v4），定位白点、色调曲线和量化误差上的差异，
并标出超出声明 matrix/TRC 子集范围的 tag。

## 支持的子集

只解析题目声明的 matrix/TRC 子集，其余 tag 一律标记为 unsupported：

- 矩阵列：`rXYZ`、`gXYZ`、`bXYZ`、`wtpt`（类型 `XYZ `，s15Fixed16）
- 色调曲线：`rTRC`、`gTRC`、`bTRC`
  - `curv`：count=0（恒等）、count=1（u8Fixed8 gamma）、count>=2（u16Fixed16 LUT）
  - `para`：function type 0..4（s15Fixed16 参数，含 sRGB 常用的 type 3）
- 头部：版本号、设备类、色彩空间、PCS、`acsp` 签名、偏移 68 的 D50 参考白点

不在上述清单中的 tag（如 `desc`、`cprt`、`chad`、`mluc` 等）只记录签名、
偏移、长度，不解释其数据。

## 对比内容

- 白点：比较 `wtpt`（缺失时回退到头部参考白点），输出 XYZ 与 CIE76 ΔE
- 矩阵：逐列比较 r/g/b XYZ，给出最大绝对差
- 曲线：在 256 个等距采样点上逐点比较两条 TRC，输出 max / RMS / 平均绝对差
- 量化误差：当一侧是 `curv` LUT、另一侧是解析曲线（gamma/para）时，在 LUT
  节点上比较编码值与解析参考值，输出 max / RMS

## 异常输入处理

解析不中断，问题收集到报告的 issues 中：

- 重复 tag：记录重复签名与首次出现位置（重复项共享同一数据区不算重叠）
- 错误长度：XYZ 必须 20 字节；curv/para 长度须与 count/function type 匹配
- 非法曲线：非正 gamma、LUT 越界或非单调、para function type 超出 0..4
- 越界 tag：偏移/长度超出文件范围、与其他 tag 数据区重叠

## 构建与运行

```bash
go build ./...
go test ./...

# 生成测试用合成 ICC（含重复 tag、错误长度、非法曲线、越界等异常样本）
go run ./cmd/genfixture testdata

# 文本报告
go run ./cmd/icccmp testdata/v2_ok.icc testdata/v4_ok.icc

# JSON 报告
go run ./cmd/icccmp --json testdata/v2_ok.icc testdata/v4_ok.icc
```

## 目录结构

- `icc/`：解析器、曲线求值、对比与量化逻辑，以及单元测试
- `icc/fixture/`：纯标准库的合成 ICC 构造器（测试用）
- `cmd/icccmp/`：CLI（文本与 JSON 输出）
- `cmd/genfixture/`：异常样本生成命令
- `testdata/`：生成的合成 ICC 文件
