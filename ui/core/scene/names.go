package scene

import (
	"strconv"
	"strings"

	"github.com/Tangerg/oolong/ui/internal/enum"
)

// Observations reach automation clients and test failures as values; these names
// keep a role or state readable there instead of a bare number.

var roleNames = [...]string{
	RoleNone:     "none",
	RoleButton:   "button",
	RoleDialog:   "dialog",
	RoleTabList:  "tablist",
	RoleTab:      "tab",
	RoleTabPanel: "tabpanel",
	RoleSlider:   "slider",
	RoleTextBox:  "textbox",
	RoleCheckBox: "checkbox",
	RoleList:     "list",
	RoleListItem: "listitem",
	RoleTree:     "tree",
	RoleTreeItem: "treeitem",
	RoleTable:    "table",
	RoleRow:      "row",
	RoleCell:     "cell",
	RoleForm:     "form",
	RoleStatus:   "status",
	RoleProgress: "progressbar",
	RoleDocument: "document",
	RoleGroup:    "group",
	RoleHeading:  "heading",
	RoleLink:     "link",
	RoleMenu:     "menu",
	RoleMenuItem: "menuitem",
}

// String is the role's ARIA name, which automation clients already speak.
func (r Role) String() string { return enum.Name(roleNames[:], r, "Role") }

var stateNames = [...]struct {
	state SemanticState
	name  string
}{
	{StateFocused, "focused"},
	{StateSelected, "selected"},
	{StateOpen, "open"},
	{StateDisabled, "disabled"},
	{StateReadOnly, "readonly"},
	{StateRequired, "required"},
	{StateInvalid, "invalid"},
	{StateBusy, "busy"},
	{StateExpanded, "expanded"},
	{StateChecked, "checked"},
}

// String joins the set flags with "|"; unknown bits are reported in hex.
func (state SemanticState) String() string {
	var names []string
	for _, flag := range stateNames {
		if state.Has(flag.state) {
			names = append(names, flag.name)
		}
	}
	if unknown := state &^ knownStates; unknown != 0 {
		names = append(names, "SemanticState(0x"+strconv.FormatUint(uint64(unknown), 16)+")")
	}
	return strings.Join(names, "|")
}

var availabilityNames = [...]string{
	ContentLive:        "live",
	ContentVirtualized: "virtualized",
	ContentArchived:    "archived",
	ContentRetired:     "retired",
}

func (a ContentAvailability) String() string {
	return enum.Name(availabilityNames[:], a, "ContentAvailability")
}

var relationNames = [...]string{
	LabelledBy:  "labelledby",
	DescribedBy: "describedby",
	Controls:    "controls",
}

func (k RelationKind) String() string { return enum.Name(relationNames[:], k, "RelationKind") }

var actionStateNames = [...]string{
	Unchanged: "unchanged",
	Applied:   "applied",
	Started:   "started",
}

func (s ActionState) String() string { return enum.Name(actionStateNames[:], s, "ActionState") }
