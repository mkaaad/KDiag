package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ---------- AddNode input ----------

// AddNodeQuery is the JSON input expected by AddNodeTool.Call.
type AddNodeQuery struct {
	ParentID string `json:"parent_id"` // empty string means attach to tree root
	Summary  string `json:"summary"`
	Type     string `json:"type"` // "finding", "metric", "log", "code_change", etc.
	Content  string `json:"content"`
}

// ---------- AddNode tool ----------

// AddNodeTool allows the LLM agent to attach new findings back into the
// correlation information tree. When the agent discovers something relevant
// (e.g., a related log pattern, a code change, or a metric anomaly), it can
// create a new tree node and optionally attach it as a child of an existing
// node. This keeps all gathered context structured and navigable.
type AddNodeTool struct {
	name string
	desc string
}

// NewAddNodeTool creates an AddNodeTool.
func NewAddNodeTool() *AddNodeTool {
	return &AddNodeTool{
		name: "AddNode",
		desc: `Attach a new discovery back into the correlation information tree.
The new node appears under the specified parent, keeping all findings structured.

Input: JSON object with:
  - "parent_id": (string, optional) ID of the parent node; empty attaches to tree root
  - "summary":   (string) short label for the new node
  - "type":      (string) one of: "finding", "metric", "log", "code_change", "trace", "span"
  - "content":   (string) detailed content that will be shown when the node is expanded

Output: the new node ID, which you can use as parent_id for further nodes.

Examples:
  {"parent_id":"tr_1", "summary":"db pool exhaustion confirmed", "type":"finding", "content":"connection pool max=50, active=50, waiting=12"}
  {"parent_id":"", "summary":"related deployment: v2.3.1 rolled back", "type":"code_change", "content":"commit abc123 reverted the connection pool fix"}`,
	}
}

func (a *AddNodeTool) Name() string       { return a.name }
func (a *AddNodeTool) Description() string { return a.desc }

func (a *AddNodeTool) Call(ctx context.Context, input string) (string, error) {
	var req AddNodeQuery
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return fmt.Sprintf("invalid input: %s", err), nil
	}
	if req.Summary == "" {
		return "summary is required", nil
	}

	// Create the new node.
	nodeCounter++
	newID := fmt.Sprintf("ag_%x", nodeCounter)

	node := &TreeNode{
		ID:      newID,
		Type:    NodeType(req.Type),
		Summary: req.Summary,
	}

	// If content is provided, store it as meta so ExpandNode can return it.
	if req.Content != "" {
		node.Meta = map[string]string{"content": req.Content}
		// Also set a query-like field so expandMetric/expandLogStream can
		// fall through to generic content display.
		node.Query = req.Content
	}

	// Register the node.
	expandNodeRegMu.Lock()
	expandNodeReg[newID] = node

	// Attach to parent if specified.
	if req.ParentID != "" {
		if parent, ok := expandNodeReg[req.ParentID]; ok {
			parent.Children = append(parent.Children, newID)
		}
	}
	expandNodeRegMu.Unlock()

	var b strings.Builder
	fmt.Fprintf(&b, "✅ Node created: `%s`\n", newID)
	fmt.Fprintf(&b, "   Summary: %s\n", req.Summary)
	fmt.Fprintf(&b, "   Type: %s\n", req.Type)
	if req.ParentID != "" {
		fmt.Fprintf(&b, "   Attached under: `%s`\n", req.ParentID)
	}
	b.WriteString("\nUse ExpandNode to view details, or AddNode to add more findings under this node.")
	return b.String(), nil
}
