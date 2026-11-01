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

// Semantic flags project current state. Scene supplies actual focus and node eligibility.
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

// TextTruncation records which displayed fields lost a suffix to the query budget.
type TextTruncation uint8

// Truncation flags distinguish incomplete names used by locators from unrelated display text.
const (
	NameTruncated TextTruncation = 1 << iota
	DescriptionTruncated
	ValueTruncated
)

// SemanticNode combines model meaning with geometry from the admitted frame.
type SemanticNode struct {
	Target Target
	Parent Target
	Key    string
	Semantic
	Bounds        image.Rectangle
	HasBounds     bool
	Actions       []ActionDescription
	TruncatedText TextTruncation
}

// Progress separates model, layout and confirmed writer advancement.
type Progress struct {
	ModelRevision       uint64
	LayoutRevision      uint64
	LayoutModelRevision uint64
	WrittenFrame        uint64
	WrittenSequence     uint64
}

// Cursor continues one root, model revision, subtree and depth. Its zero value
// starts a traversal. Pass Observation.Next unchanged; page budgets may vary.
type Cursor struct {
	origin   Target
	revision uint64
	depth    int
	next     *continuation
}

// The model revision pins child order. A bookmark stores just the path to
// the first omitted node, so readers can replay a cursor without retaining
// detached subtrees or reevaluating the callbacks for earlier siblings.
type continuation struct {
	path  []observationStep
	modal Target
	root  Target
	key   string
}

type observationStep struct {
	child  int
	parent Target
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
	ErrIncompleteObservation = errors.New("scene: incomplete observation")
	ErrNotFound              = errors.New("scene: no matching target")
	ErrAmbiguous             = errors.New("scene: more than one matching target")
	ErrObservationChanged    = errors.New("scene: observation changed between pages")
)

// Observation is a detached bounded semantic projection. Truncated means that
// nodes were omitted before, after, or below this page. Only Next requests another page.
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

type matchEvidence uint8

const (
	noMatch matchEvidence = iota
	exactMatch
	possibleMatch
)

func (m Match) evaluate(node SemanticNode) matchEvidence {
	if m.Key != "" && node.Key != m.Key || m.Role != RoleNone && node.Role != m.Role {
		return noMatch
	}
	if m.Name == "" {
		return exactMatch
	}
	if node.TruncatedText&NameTruncated != 0 {
		// A clipped name is strictly longer than its displayed prefix.
		if len(m.Name) > len(node.Name) && strings.HasPrefix(m.Name, node.Name) {
			return possibleMatch
		}
		return noMatch
	}
	if m.Name == node.Name {
		return exactMatch
	}
	return noMatch
}

