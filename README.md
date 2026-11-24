# colorgraph — 色彩转换图：拓扑排序 / 循环检测 / 未使用节点 / 输入色彩空间预检

后期团队维护一张色彩转换图：节点是调色、矩阵、混合、格式转换、编码等操作，
节点之间通过**全局输出句柄（output handle）**或节点 id 接线。图在长期维护中
可能被接入**循环依赖**，也可能有两个节点**声明了同名输出（重复输出）**。

本工具在执行任何测试帧之前完成：

1. **拓扑排序**：Kahn 算法 + 最小 id 确定性平局选择，输出稳定顺序；
2. **真正循环检测**：Tarjan 强连通分量（SCC）+ 闭环路径还原，区分
   - `[合法分叉 fork]`：同一个输出被 ≥2 个消费者使用（一对多，合法）；
   - `[合法合并 merge]`：一个节点有 ≥2 个前驱输入（多对一，合法）；
   - `[真正循环 cycle]`：依赖成环（非法，退出码 2，阻止执行）；
3. **重复输出检测**：同名 handle 被多个节点声明时给出 `duplicate output`
   及全部声明节点，并判为非法；
4. **未使用节点检测**：以请求目标（`targets`，缺省为所有末端 sink）做反向
   可达，不在目标依赖链上的节点逐条给出原因，并列出无人引用的输出句柄；
5. **执行前说明每个节点实际采用的输入色彩空间**：逐输入列出来源
   （上游输出声明 / 输入显式假定 / 图默认值 / 内置 sRGB 兜底）、上游空间、
   节点实际采用空间，以及是否需要隐式转换；
6. 仅使用 **Go 标准库**，并在仓库自带的 3 张测试帧上按拓扑序真正执行像素处理。

## 仓库提供的图配置与测试帧

- `configs/color_graph.json` — 主示例：解码后一分二调色（fork）→ 加权混合
  （merge）→ 空间转换 → 编码交付；另含一条完整的未使用备份支路和一个游离节点。
- `configs/diamond.json` — 最小“分叉 + 合并”菱形（无未使用节点）。
- `configs/cycle.json` — `grade_a -> blur_b -> grade_c -> grade_a` 真循环。
- `configs/duplicate_outputs.json` — 两个源节点重复声明句柄 `plate_a`。
- `frames/input/frame_0001.png … frame_0003.png` — 标准库生成的 96×64
  测试帧（彩条 + 灰阶 + 逐帧移动色块）。可用 `go run ./cmd/genframes` 重新生成。

## 快速开始

```bash
go test ./...          # 全套单元/端到端测试（27 个顶层测试函数）
bash acceptance.sh     # 单测 + 全部正/负场景 + 三帧执行的完整复测脚本

go run .               # 无参数 = 用主配置处理 frames/input，输出到 colorgraph-out/

go run . analyze -config configs/color_graph.json        # 文本预检报告
go run . plan    -config configs/color_graph.json -json  # 结构化 JSON
go run . run     -config configs/color_graph.json \
                 -frames frames/input -out colorgraph-out
```

建议先 `go build -o colorgraph .` 再直接运行二进制：非法图返回退出码 `2`
（`go run` 会把非零退出统一打印成 `exit status 2` 且外层返回 1）。

退出码：`0` 合法且成功；`2` 图不合法（循环 / 重复输出 / 悬空引用 /
色彩空间冲突），此时 `run` 只写预检报告、绝不处理帧；`1` 为 IO/执行错误。

## 预检报告长什么样

`analyze`/`run` 的 stdout 与 `-out/preflight.txt` 包含下列固定可检索标记
（中英双语，便于脚本校验）：

```text
【拓扑排序 / Topological order】
  plate -> decode_linear -> grade_warm -> grade_cool -> blend_grades -> ...
【结构判定 / Structure: 合法分叉 fork · 合法合并 merge · 真正循环 cycle】
  [合法分叉 fork ] 节点 decode_linear（输出 scene_linear） 一对多供给 [...] —— ...合法。
  [合法合并 merge] 节点 blend_grades 汇聚来自 [grade_cool grade_warm] ... —— ...合法。
  [真正循环 cycle] grade_a -> blur_b -> grade_c -> grade_a —— ...非法...
【重复输出 / Duplicate outputs】
  [重复输出 duplicate output] 句柄 "plate_a" 同时由节点 [...] 声明 ...
【未使用节点 / Unused nodes】
  [未使用 unused] backup_encode (type=encode) —— 末端节点，输出无人消费且未被列为目标。
【执行前：每个节点实际采用的输入色彩空间 / Effective input color space per node】
      上游色彩空间 incoming="sRGB" (取自上游输出声明)；节点实际采用 adopted="Linear sRGB"；
      执行前插入隐式转换 sRGB -> Linear sRGB
```

`-json`（analyze）或 `-out/preflight.json`（run）给出同一份机器可读报告，
字段包括 `valid`、`topologicalOrder`、`forks`、`merges`、`cycles`、
`duplicateOutputs`、`unusedNodes`、`unusedOutputs`、`targets`、
`colorPlan[].inputs[]`、`diagnostics[]`。

## 配置格式（宽容解析）

