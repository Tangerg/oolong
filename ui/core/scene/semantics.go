package scene

import (
	"fmt"
	"image"
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

// Semantic flags project current state. StateFocused and StateDisabled appear only
// in observations: the scene derives them, and a behavior reporting them is refused.
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

const (
	knownStates = StateChecked<<1 - 1
	// sceneStates are facts the scene owns. A behavior owner cannot report them;
	// observation derives them from actual focus and eligibility.
	sceneStates = StateFocused | StateDisabled
)

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
	case s.State&sceneStates != 0:
		return fmt.Errorf("%w: semantic state %#x belongs to the scene", ErrInvalid, s.State&sceneStates)
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
// Disabled is kept apart from Reason because the query budget may truncate the
// reason, even to nothing, without making a refused operation available.
type ActionDescription struct {
	ID                 ActionID
	Label              string
	ParameterType      string
	ResultType         string
	Disabled           bool
	Reason             string
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
