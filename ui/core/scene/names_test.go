package scene_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestSemanticValuesHaveReadableNames(t *testing.T) {
	for got, want := range map[string]string{
		scene.RoleTreeItem.String():                             "treeitem",
		scene.Role(200).String():                                "Role(200)",
		(scene.StateSelected | scene.StateChecked).String():     "selected|checked",
		scene.SemanticState(0).String():                         "",
		scene.ContentArchived.String():                          "archived",
		scene.DescribedBy.String():                              "describedby",
		scene.Started.String():                                  "started",
		(scene.StateBusy | scene.SemanticState(1<<15)).String(): "busy|SemanticState(0x8000)",
	} {
		if got != want {
			t.Errorf("name = %q, want %q", got, want)
		}
	}
}

func TestEveryRoleIsNamed(t *testing.T) {
	for role := scene.RoleNone; role <= scene.RoleMenuItem; role++ {
		if name := role.String(); name == "" || name[0] == 'R' {
			t.Errorf("role %d has no name: %q", role, name)
		}
	}
}
