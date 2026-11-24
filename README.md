# lut3d

严格、纯 Go 标准库的 3D CUBE LUT 解析与三线性插值实现。不依赖颜色库。

解析是全有或全无的：只有头、域、步长、`N³` 表项和每一行数值全部通过后，才会返回 `*LUT`；失败时返回 `nil` 和带 1-based 行号的 `*ParseError`。

## 支持范围

```text
TITLE "optional title"
UNIT
DOMAIN_MIN rmin gmin bmin
DOMAIN_MAX rmax gmax bmax
LUT_3D_INPUT_RANGE lo hi
LUT_3D_SIZE N
r g b
... exactly N*N*N rows
```

- `UNIT` 表示三个轴都是 `[0,1]`，也是省略域声明时的默认值。
- `DOMAIN_MIN` / `DOMAIN_MAX` 支持逐轴不同的矩形域，例如 `[0.1,0.9] × [0.2,1.2] × [0.4,1.6]`。每个轴内部仍是等步长；本实现不支持任意不等距节点数组。
- `LUT_3D_INPUT_RANGE lo hi` 是覆盖三个轴的统一范围，便于兼容 Resolve 风格文件。
- `UNIT`、显式 `DOMAIN_MIN`/`DOMAIN_MAX`、`LUT_3D_INPUT_RANGE` 三者互斥。
- 数据顺序遵循 CUBE 约定：R 变化最快，其次 G，最后 B；`(r,g,b)` 位于 `r + N*(g + N*b)`。
- 允许空行、整行注释和行尾 `#` 注释；接受 UTF-8 BOM 与 CRLF。
- `N` 必须为 `2..256`，必须有有限十进制浮点数值，并且行数必须恰好为 `N³`。

## 严格拒绝

- 未知指令、重复或冲突的头、表项开始后出现的指令。
- 表项在 `LUT_3D_SIZE` 之前出现。
- `DOMAIN_MIN`/`DOMAIN_MAX` 只出现一个、重复出现，或任一轴 `min >= max`。
- 行内不是恰好 3 个值、数字非法或不是有限值、表项少于/多于 `N³`。
- 1D LUT、非 UTF-8 输入、超过行长或尺寸安全上限的输入。

所有语义错误都带出错行。例如：

```text
$ go run ./cmd/lut3d testdata/inconsistent.cube
testdata/inconsistent.cube: line 13: LUT_3D_SIZE 3 requires 27 entries, got 8
```

## 插值与手算参考

输入先按各轴映射到 `[0,1]`，再取包围它的 8 个格点做三线性插值。超出域的值钳制到最近边界面；任一输入为 NaN 时结果为 NaN。

`testdata/valid_nonuniform.cube` 使用：

```text
R domain [0.1,0.9]   G domain [0.2,1.2]   B domain [0.4,1.6]
2 × 2 × 2 非仿射角点
```

采样 `(0.3,0.7,1.3)` 得到归一化坐标 `(u,v,w)=(0.25,0.5,0.75)`。八个角点权重为：

```text
000 0.09375   100 0.03125   010 0.09375   110 0.03125
001 0.28125   101 0.09375   011 0.28125   111 0.09375
```

按这组权重和文件中角点值逐通道加权，手算结果为 `(19.09375, 4.25, 4.5)`：

```text
$ go run ./cmd/lut3d -r 0.3 -g 0.7 -b 1.3 testdata/valid_nonuniform.cube
valid "hand calculation": size=2 entries=8
R domain=[0.1,0.9] step=0.8
G domain=[0.2,1.2] step=1
B domain=[0.4,1.6] step=1.2000000000000002
lookup(0.3,0.7,1.3)=(19.09375,4.25,4.5)
```

## Go API

```go
lut, err := lut3d.ParseFile("look.cube")
if err != nil {
    // err is a *lut3d.ParseError; err.Line is the failing source line.
}
out := lut.Lookup(lut3d.RGB{R: 0.3, G: 0.7, B: 1.3})
```

也可使用 `Parse(io.Reader)`、`ParseBytes([]byte)` 和 `Sample(r,g,b)`。

## 验证

```sh
go test ./...
go vet ./...
```

测试覆盖单位域、逐轴非均匀域、2³/3³ 插值、R 最快表序、边界钳制、NaN、BOM/CRLF/注释，以及重复头、域冲突、非法数字、数据先于尺寸、表项截断/超量等拒绝路径。
