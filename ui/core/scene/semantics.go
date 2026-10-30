package scene

import (
	"errors"
	"image"
	"slices"
	"strings"
)

// Role describes a node's semantic purpose without changing its behavior.
type Role uint8

// Standard roles describe the composed scene rather than a second editable tree.
const (
	RoleNone Role = iota
	RoleButton
	RoleDialog
	RoleTabList
	RoleTab
	RoleTabPanel
	RoleSlider
	RoleTextBox
	RoleCheckBox
	RoleList
	RoleListItem
	RoleTree
	RoleTreeItem
	RoleTable
	RoleRow
	RoleCell
	RoleForm
	RoleStatus
	RoleProgress
	RoleDocument
	RoleGroup
	RoleHeading
	RoleLink
	RoleMenu
	RoleMenuItem
)

// SemanticState combines independently projected accessibility states.
type SemanticState uint16

// Semantic flags project current state. Scene alone supplies actual focus.
const (
	StateFocused SemanticState = 1 << iota
	StateSelected
	StateOpen
	StateDisabled
	StateReadOnly
	StateRequired
	StateInvalid
	StateBusy
	StateExpanded
	StateChecked
)

// Has reports whether every state in the mask is present.
func (state SemanticState) Has(flags SemanticState) bool { return flags != 0 && state&flags == flags }

// ContentAvailability describes whether content is live, archived or no longer retained.
type ContentAvailability uint8

// Content locations describe which capability can still provide an item.
const (
	ContentLive ContentAvailability = iota
	ContentVirtualized
	ContentArchived
	ContentRetired
)

// RelationKind names a semantic relation between exact targets.
type RelationKind uint8

// Relation kinds identify common accessible naming and control relationships.
const (
	LabelledBy RelationKind = iota
	DescribedBy
	Controls
)

// Relation links one semantic node to another current attachment.
type Relation struct {
	Kind   RelationKind
	Target Target
}

// Semantic is a read projection from the behavior owner. There is no independent
// semantic tree to edit. ContentRevision guards interpretation of old text hits.
type Semantic struct {
	Role            Role
	Name            string
	Description     string
	Value           string
	State           SemanticState
	Sensitive       bool
	Relations       []Relation
	ContentID       string
	ContentRevision uint64
	Availability    ContentAvailability
}

// ActionDescription is a read-only operation signature and availability projection.
type ActionDescription struct {
	ID                 ActionID
	Label              string
	ParameterType      string
	ResultType         string
	Availability       Availability
	RequiresParameters bool
}

// SemanticNode combines model meaning with geometry from the admitted frame.
type SemanticNode struct {
	Target Target
	Parent Target
	Key    string
	Semantic
	Bounds        image.Rectangle
	HasBounds     bool
	Actions       []ActionDescription
	TextTruncated bool
}

// Progress separates model, layout and confirmed writer advancement.
type Progress struct {
	ModelRevision       uint64
	LayoutRevision      uint64
	LayoutModelRevision uint64
	WrittenFrame        uint64
	WrittenSequence     uint64
}

// Cursor pins pagination to a model revision. A changed model rejects the next
// page rather than presenting a mixed-version tree as one observation.
type Cursor struct {
	Revision uint64
	Offset   int
}

// Query bounds a semantic subtree, output count and text size. Zero limits use package defaults.
type Query struct {
	Root      Target
	RootKey   string
	Depth     int
	Limit     int
	TextLimit int
	Cursor    Cursor
}

// DefaultQueryLimit is the default maximum number of returned semantic nodes.
const (
	DefaultQueryLimit = 256
	// MaxQueryLimit caps one observation's returned node count.
	MaxQueryLimit = 4096
	// DefaultTextLimit is the default byte budget per projected text field.
	DefaultTextLimit = 4096
	// MaxTextLimit caps the byte budget per projected text field.
	MaxTextLimit = 1 << 20
)

// Observation errors keep absence, ambiguity and stale pagination explicit.
var (
	ErrNotFound           = errors.New("scene: no matching target")
	ErrAmbiguous          = errors.New("scene: more than one matching target")
	ErrObservationChanged = errors.New("scene: observation changed between pages")
)

// Observation is a detached bounded semantic projection with explicit pagination and progress.
type Observation struct {
	Progress  Progress
	Nodes     []SemanticNode
	Truncated bool
	Next      Cursor
}

// Match filters by key, role and name; empty fields impose no condition.
type Match struct {
	Key  string
	Role Role
	Name string
}

// FindUnique rejects absent, ambiguous or truncated results instead of guessing a target.
func (o Observation) FindUnique(match Match) (Target, error) {
	var found Target
	for _, node := range o.Nodes {
		if match.Key != "" && node.Key != match.Key || match.Role != RoleNone && node.Role != match.Role || match.Name != "" && node.Name != match.Name {
			continue
		}
		if !found.IsZero() {
			return Target{}, ErrAmbiguous
		}
		found = node.Target
	}
	if found.IsZero() {
		return Target{}, ErrNotFound
	}
	if o.Truncated {
		return Target{}, errors.New("scene: cannot prove a unique match in a truncated observation")
	}
	return found, nil
}

// Progress reads the current model, admitted-layout and writer-completion revisions.
func (r *Root) Progress() Progress {
	if r == nil {
		return Progress{}
	}
	p := Progress{ModelRevision: r.modelRevision, WrittenSequence: r.writtenSequence}
	if r.layout != nil {
		p.LayoutRevision, p.LayoutModelRevision = r.layout.revision, r.layout.model
	}
	if r.written != nil {
		p.WrittenFrame = r.written.revision
	}
	return p
}