```jsonc
{
  "name": "di-dailies-grade",
  "defaultColorSpace": "sRGB",      // 未声明时的兜底；也接受 default_space
  "autoConvert": true,              // 空间不一致时插入隐式转换，默认 true
  "frames": "frames/input",         // 目录或 glob
  "targets": ["deliver"],           // 节点 id 或输出句柄；也接受单个字符串/outputs
  "nodes": [
    {
      "id": "decode_linear",        // 也接受 name
      "type": "convert",            // 也接受 op / kind
      "inputs": [
        { "name": "encoded", "from": "plate_rgb", "expect": "Linear sRGB" }
        // 或直接写 "plate_rgb"；from 可以是输出句柄或节点 id
      ],
      "outputs": [
        { "name": "scene_linear", "space": "Linear sRGB" }
        // 或直接写 "scene_linear"
      ],
      "to": "Linear sRGB"           // 未嵌套在 params 里的键也会作为算子参数
    }
  ],
  "edges": [["src", "dst"]]         // 可选：显式边，也接受 {"from":"a","to":"b"}
}
```

兼容的别名字段：输入 `from/ref/source/output/handle/producer/node`、
`space/colorSpace/assumeSpace`、`expect/targetSpace/workingSpace`；
输出 `name/handle/id/key` + `space/colorSpace`。输入/输出都允许“单个字符串、
单个对象、数组”三种写法。

## 关键判定语义

- **重复输出**：全局 handle 名被 ≥2 个节点的 `outputs` 声明即报错
  `duplicate_output`，因为下游按 handle 引用时来源不唯一。
- **边的来源**：输入引用优先匹配输出 handle，其次匹配节点 id（单输出节点可
  省略 socket；多输出未指定 socket 给 `ambiguous_socket` 警告并取第一个）。
  `edges` 中的显式边会与输入推导出的边去重合并。
- **拓扑序**：Kahn 入度清零，就绪集合按 id 字典序取最小，保证多次运行/多个
  平台输出完全一致。有环时返回环外前缀与 `*CycleError`。
- **循环 vs 分叉/合并**：Tarjan SCC 中大小 >1 或存在自环的分量才是
  “真正循环”，并在分量内还原一条闭合路径；普通的扇出（一个 socket ≥2 个
  消费者）记为 fork，前驱数 ≥2 的节点记为 merge，二者都是合法 DAG 结构。
- **未使用节点**：`targets` 为空时取所有出度为 0 的节点；从目标沿反向边
  BFS 得到使用集合，其余节点为 unused。`run` 只执行使用集合内的节点，因此
  未使用分支即使算子参数不完整也不会影响交付目标。
- **实际输入色彩空间**，对每条输入按以下优先级确定“上游空间”：
  1. 上游输出声明的 `space`（`output-declared`）；
  2. 输入对象显式假定的 `colorSpace/assumeSpace`（`input-assumption`）；
  3. 图级 `defaultColorSpace`（`config-default`）；
  4. 内置 `sRGB`（PNG 惯例，`built-in-default`）。

  若该输入或所在 merge 节点还要求 `expect/workingSpace`，且与上游空间不同：
  `autoConvert=true`（默认）时预检会写明 `X -> Y` 的隐式转换，执行器在取数
  阶段真正做这次转换；`autoConvert=false` 时记 `color_space_mismatch` 错误并
  阻止执行。支持 `sRGB`、`Linear sRGB`、`BT.709`、`Linear BT.709`、
  `Display P3`、`Linear Display P3`，转换经过传递曲线 + XYZ 三基色矩阵
  （基色矩阵由 xy 色度坐标推导，带标准值单测）。

## 帧执行与产物

`run` 先把完整文本预检打印到 stdout 并写入 `<out>/preflight.txt` /
`preflight.json`，确认合法后才枚举 `-frames` 目录或 glob 中的 `.png`/`.jpg`
（按文件名自然排序，`frame_2` 在 `frame_10` 前），逐帧按拓扑序执行（PNG 与 JPEG 均可读入）：

- 算子：`load/source`、`convert/colorspace`、`gain`（含 `exposure`、`rgb`
  通道增益）、`matrix`（3×3 + offset）、`gamma`、`clamp`、
  `mix/merge/blend`（`average`/`weighted`/`sum`，多输入自动统一工作空间）、
  `encode/sink/passthrough`。
- 每个节点产出的缓冲会转换到其 `outputs[0].space`（或 `params.to`），因此
  `convert` 节点只需声明目标空间即可完成真正的像素转换。
- 每个目标节点每帧写出 `<frame>__<target>.png`（统一渲染为 sRGB 预览），
  并写 `run-report.json`（执行节点序列、跳过的未使用节点、输出尺寸/均值）。

## 代码地图

| 文件 | 职责 |
| --- | --- |
| `types.go` | 配置模型与宽容 JSON 解析（别名/单复数/扁平参数） |
| `graph.go` | 建图、handle 注册、重复输出与悬空引用检测 |
| `topology.go` | Kahn 拓扑序、Tarjan SCC、环路径、fork/merge/unused |
| `plan.go` / `render.go` | 输入色彩空间解析、报告模型与双语文本渲染 |
| `colorspace.go` / `mat3.go` | 色彩空间注册表、传递曲线、基色矩阵与像素转换 |
| `frame.go` | Frame、PNG 读写、自然排序、帧枚举 |
| `execute.go` | 拓扑执行器与全部像素算子 |
| `main.go` / `config.go` | CLI（analyze/plan/run）、退出码、配置加载 |
| `cmd/genframes` | 重新生成 `frames/input` 测试帧 |
| `*_test.go` | 27 个顶层测试：拓扑/循环/重复/未使用/色彩数学/预检/端到端/CLI |

仅依赖 Go 标准库（`go.mod` 无 require）。
