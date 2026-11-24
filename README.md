# ancscan — SMPTE ANC/VANC 解析器

纯 Go 标准库实现的 SMPTE ST 291-1 ANC（Ancillary Data）数据包扫描工具与库。
它逐视频行搜索 Ancillary Data Flag，解析 DID / SDID(DBN) / DC / UDW(payload) /
CS，并校验 DID、SDID、DC 字的奇偶位与校验字。

设计前提正是题目描述的脏数据场景：**包内的 DID、SDID、DC、校验字可能与它所在的
场/行号“对不上”，也可能自身损坏**。因此本工具：

- 只相信字节，不按行号推断包类型；场（field）、行（line）、行内字节偏移
  （`byte_offset`）、字偏移与原始十六进制数据一律如实输出；
- 奇偶错误、校验和错误只打标签，不丢包，DID/payload 仍然解析保留；
- 未知 DID 照样保留 `did`、`payload_hex`、`raw_hex`，并标记 `unknown_did`；
- 区分 `valid_vanc`（合法 VANC）、`duplicate`（重复包）、
  `truncated`（行间截断）以及 `invalid`（完整但校验失败）。

## ANC 字格式（已实现的校验规则）

10-bit 接口表示（ST 291-1 / RFC 8331）：

```text
ADF = 000h, 3FFh, 3FFh
DID : b7..b0 = 数据号；b8 = b7..b0 的偶校验位；b9 = NOT b8
SDID/DBN、DC 同理（DC 计数只取低 8 位）
UDW × DC
CS  : b8..b0 = (DID + SDID/DBN + DC + 全部 UDW 的低 9 位之和) mod 512
      b9 = NOT CS_b8
```

8-bit 字节表示（`anc8`，ST 436M / MCC 风格）没有奇偶位，ADF 为
`00 FF FF`，CS 为 `DID+SDID+DC+payload` 的模 256 和。

## 分类口径

主分类 `classification` 是互斥的，优先级为：

1. `truncated`：按 DC 声明的长度，包在当前行结束前放不下（缺 UDW 或 CS），
   即“行间截断”。若相邻输入行开头恰好能补齐并通过校验，
   `continuation.reassembled=true`、`continuation.checksum_ok=true`，
   并带 `cross_line` / `reassembled_checksum_ok` 标签；主分类仍为 `truncated`，
   因为标准 ANC 包不允许跨视频行。
2. `duplicate`：后续出现的、与更早包的 `DID/SDID/DC/payload` 完全同内容的包。
   原始 10-bit 字序列也一致时 `duplicate_match="raw"`，否则
   `duplicate_match="content"`（例如第二个包奇偶位被打坏）。位置见
   `duplicate_of`。截断包不参与重复归类。
3. `invalid`：包完整，但奇偶位、CS 数值或 CS 帧位有错。
4. `valid_vanc`：奇偶（若存在）与校验全部正确。

所有“问题维度”还会独立放进 `labels`，例如 `parity_error`、`checksum_error`、
`checksum_framing_error`、`unknown_did`、`cross_line`、`did_line_mismatch`，
便于脚本按标签筛选而不必依赖单一分类。

## 快速开始

```sh
go test ./...
go build ./...

# 扫描内置示例（10 行，覆盖全部形态），输出 JSON 报告
go run . scan testdata/scenario.json

# 人类可读文本报告（中文分类名）
go run . scan testdata/scenario.json --format text

# 生成/重新生成示例输入
go run . gen-scenario --out testdata/scenario.json

# 简易“十六进制行”文本格式
go run . scan testdata/hexlines-example.txt --input-format hexlines --format text

# 原始二进制：按固定行宽切分后按 anc8（或 --encoding u16be）扫描
go run . scan capture.bin --input-format raw --encoding anc8 --line-bytes 1720

# 也可以从标准输入读取
cat testdata/scenario.json | go run ./cmd/ancscan scan -
```

仓库根的 `main.go` 与 `./cmd/ancscan` 是同一个 CLI。示例输出见
`artifacts/report.json` 与 `artifacts/report.txt`。

## 输入格式

支持一个 JSON 文档、JSON 数组或 JSONL（每行一个行记录）：

```json
{
  "schema": "ancscan/input-v1",
  "standard": "625i50",
  "encoding": "u16be",
  "continuation_lines": 1,
  "lines": [
    {"field": 1, "line": 22, "data_hex": "80808080...."},
    {"field": 1, "line": 23, "encoding": "anc8", "data_hex": "00 FF FF 61 01 01 41 A4"},
    {"field": 2, "line": 330, "words": [0, 1023, 1023, 353, 257, 260]}
  ]
}
```

