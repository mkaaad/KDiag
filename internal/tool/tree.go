package tool

import (
	"fmt"
	"slices"
	"strings"
)

// nodeCounter is used by RegisterExpandNodes and AddNodeTool to generate
// unique node IDs.
var nodeCounter int

// NodeType identifies the kind of information a tree node represents.
type NodeType string

const (
	NodeTrace  NodeType = "trace"
	NodeSpan   NodeType = "span"
	NodeLog    NodeType = "log"
	NodeMetric NodeType = "metric"
	NodeLogs   NodeType = "logs"
)

// TreeNode is a single node in the correlation information tree. Children hold
// the IDs of sub-nodes. Expansion fields (Service, TraceID, etc.) are
// populated by BuildContext and consumed by ExpandNodeTool.
type TreeNode struct {
	ID       string            `json:"id"`
	Type     NodeType          `json:"type"`
	Summary  string            `json:"summary"`
	Children []string          `json:"children,omitempty"`
	Meta     map[string]string `json:"meta,omitempty"`

	// Expansion details.
	Service    string `json:"service,omitempty"`
	Operation  string `json:"operation,omitempty"`
	TraceID    string `json:"trace_id,omitempty"`
	SpanID     string `json:"span_id,omitempty"`
	Query      string `json:"query,omitempty"`
	StartMicro int64  `json:"start_micro,omitempty"`
	EndMicro   int64  `json:"end_micro,omitempty"`
	LokiQuery  string `json:"loki_query,omitempty"`
}

// InfoTree holds all nodes discovered during correlation context building.
type InfoTree struct {
	Nodes []*TreeNode `json:"nodes"`
}

// Format serializes the tree as compact Markdown. Each node line ends with
// its node ID in backticks, which the agent passes to ExpandNode.
func (t *InfoTree) Format() string {
	if len(t.Nodes) == 0 {
		return ""
	}

	var b strings.Builder
	byID := make(map[string]*TreeNode, len(t.Nodes))
	childSet := make(map[string]bool, len(t.Nodes))
	for _, n := range t.Nodes {
		byID[n.ID] = n
		for _, c := range n.Children {
			childSet[c] = true
		}
	}

	// Group roots by type order: metrics → traces → logs.
	type ordered struct {
		label string
		nodes []*TreeNode
	}
	groups := []ordered{
		{"Metrics", nil},
		{"Traces", nil},
		{"Logs", nil},
	}
	groupIdx := map[NodeType]int{NodeMetric: 0, NodeTrace: 1, NodeLogs: 2}
	for _, n := range t.Nodes {
		if childSet[n.ID] {
			continue
		}
		idx := 0
		if i, ok := groupIdx[n.Type]; ok {
			idx = i
		}
		groups[idx].nodes = append(groups[idx].nodes, n)
	}

	b.WriteString("\n\n## 关联信息树\n")
	b.WriteString("使用 ExpandNode 工具展开任意节点查看详情。\n\n")

	for gi, g := range groups {
		if len(g.nodes) == 0 {
			continue
		}
		// Section header.
		header := "├─"
		if gi == len(groups)-1 {
			header = "└─"
		} else {
			// Check if all remaining groups are empty.
			allEmpty := true
			for _, gg := range groups[gi+1:] {
				if len(gg.nodes) > 0 {
					allEmpty = false
					break
				}
			}
			if allEmpty {
				header = "└─"
			}
		}
		// └─ for last non-empty group.
		isLast := true
		for _, gg := range groups[gi+1:] {
			if len(gg.nodes) > 0 {
				isLast = false
				break
			}
		}
		if isLast {
			header = "└─"
		}

		fmt.Fprintf(&b, "%s %s ───────────────────────\n", header, g.label)
		for ni, n := range g.nodes {
			nPrefix := "  ├"
			if ni == len(g.nodes)-1 {
				nPrefix = "  └"
			}
			t.writeTreeNode(&b, byID, n, nPrefix, "")
		}
		b.WriteString("\n")
	}

	b.WriteString("使用方式: ExpandNode({\"node_id\":\"节点ID\"})\n")
	return b.String()
}

func (t *InfoTree) writeTreeNode(b *strings.Builder, byID map[string]*TreeNode, n *TreeNode, prefix, indent string) {
	metaStr := ""
	if len(n.Meta) > 0 {
		var pairs []string
		for k, v := range n.Meta {
			pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
		}
		metaStr = " (" + strings.Join(pairs, ", ") + ")"
	}
	extra := ""
	if len(n.Children) > 0 {
		extra = fmt.Sprintf(" +%d", len(n.Children))
	}
	fmt.Fprintf(b, "%s%s %s%s%s `%s`\n", indent, prefix, n.Summary, metaStr, extra, n.ID)

	for ci, childID := range n.Children {
		child, ok := byID[childID]
		if !ok {
			continue
		}
		cPrefix := "├"
		if ci == len(n.Children)-1 {
			cPrefix = "└"
		}
		cIndent := indent + "  "
		if strings.HasPrefix(prefix, "├") {
			cIndent = indent + "│ "
		}
		if strings.HasPrefix(prefix, "└") {
			cIndent = indent + "  "
		}
		t.writeTreeNode(b, byID, child, cPrefix, cIndent)
	}
}

// ExtractPaths returns all root-to-leaf paths as a single string suitable
// for embedding. Each path node is formatted as "type:summary". Paths are
// separated by newlines, with each path ordered from leaf to root.
func ExtractPaths(nodes []*TreeNode) string {
	if len(nodes) == 0 {
		return ""
	}

	byID := make(map[string]*TreeNode, len(nodes))
	childSet := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
		for _, c := range n.Children {
			childSet[c] = true
		}
	}

	var b strings.Builder
	first := true
	for _, n := range nodes {
		if childSet[n.ID] {
			continue
		}
		if !first {
			b.WriteByte('\n')
		}
		first = false
		writePath(&b, byID, n)
	}
	return b.String()
}

func writePath(b *strings.Builder, byID map[string]*TreeNode, n *TreeNode) {
	b.WriteString(string(n.Type))
	b.WriteByte(':')
	b.WriteString(n.Summary)

	// Walk up to root.
	parentID := ""
	for _, other := range byID {
		if slices.Contains(other.Children, n.ID) {
			parentID = other.ID
			break
		}
	}
	if parentID != "" {
		b.WriteString(" ← ")
		writePath(b, byID, byID[parentID])
	}
}
