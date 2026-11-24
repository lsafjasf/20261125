package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// RenderText renders the preflight analysis in deterministic, grep-friendly
// bilingual text. It is printed before any frame is executed.
func (r *Report) RenderText() string {
	var b strings.Builder
	title := "色彩转换图预检 · Color Graph Preflight"
	if r.Name != "" {
		title += " — " + r.Name
	}
	fmt.Fprintln(&b, title)
	fmt.Fprintln(&b, strings.Repeat("=", utf8.RuneCountInString(title)+4))
	fmt.Fprintf(&b, "默认输入色彩空间 / default input space: %s\n", r.DefaultSpace)
	fmt.Fprintf(&b, "隐式转换 / autoConvert: %t\n", r.AutoConvert)

	r.renderStructure(&b)
	r.renderDuplicates(&b)
	r.renderUnused(&b)
	r.renderColorPlan(&b)
	r.renderDiagnostics(&b)

	if r.Valid {
		fmt.Fprintln(&b, "结论 / verdict: 图有效 (valid)，拓扑可排序，可以执行测试帧。")
	} else {
		fmt.Fprintln(&b, "结论 / verdict: 图无效 (invalid)，存在上述错误；执行已阻止，未处理任何帧。")
	}
	return b.String()
}

func (r *Report) renderStructure(b *strings.Builder) {
	fmt.Fprintln(b)
	fmt.Fprintln(b, "【拓扑排序 / Topological order】")
	if len(r.TopologicalOrder) == 0 {
		fmt.Fprintln(b, "  (无节点)")
	} else {
		fmt.Fprintln(b, "  "+strings.Join(r.TopologicalOrder, " -> "))
	}
	if len(r.Cycles) > 0 {
		fmt.Fprintln(b, "  注意：图中存在循环，以上仅为环外前缀，不是完整合法拓扑序。")
	}

	fmt.Fprintln(b)
	fmt.Fprintln(b, "【结构判定 / Structure: 合法分叉 fork · 合法合并 merge · 真正循环 cycle】")
	if len(r.Forks) == 0 && len(r.Merges) == 0 && len(r.Cycles) == 0 {
		fmt.Fprintln(b, "  无分叉、合并或循环。")
	}
	acyclic := len(r.Cycles) == 0
	for _, f := range r.Forks {
		handle := ""
		if f.Handle != "" {
			handle = "（输出 " + f.Handle + "）"
		}
		if acyclic {
			fmt.Fprintf(b, "  [合法分叉 fork ] 节点 %s%s 一对多供给 %v —— 同一输出被多个消费者使用，是合法分叉，不是循环。\n",
				f.Source, handle, f.Consumers)
		} else {
			fmt.Fprintf(b, "  [分叉 fork/含环] 节点 %s%s 一对多供给 %v —— 扇出形态本身不是循环，但本图另有真循环，整图仍非法。\n",
				f.Source, handle, f.Consumers)
		}
	}
	for _, m := range r.Merges {
		if acyclic {
			fmt.Fprintf(b, "  [合法合并 merge] 节点 %s 汇聚来自 %v 的多路输入 —— 多对一合并，是合法合并，不是循环。\n",
				m.Node, m.Producers)
		} else {
			fmt.Fprintf(b, "  [合并 merge/含环] 节点 %s 汇聚来自 %v 的多路输入 —— 汇聚形态本身不是循环，但本图另有真循环，整图仍非法。\n",
				m.Node, m.Producers)
		}
	}
	for _, c := range r.Cycles {
		fmt.Fprintf(b, "  [真正循环 cycle] %s —— 依赖成环，非法，无法拓扑排序/执行。\n", joinPath(c.Path))
	}
}

func (r *Report) renderDuplicates(b *strings.Builder) {
	fmt.Fprintln(b)
	fmt.Fprintln(b, "【重复输出 / Duplicate outputs】")
	if len(r.DuplicateOutputs) == 0 {
		fmt.Fprintln(b, "  无重复输出声明。")
		return
	}
	for _, d := range r.DuplicateOutputs {
		fmt.Fprintf(b, "  [重复输出 duplicate output] 句柄 %q 同时由节点 %v 声明，接线存在歧义。\n",
			d.Handle, d.DeclaredBy)
	}
}

