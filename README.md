# adlabel — 广告片语言/时长标签批量修复（纯 Go）

一批广告片的 `language`（语言）和 `duration`（时长）标签批量出错。本工具先建立
**字段权威链**，只修改白名单内的两个字段，保证**原始媒体哈希不变**，并以
**字段组事务**方式落地：一组字段要么一起成功，要么整体回滚。仅依赖 Go 标准库。

## 字段权威链

每个标签的取值可能来自多个来源，按可信度从弱到强排序：

```
filesystem（文件名/目录约定） < container（容器头标签） < bitstream（码流实测） < sidecar（人工策展）
```

- `filesystem` 只是线索，**永远不能**单独授权一次自动修正。
- 修正值先做规范化（语言归一到 ISO 639-1 主标签，如 `eng`/`en-US` → `en`；
  时长支持 `MM:SS`、`HH:MM:SS`、Go duration、裸秒数，统一为毫秒精度 `Ns`）。
- 规范化后必须与**最强可采信来源**一致才 `accepted`；与权威值矛盾 → `rejected`；
  来源之间或来源内部冲突、缺少可采信证据 → `manual`（人工复核，绝不猜测）。

## 允许修改的字段

白名单只有两个：`labels.language.value` 与 `labels.duration.value`（含其证据追加）。
其余所有顶层字段（`media_id`、`name`、`media_path`、`media_hash`、`created_at`
及任意未知/业务字段）在解码—编码往返中按原始 JSON 保留，写入前还有一轮
「受保护字段零变化」断言，任何一项变化都会中止该组。

## 事务与安全语义

- **字段组**：每个 `group_id` 必须恰好包含同一 `media_id` 的 language + duration
  两条修正；组形状非法时整个批次在任何写入前被拒绝。
- **逐组事务**：某组失败只回滚该组，其它已成功组保留；进程以退出码 1 暴露失败，
  部分修复不会被伪装成成功。
- **全有或全无**：`-all-or-nothing` 下任一组失败，已暂存提交的组也按原始字节还原。
- **原子写**：同目录临时文件 + `rename`，读者不会看到写一半的 sidecar。
- **媒体哈希**：媒体文件从不打开写；提交前后各计算一次 SHA-256，不一致即失败回滚。
- **只读文件**：写入前检查权限位，只读 sidecar 的组在预检阶段直接拒绝，
  不会借助 rename 绕过权限。
- **同名多语**：修正按 `media_id` 定位而非名称；报告附带同名其它 `media_id`，
  证明多语变体被正确区分、互不串改。

## 目录结构

```
cmd/adlabel        CLI 入口（sample / chain / dry-run / repair）
internal/catalog   sidecar 编解码、库加载、SHA-256、原子写
internal/authority 字段权威链、语言/时长规范化、裁决
internal/repair    分组校验、预检、暂存提交与回滚
internal/report    JSON 与可读报告、退出码
internal/fixture   确定性演示夹具生成
```

## 用法

```sh
# 1. 生成演示库（含同名多语、只读 sidecar、证据冲突、坏批次）
go run ./cmd/adlabel sample -out demo

# 2. 只看权威链裁决，不写任何文件
go run ./cmd/adlabel chain -library demo -corrections demo/batches/good.json

# 3. 预演：完整校验并报告将发生的变化
go run ./cmd/adlabel dry-run -library demo -corrections demo/batches/good.json

# 4. 执行：逐组事务，输出 JSON 报告
go run ./cmd/adlabel repair -library demo -corrections demo/batches/good.json \
  -report-json report.json

# 5. 全有或全无：一批同生共死
go run ./cmd/adlabel repair -library demo -corrections demo/batches/bad.json \
  -all-or-nothing
```

## 演示夹具覆盖的情形

- `ad-002` / `ad-003`：同名 `holiday-cheer` 的英语/日语多语变体，按 id 精确修复。
- `ad-004`：只读 sidecar（权限 `0444`），合法修正也被干净拦截。
- `ad-005`：bitstream 与 container 时长冲突（45s vs 47s），转人工不自动改。
- `bad.json`：非法语言、与权威值矛盾的时长，分别被 `rejected`。
- `malformed-group.json`：只有一个字段的组，写入前整体拒绝。

## 测试

```sh
go test ./... -count=1
go vet ./...
```

测试覆盖：权威链各裁决路径、分组形状、受保护字段往返、原子写、只读拦截、
同名多语隔离、逐组与全有或全无回滚、媒体哈希稳定。