// FindUnique rejects absent, ambiguous or truncated results instead of guessing a target.
func (o Observation) FindUnique(match Match) (Target, error) {
	var found Target
	incomplete := o.Truncated
	for _, node := range o.Nodes {
		switch match.evaluate(node) {
		case noMatch:
		case possibleMatch:
			incomplete = true
		case exactMatch:
			if !found.IsZero() {
				return Target{}, ErrAmbiguous
			}
			found = node.Target
		}
	}
	if incomplete {
		return Target{}, ErrIncompleteObservation
	}
	if found.IsZero() {
		return Target{}, ErrNotFound
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
// An unsettled frame candidate returns ErrBusy before any projection callback runs.
func (r *Root) Observe(query Query) (Observation, error) {
	if r == nil || r.closed {
		return Observation{}, ErrClosed
	}
	if r.preparing || r.observing || r.pending != nil {
		return Observation{}, ErrBusy
	}
	if query.Depth < 0 || query.Limit < 0 || query.TextLimit < 0 {
		return Observation{}, ErrInvalid
	}
	r.observing = true
	defer func() { r.observing = false }()
	origin, cursor, err := r.traversal(query)
	if err != nil {
		return Observation{}, err
	}
	query.Cursor = cursor
	if query.Limit == 0 {
		query.Limit = DefaultQueryLimit
	}
	if query.TextLimit == 0 {
		query.TextLimit = DefaultTextLimit
	}
	query.Limit, query.TextLimit = min(query.Limit, MaxQueryLimit), min(query.TextLimit, MaxTextLimit)
	builder := observationBuilder{root: r, query: query, result: Observation{Progress: r.Progress()}}
	if cursor.next == nil {
		builder.modal = r.modal()
	} else {
		builder.resume = cursor.next.path
		builder.modal = r.resolve(cursor.next.modal)
		builder.result.Truncated = true
	}
	builder.visit(origin, Target{}, 0)
	return builder.result, nil
}

func (r *Root) traversal(q Query) (*node, Cursor, error) {
	c := q.Cursor
	if c.next != nil {
		if c.origin.root != r || c.revision != r.modelRevision || c.depth != q.Depth {
			return nil, Cursor{}, ErrObservationChanged
		}
		if q.Root == c.next.root && q.RootKey == c.next.key {
			return r.resolve(c.origin), c, nil
		}
	}
	origin, err := r.queryRoot(q)
	if err != nil {
		return nil, Cursor{}, err
	}
	cursor := Cursor{revision: r.modelRevision, depth: q.Depth, next: c.next}
	if origin != nil {
		cursor.origin = origin.target()
	}
	if c.next != nil && cursor.origin != c.origin {
		return nil, Cursor{}, ErrObservationChanged
	}
	return origin, cursor, nil
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
	modal  *node
	query  Query
	result Observation
	resume []observationStep
	path   []observationStep
}

func (b *observationBuilder) visit(n *node, parent Target, depth int) bool {
	if n == nil {
		return true
	}
	if b.query.Depth > 0 && depth > b.query.Depth {
		b.result.Truncated = true
		return true
	}
	first := 0
	if depth < len(b.resume) {
		step := b.resume[depth]
		first, parent = step.child, step.parent
	} else {
		b.resume = nil
		var more bool
		parent, more = b.project(n, parent)
		if !more {
			return false
		}
	}
	if n.group != nil {
		for child := first; child < len(n.group.items); child++ {
			b.path = append(b.path, observationStep{child: child, parent: parent})
			if !b.visit(n.group.items[child].Element.node, parent, depth+1) {
				return false
			}
			b.path = b.path[:depth]
		}
	}
	return true
}

func (b *observationBuilder) project(n *node, parent Target) (Target, bool) {
	var semantic Semantic
	if n.config.Semantics != nil {
		semantic = n.config.Semantics()
	}
	if semantic.Role == RoleNone && semantic.Name == "" && n.config.Key == "" && len(n.config.Actions) == 0 {
		return parent, true
	}
	if len(b.result.Nodes) == b.query.Limit {
		next := &continuation{path: slices.Clone(b.path), root: b.query.Root, key: strings.Clone(b.query.RootKey)}
		if b.modal != nil {
			next.modal = b.modal.target()
		}
		b.result.Truncated = true
		b.result.Next = b.query.Cursor
		b.result.Next.next = next
		return parent, false
	}
	b.result.Nodes = append(b.result.Nodes, b.semanticNode(n, parent, semantic))
	return n.target(), true
}

func (b *observationBuilder) semanticNode(n *node, parent Target, semantic Semantic) SemanticNode {
	r, textLimit := b.root, b.query.TextLimit
	var truncated TextTruncation
	clip := func(value string, field TextTruncation) string {
		if len(value) > textLimit {
			truncated |= field
		}
		return boundedText(value, textLimit)
	}
	semantic.Name = clip(semantic.Name, NameTruncated)
	semantic.Description = clip(semantic.Description, DescriptionTruncated)
	if semantic.Sensitive {
		semantic.Value = ""
	} else {
		semantic.Value = clip(semantic.Value, ValueTruncated)
	}
	semantic.ContentID = strings.Clone(semantic.ContentID)
	semantic.Relations = slices.DeleteFunc(slices.Clone(semantic.Relations), func(rel Relation) bool { return !r.Valid(rel.Target) })
	allowed := r.allowedIn(n, b.modal)
	semantic.State &^= StateFocused | StateDisabled
	if r.focus == n.target() {
		semantic.State |= StateFocused
	}
	if !allowed {
		semantic.State |= StateDisabled
	}
	entry := SemanticNode{Target: n.target(), Parent: parent, Key: strings.Clone(n.config.Key), Semantic: semantic, TruncatedText: truncated}
	if placement, ok := r.layout.lookup(n.target()); ok && r.layout.epoch == r.geometryEpoch {
		entry.Bounds, entry.HasBounds = placement.clip, true
	}
	for _, action := range n.config.Actions {
		var availability Availability
		needs := !unitParameter(action.parameter)
		if !allowed {
			availability = Availability{Disabled: true, Reason: "target is outside the active scope"}
		} else if !needs {
			availability, _ = action.availability(nil)
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