| 编码 `encoding` | 行数据 | 奇偶校验 | 字节偏移含义 |
| --- | --- | --- | --- |
| `u16be`（默认） | 每个 10-bit 字占 2 字节，大端 | 完整校验 | ADF 在行字节中的真实偏移 |
| `u16le` | 同上，小端 | 完整校验 | 同上 |
| `words` | JSON 整数数组（0..1023） | 完整校验 | 规范化为大端 16-bit 后的 `2*word_offset` |
| `anc8` | `00 FF FF DID SDID DC ... CS8` | 无（报告 `absent`），CS 用模 256 和 | 真实字节偏移 |
| `v210` | V210 打包行（4 字节 3 个 10-bit 样本） | 完整校验 | ADF 首字所在字节 |

行记录同时接受 `hex`、`data_base64`/`base64`、`number`（作为 `line` 别名）。
`data_hex` 允许空格、冒号、逗号与 `0x` 前缀。

更简单的 `hexlines` 文本格式：每行一条视频行，可用 `F<场>-L<行>:` 前缀，
`#` 开头为注释，缺省行号取文本行号：

```text
F1-L21: 00 FF FF 61 01 01 41 A4
F2-L334: 00 FF FF CA 10 00 DA
```

## 输出字段

每个 `packets[]` 元素都包含定位、解析与原始证据：

```text
classification        valid_vanc | duplicate | truncated | invalid
labels[]              所有适用标签
field / line          包所在场（1/2；0 = 逐行或未知）与视频行号
byte_offset           ADF 在该行字节数据中的偏移
word_offset           字编码下的字偏移
stream_offset         可选的全局偏移（行 StreamOffset 基址 + byte_offset）
did / did_hex / did_name / unknown_did
id_type               type1_dbn（DID b7=1）或 type2_sdid
sdid_or_dbn / sdid_or_dbn_hex / sdid_name
dc                    数据计数（低 8 位）
payload_hex           解析出的 UDW 低 8 位（未知 DID 也保留）
udw[]                 完整 10-bit UDW 数值
raw_hex / raw_length  定位到的原始数据（ADF 到 CS，截断时到行尾）
offsets               ADF/DID/SDID/DC/payload/CS 各自的行内字节偏移
parity                DID/SDID/DC 的 ok | error | absent
checksum              模式、期望值/实际值、CS 帧位状态
duplicate_of          首个相同包的 field/line/byte_offset/...
continuation          跨行续写位置、取了多少字、是否补齐、校验是否通过
```

顶层 `summary` 给出各类计数。完整示例见 `artifacts/report.json`。

## Go API

```go
package main

import (
    "fmt"
    "ancscan/anc"
)

func main() {
    pkt, _ := anc.BuildWords(0x61, 0x01, []byte{0x01, 0x02}) // 自动奇偶+CS
    raw, _ := anc.SerializeWords(pkt, anc.EncU16BE)

    rep, err := anc.Scan([]anc.Line{{
        Field: 1, Number: 22, Bytes: raw, Encoding: anc.EncU16BE,
    }}, &anc.Options{
        ContinuationLines: 1,
        // ExpectedLines: map[byte][]int{0x61: {21, 22}}, // 可选行号建议
    })
    if err != nil { panic(err) }

    p := rep.Packets[0]
    fmt.Println(p.Classification, p.Field, p.Line, p.ByteOffset, p.DIDHex, p.PayloadHex)
}
```

也可用 `anc.ParseAndScan(reader, opts)` 直接消费 JSON/JSONL。
`anc.RegisterDID` 可追加单位内部 DID 名称表；解析与保留逻辑不依赖注册表。

## 目录

- `anc/`：无外部依赖的核心库（字/奇偶/校验、解码、扫描、分类、JSON IO、构造器）。
- `internal/cli/`：CLI 与文本渲染、示例场景生成。
- `cmd/ancscan/`、`main.go`：命令入口。
- `testdata/scenario.json`：覆盖合法、未知 DID、重复、奇偶/校验损坏、
  跨行补齐、下一行是新 ADF、无续写截断等形态的示例输入。
- `testdata/hexlines-example.txt`：十六进制行格式示例。
- `artifacts/report.json`、`artifacts/report.txt`：对示例输入的实际扫描结果。

## 校验与测试

```sh
gofmt -w .
go vet ./...
go test ./...
```

测试包含手工推导的 ST 291 向量
（`DID=61 SDID=01 DC=02 payload=AA 55` → DID 字 `161`、CS `263`）、
奇偶翻转、8-bit/10-bit 校验和、跨场重复、内容级重复、行间截断补齐与坏 CS、
下一行新 ADF、一行多包、V210 往返与偏移、JSON/JSONL/hexlines 输入和 CLI。

## 参考

- SMPTE ST 291-1:2011 Ancillary Data Packet and Space Formatting
- RFC 8331 — RTP Payload for SMPTE ST 291-1 Ancillary Data
- SMPTE ST 436M（8-bit ANC payload 与 wrapping/field 概念）
