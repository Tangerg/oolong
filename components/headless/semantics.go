package headless

// Role identifies what a semantic node means rather than how many cells draw it.
//
// Roles are typed constants instead of strings so inspection, automation and future
// host integrations can distinguish controls without agreeing on an attribute
// vocabulary at runtime.
type Role uint8

const (
	// RoleNone is a meaningful node whose specialized role is not known.
	RoleNone Role = iota
	// RoleButton is an activatable control.
	RoleButton
	// RoleDialog is modal content with an open lifecycle.
	RoleDialog
	// RoleTabList is a controller grouping tabs and their selected panel.
	RoleTabList
	// RoleTab is one selectable label in a tab list.
	RoleTab
	// RoleTabPanel is the content selected by a tab.
	RoleTabPanel
)

// SemanticState is a set of independent facts about a semantic node.
type SemanticState uint8

const (
	// StateFocused says the node or its active part owns the keyboard.
	StateFocused SemanticState = 1 << iota
	// StateSelected says the node is the selected choice among peers.
	StateSelected
	// StateOpen says the node has exposed content which can be dismissed.
	StateOpen
)

// Has reports whether state contains every bit in flags.
func (state SemanticState) Has(flags SemanticState) bool {
	return flags != 0 && state&flags == flags
}

// SemanticNode is one meaningful control or part of a control.
//
// The tree is structural, not visual: decorative borders do not appear, one control
// may be painted by several boxes, and a child need not occupy a child rectangle.
// Callers that retain the result own it; components may rebuild slices on each call.
type SemanticNode struct {
	Role        Role
	Label       string
	Description string
	State       SemanticState
	Children    []SemanticNode
}

// Semantic is implemented by controls that expose a structural semantic projection.
type Semantic interface {
	Semantics() SemanticNode
}