// Observe returns a bounded detached projection without changing model, layout
// or focus. Pagination is pinned to the model revision that produced its cursor.
func (r *Root) Observe(query Query) (Observation, error) {
	if r == nil || r.closed {
		return Observation{}, ErrClosed
	}
	if r.preparing || r.observing {
		return Observation{}, ErrBusy
	}
	if query.Cursor.Offset < 0 || query.Depth < 0 || query.Limit < 0 || query.TextLimit < 0 {
		return Observation{}, ErrInvalid
	}
	if query.Cursor.Offset > 0 && query.Cursor.Revision != r.modelRevision {
		return Observation{}, ErrObservationChanged
	}
	r.observing = true
	defer func() { r.observing = false }()
	origin, err := r.queryRoot(query)
	if err != nil {
		return Observation{}, err
	}
	if query.Limit == 0 {
		query.Limit = DefaultQueryLimit
	}
	if query.TextLimit == 0 {
		query.TextLimit = DefaultTextLimit
	}
	query.Limit, query.TextLimit = min(query.Limit, MaxQueryLimit), min(query.TextLimit, MaxTextLimit)
	builder := observationBuilder{root: r, query: query, result: Observation{Progress: r.Progress()}}
	builder.visit(origin, Target{}, 0)
	return builder.result, nil
}

func (r *Root) queryRoot(query Query) (*node, error) {
	origin := r.content.node
	if !query.Root.IsZero() {
		origin = r.resolve(query.Root)
		if origin == nil {
			return nil, ErrStale
		}
	}
	if query.RootKey == "" {
		return origin, nil
	}
	var found *node
	ambiguous := false
	r.walk(origin, func(n *node) bool {
		if n.config.Key != query.RootKey {
			return true
		}
		if found != nil {
			ambiguous = true
			return false
		}
		found = n
		return true
	})
	if ambiguous {
		return nil, ErrAmbiguous
	}
	if found == nil {
		return nil, ErrNotFound
	}
	return found, nil
}

type observationBuilder struct {
	root   *Root
	query  Query
	result Observation
	seen   int
}

func (b *observationBuilder) visit(n *node, parent Target, depth int) bool {
	if n == nil {
		return true
	}
	if b.query.Depth > 0 && depth > b.query.Depth {
		b.result.Truncated = true
		return true
	}
	var semantic Semantic
	if n.config.Semantics != nil {
		semantic = n.config.Semantics()
	}
	if semantic.Role != RoleNone || semantic.Name != "" || n.config.Key != "" || len(n.config.Actions) > 0 {
		if !b.append(n, parent, semantic) {
			return false
		}
		parent = n.target()
	}
	if n.group == nil {
		return true
	}
	for _, item := range n.group.items {
		if !b.visit(item.Element.node, parent, depth+1) {
			return false
		}
	}
	return true
}

func (b *observationBuilder) append(n *node, parent Target, semantic Semantic) bool {
	if b.seen >= b.query.Cursor.Offset {
		if len(b.result.Nodes) == b.query.Limit {
			b.result.Truncated = true
			b.result.Next = Cursor{Revision: b.root.modelRevision, Offset: b.seen}
			return false
		}
		b.result.Nodes = append(b.result.Nodes, b.root.semanticNode(n, parent, semantic, b.query.TextLimit))
	}
	b.seen++
	return true
}

func (r *Root) semanticNode(n *node, parent Target, semantic Semantic, textLimit int) SemanticNode {
	truncated := len(semantic.Name) > textLimit || len(semantic.Description) > textLimit || !semantic.Sensitive && len(semantic.Value) > textLimit
	semantic.Name = boundedText(semantic.Name, textLimit)
	semantic.Description = boundedText(semantic.Description, textLimit)
	if semantic.Sensitive {
		semantic.Value = ""
	} else {
		semantic.Value = boundedText(semantic.Value, textLimit)
	}
	semantic.ContentID = strings.Clone(semantic.ContentID)
	semantic.Relations = slices.Clone(semantic.Relations)
	semantic.State &^= StateFocused
	if r.focus == n.target() {
		semantic.State |= StateFocused
	}
	if !r.allowed(n) {
		semantic.State |= StateDisabled
	}
	entry := SemanticNode{Target: n.target(), Parent: parent, Key: strings.Clone(n.config.Key), Semantic: semantic, TextTruncated: truncated}
	if placement, ok := findGeometry(r.layout, n.target()); ok {
		entry.Bounds, entry.HasBounds = placement.clip, true
	}
	for _, action := range n.config.Actions {
		availability, err := action.availability(nil)
		needs := err != nil
		if !r.allowed(n) {
			availability = Availability{Disabled: true, Reason: "target is outside the active scope"}
		}
		entry.Actions = append(entry.Actions, ActionDescription{ID: action.id, Label: strings.Clone(action.label), ParameterType: action.parameter.String(), ResultType: action.result.String(), Availability: availability, RequiresParameters: needs})
	}
	return entry
}

func boundedText(value string, maximum int) string {
	if len(value) <= maximum {
		return strings.Clone(value)
	}
	end := maximum
	for end > 0 && value[end]&0xc0 == 0x80 {
		end--
	}
	return strings.Clone(value[:end])
}