func (r *Report) renderUnused(b *strings.Builder) {
	fmt.Fprintln(b)
	fmt.Fprintln(b, "【未使用节点 / Unused nodes】")
	if len(r.UnusedNodes) == 0 {
		fmt.Fprintln(b, "  无未使用节点；所有节点都在目标输出的依赖链上。")
	}
	for _, u := range r.UnusedNodes {
		fmt.Fprintf(b, "  [未使用 unused] %s (type=%s) —— %s。\n", u.ID, u.Type, u.Reason)
	}
	if len(r.UnusedOutputs) > 0 {
		fmt.Fprintf(b, "  未被引用的输出句柄 / unused handles: %s\n", strings.Join(r.UnusedOutputs, ", "))
	}
	fmt.Fprintf(b, "  请求目标 / targets: %s\n", strings.Join(r.Targets, ", "))
}

func (r *Report) renderColorPlan(b *strings.Builder) {
	fmt.Fprintln(b)
	fmt.Fprintln(b, "【执行前：每个节点实际采用的输入色彩空间 / Effective input color space per node】")
	for _, np := range r.ColorPlan {
		if len(np.Inputs) == 0 {
			noInputNote := "无输入（源节点）"
			if node, ok := r.graph.NodeMap[np.Node]; ok && !isSourceLike(node.Type) {
				noInputNote = "无输入（孤立节点；因未使用，执行时跳过）"
			}
			outputs := []string{}
			for _, o := range np.Outputs {
				if o.Space != "" {
					outputs = append(outputs, fmt.Sprintf("%s=%s", o.Name, o.Space))
				} else {
					outputs = append(outputs, o.Name)
				}
			}
			fmt.Fprintf(b, "  - 节点 %s (type=%s): %s；产出 %s\n",
				np.Node, np.Type, noInputNote, strings.Join(outputs, ", "))
			continue
		}
		fmt.Fprintf(b, "  - 节点 %s (type=%s):\n", np.Node, np.Type)
		for _, ip := range np.Inputs {
			label := ip.InputName
			if label == "" {
				label = ip.Ref
			}
			origin := ip.FromNode
			if ip.Handle != "" {
				origin += "." + ip.Handle
			}
			fmt.Fprintf(b, "      输入 %q <- %s\n", label, origin)
			fmt.Fprintf(b, "          上游色彩空间 incoming=%q (%s)；节点实际采用 adopted=%q",
				ip.IncomingSpace, resolutionCN(ip.Resolution), ip.AdoptedSpace)
			if ip.AutoConverted && ip.Conversion != "" {
				fmt.Fprintf(b, "；执行前插入隐式转换 %s", ip.Conversion)
			}
			if ip.Status != "ok" {
				fmt.Fprintf(b, "；状态=%s（%s）", ip.Status, ip.Message)
			}
			fmt.Fprintln(b)
		}
		if np.WorkingSpace != "" {
			fmt.Fprintf(b, "      该节点统一工作色彩空间 / working space: %q\n", np.WorkingSpace)
		}
	}
}

func resolutionCN(code string) string {
	switch code {
	case "output-declared":
		return "取自上游输出声明"
	case "input-assumption":
		return "取自输入显式假定"
	case "config-default":
		return "取自图默认值"
	case "built-in-default":
		return "未声明，按 PNG 惯例取内置默认 sRGB"
	}
	return code
}

func (r *Report) renderDiagnostics(b *strings.Builder) {
	var errs, warns []Diagnostic
	for _, d := range r.Diagnostics {
		switch d.Severity {
		case "error":
			errs = append(errs, d)
		case "warning":
			warns = append(warns, d)
		}
	}
	if len(warns) > 0 {
		fmt.Fprintln(b)
		fmt.Fprintln(b, "【警告 / Warnings】")
		for _, d := range warns {
			fmt.Fprintf(b, "  [warn:%s] %s\n", d.Code, d.Message)
		}
	}
	if len(errs) > 0 {
		fmt.Fprintln(b)
		fmt.Fprintln(b, "【错误 / Errors】")
		for _, d := range errs {
			fmt.Fprintf(b, "  [error:%s] %s\n", d.Code, d.Message)
		}
	}
}
