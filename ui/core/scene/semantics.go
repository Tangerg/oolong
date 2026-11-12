package scene

import (
	"errors"
	"fmt"
	"image"
	"slices"
	"strings"
	"unicode/utf8"
)

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

func (state SemanticState) Has(flags SemanticState) bool { return flags != 0 && state&flags == flags }

const knownStates = StateChecked<<1 - 1

type ContentAvailability uint8

// Content locations describe which capability can still provide an item.
const (
	ContentLive ContentAvailability = iota
	ContentVirtualized
	ContentArchived
	ContentRetired
)

type RelationKind uint8

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

// validate refuses values no automation client can interpret. The behavior owner
// supplies them, so Observe checks them where they cross into an observation.
func (s Semantic) validate() error {
	switch {
	case s.Role > RoleMenuItem:
		return fmt.Errorf("%w: semantic role %d", ErrInvalid, s.Role)
	case s.State&^knownStates != 0:
		return fmt.Errorf("%w: semantic state %#x", ErrInvalid, s.State)
	case s.Availability > ContentRetired:
		return fmt.Errorf("%w: content availability %d", ErrInvalid, s.Availability)
	}
	for _, relation := range s.Relations {
		if relation.Kind > Controls {
			return fmt.Errorf("%w: relation kind %d", ErrInvalid, relation.Kind)
		}
	}
	return nil
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
	TruncatedText      TextTruncation
}

// TextTruncation records which displayed fields lost a suffix to the query budget.
type TextTruncation uint8

// Truncation flags distinguish incomplete names used by locators from unrelated display text.
const (
	NameTruncated TextTruncation = 1 << iota
	DescriptionTruncated
	ValueTruncated
	LabelTruncated
	ReasonTruncated
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

// Query budgets keep each admitted node complete. Zero limits use package
// defaults; positive limits are capped by the corresponding package maximum.
// If the first node cannot fit, Observe returns ErrObservationBudget so callers
// can increase the budget without mistaking an omitted node for absence.
// Identities remain complete; only presentation text can be clipped.
type Query struct {
	Root      Target
	RootKey   string
	Depth     int
	Limit     int
	TextLimit int
	// ByteLimit covers all strings in the returned nodes and action descriptions.
	ByteLimit int
	// DetailLimit bounds the combined number of actions and live relations.
	DetailLimit int
	Cursor      Cursor
}

const (
	DefaultQueryLimit  = 256
	MaxQueryLimit      = 4096
	DefaultTextLimit   = 4096
	MaxTextLimit       = 1 << 20
	DefaultByteLimit   = 1 << 20
	MaxByteLimit       = 8 << 20
	DefaultDetailLimit = 4096
	MaxDetailLimit     = 1 << 16
)

// Observation errors keep absence, ambiguity and stale pagination explicit.
var (
	ErrIncompleteObservation = errors.New("scene: incomplete observation")
	ErrNotFound              = errors.New("scene: no matching target")
	ErrAmbiguous             = errors.New("scene: more than one matching target")
	ErrObservationChanged    = errors.New("scene: observation changed between pages")
	ErrObservationBudget     = errors.New("scene: semantic node exceeds observation budget")
)

func (q Query) normalized() (Query, error) {
	if q.Depth < 0 || q.Limit < 0 || q.TextLimit < 0 || q.ByteLimit < 0 || q.DetailLimit < 0 {
		return Query{}, ErrInvalid
	}
	q.Limit = queryLimit(q.Limit, DefaultQueryLimit, MaxQueryLimit)
	q.TextLimit = queryLimit(q.TextLimit, DefaultTextLimit, MaxTextLimit)
	q.ByteLimit = queryLimit(q.ByteLimit, DefaultByteLimit, MaxByteLimit)
	q.DetailLimit = queryLimit(q.DetailLimit, DefaultDetailLimit, MaxDetailLimit)
	return q, nil
}

func queryLimit(value, fallback, maximum int) int {
	if value == 0 {
		return fallback
	}
	return min(value, maximum)
}

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

func (r *Root) Progress() Progress {
	if r == nil {
		return Progress{}
	}
	p := Progress{ModelRevision: r.modelRevision, WrittenSequence: r.frames.writtenSequence()}
	if r.layout != nil {
		p.LayoutRevision, p.LayoutModelRevision = r.layout.revision, r.layout.model
	}
	if written := r.frames.written; written != nil {
		p.WrittenFrame = written.revision
	}
	return p
}

// Observe returns a bounded detached projection without changing model, layout
// or focus. Pagination is pinned to the model revision that produced its cursor.
// An unsettled frame candidate returns ErrBusy before any projection callback runs.
func (r *Root) Observe(query Query) (Observation, error) {
	if err := r.mutable(); err != nil {
		return Observation{}, err
	}
	query, err := query.normalized()
	if err != nil {
		return Observation{}, err
	}
	r.observing = true
	defer func() { r.observing = false }()
	origin, cursor, err := r.traversal(query)
	if err != nil {
		return Observation{}, err
	}
	query.Cursor = cursor
	builder := observationBuilder{
		root: r, query: query, result: Observation{Progress: r.Progress()},
		budget: observationBudget{bytes: query.ByteLimit, details: query.DetailLimit},
	}
	if cursor.next == nil {
		builder.modal = r.modal()
	} else {
		builder.resume = cursor.next.path
		builder.modal = r.resolve(cursor.next.modal)
		builder.result.Truncated = true
	}
	builder.visit(origin, Target{}, 0)
	if builder.err != nil {
		return Observation{}, builder.err
	}
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
	budget observationBudget
	err    error
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
	if err := semantic.validate(); err != nil {
		b.err = err
		return parent, false
	}
	if semantic.Role == RoleNone && semantic.Name == "" && n.config.Key == "" && len(n.config.Actions) == 0 {
		return parent, true
	}
	if len(b.result.Nodes) == b.query.Limit {
		b.continueFromCurrent()
		return parent, false
	}
	remaining := b.budget
	entry := b.semanticNode(n, parent, semantic, &remaining)
	if remaining.err != nil {
		if len(b.result.Nodes) == 0 {
			b.err = remaining.err
		} else {
			b.continueFromCurrent()
		}
		return parent, false
	}
	b.budget = remaining
	b.result.Nodes = append(b.result.Nodes, entry)
	return n.target(), true
}

func (b *observationBuilder) continueFromCurrent() {
	next := &continuation{path: slices.Clone(b.path), root: b.query.Root, key: strings.Clone(b.query.RootKey)}
	if b.modal != nil {
		next.modal = b.modal.target()
	}
	b.result.Truncated = true
	b.result.Next = b.query.Cursor
	b.result.Next.next = next
}

func (b *observationBuilder) semanticNode(n *node, parent Target, semantic Semantic, budget *observationBudget) SemanticNode {
	r := b.root
	var truncated TextTruncation
	clip := func(value string, field TextTruncation) string {
		if len(value) > b.query.TextLimit {
			truncated |= field
		}
		return budget.text(textPrefix(value, b.query.TextLimit))
	}
	semantic.Name = clip(semantic.Name, NameTruncated)
	semantic.Description = clip(semantic.Description, DescriptionTruncated)
	if semantic.Sensitive {
		semantic.Value = ""
	} else {
		semantic.Value = clip(semantic.Value, ValueTruncated)
	}
	semantic.ContentID = budget.text(semantic.ContentID)
	semantic.Relations = b.relations(semantic.Relations, budget)
	access := r.eligibilityIn(n, b.modal)
	allowed := access == eligible
	semantic.State &^= StateFocused | StateDisabled
	if r.focus == n.target() {
		semantic.State |= StateFocused
	}
	if !allowed {
		semantic.State |= StateDisabled
	}
	entry := SemanticNode{
		Target: budget.target(n.target()), Parent: budget.target(parent), Key: budget.text(n.config.Key),
		Semantic: semantic, TruncatedText: truncated,
	}
	if r.current(r.layout) {
		if placement, ok := r.layout.lookup(n.target()); ok {
			entry.Bounds, entry.HasBounds = placement.clip, true
		}
	}
	for _, action := range n.config.Actions {
		if !budget.detail() {
			break
		}
		entry.Actions = append(entry.Actions, b.actionDescription(action, access, budget))
	}
	return entry
}

func (b *observationBuilder) relations(relations []Relation, budget *observationBudget) []Relation {
	var live []Relation
	for _, relation := range relations {
		if !b.root.Valid(relation.Target) {
			continue
		}
		if !budget.detail() {
			break
		}
		relation.Target = budget.target(relation.Target)
		live = append(live, relation)
	}
	return live
}

func (b *observationBuilder) actionDescription(action ActionBinding, access eligibility, budget *observationBudget) ActionDescription {
	var available Availability
	needs := !unitParameter(action.parameter)
	if access != eligible {
		available = Availability{Disabled: true, Reason: access.reason()}
	} else if !needs {
		available, _ = action.availability(nil)
	}
	var truncated TextTruncation
	if len(action.label) > b.query.TextLimit {
		truncated |= LabelTruncated
	}
	if len(available.Reason) > b.query.TextLimit {
		truncated |= ReasonTruncated
	}
	available.Reason = budget.text(textPrefix(available.Reason, b.query.TextLimit))
	return ActionDescription{
		ID:            ActionID(budget.text(string(action.id))),
		Label:         budget.text(textPrefix(action.label, b.query.TextLimit)),
		ParameterType: budget.text(action.parameter.String()), ResultType: budget.text(action.result.String()),
		Availability: available, RequiresParameters: needs, TruncatedText: truncated,
	}
}

// A node spends a provisional budget. Admission publishes the node and its
// remaining budget together, so pagination never exposes half of its metadata.
type observationBudget struct {
	bytes   int
	details int
	err     error
}

func (b *observationBudget) target(target Target) Target {
	target.part = PartID(b.text(string(target.part)))
	return target
}

func (b *observationBudget) text(value string) string {
	if b.err != nil {
		return ""
	}
	if len(value) > b.bytes {
		b.err = fmt.Errorf("%w: string bytes exceed ByteLimit", ErrObservationBudget)
		return ""
	}
	b.bytes -= len(value)
	return strings.Clone(value)
}

func (b *observationBudget) detail() bool {
	if b.err != nil {
		return false
	}
	if b.details == 0 {
		b.err = fmt.Errorf("%w: actions and relations exceed DetailLimit", ErrObservationBudget)
		return false
	}
	b.details--
	return true
}

func textPrefix(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	end := maximum
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}
