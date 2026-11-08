package base_test

import (
	"cmp"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

func semanticOwner(t *testing.T, element scene.Element) *scene.Root {
	t.Helper()
	root, err := scene.NewRoot(scene.RootConfig{Element: element})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(root.Close)
	return root
}

func semanticRevision(t *testing.T, root *scene.Root, element scene.Element) uint64 {
	t.Helper()
	observation, err := root.Observe(scene.Query{Root: element.Target()})
	if err != nil || len(observation.Nodes) == 0 {
		t.Fatalf("observe collection: %+v, %v", observation, err)
	}
	return observation.Nodes[0].ContentRevision
}

func TestCollectionActionsUseBoundedSnapshotsAndRevisionQualifiedAddresses(t *testing.T) {
	var list base.List[string]
	list.SetItems([]string{"first", "second", "third"})
	root := semanticOwner(t, list.Element())
	revision := semanticRevision(t, root, list.Element())
	read, err := root.Invoke(t.Context(), list.Element().Target(), base.ActionReadItems, base.CollectionQuery{ContentRevision: revision, Start: 1, Limit: 1})
	if err != nil || read.State != scene.Unchanged {
		t.Fatalf("read = %+v, %v", read, err)
	}
	page := read.Value.(base.CollectionPage[string])
	if page.Start != 1 || page.Total != 3 || page.Selected != 0 || !slices.Equal(page.Items, []string{"second"}) {
		t.Fatalf("page = %+v", page)
	}
	page.Items[0] = "modified snapshot"
	if item, _ := list.At(1); item != "second" {
		t.Fatal("snapshot retained the owned item slice")
	}
	address := base.CollectionIndex{ContentRevision: revision, Index: 1}
	selected, err := root.Invoke(t.Context(), list.Element().Target(), scene.ActionSelect, address)
	if err != nil || selected.State != scene.Applied || list.Selected() != 1 {
		t.Fatalf("selection = %+v, %v", selected, err)
	}
	if value := selected.Value.(base.CollectionSelection[string]); value.Address != address || value.Item != "second" || !value.Present {
		t.Fatalf("selected value = %+v", value)
	}
	unchanged, err := root.Invoke(t.Context(), list.Element().Target(), scene.ActionSelect, address)
	if err != nil || unchanged.State != scene.Unchanged {
		t.Fatalf("repeated selection = %+v, %v", unchanged, err)
	}
	if _, err := root.Invoke(t.Context(), list.Element().Target(), scene.ActionSelect, 1); !errors.Is(err, scene.ErrArgument) {
		t.Fatalf("unqualified index = %v", err)
	}
	for _, query := range []base.CollectionQuery{
		{ContentRevision: revision, Start: -1},
		{ContentRevision: revision, Start: 4},
		{ContentRevision: revision, Limit: base.MaxCollectionPageSize + 1},
	} {
		if _, err := root.Invoke(t.Context(), list.Element().Target(), base.ActionReadItems, query); !errors.Is(err, scene.ErrUnavailable) {
			t.Fatalf("invalid query %+v = %v", query, err)
		}
	}
	list.SetItems([]string{"replacement"})
	if _, err := root.Invoke(t.Context(), list.Element().Target(), scene.ActionSelect, address); !errors.Is(err, scene.ErrUnavailable) {
		t.Fatalf("stale address = %v", err)
	}
	if _, err := root.Invoke(t.Context(), list.Element().Target(), base.ActionReadItems, base.CollectionQuery{ContentRevision: revision}); !errors.Is(err, scene.ErrUnavailable) {
		t.Fatalf("stale page = %v", err)
	}
}

func TestFilterSemanticFillAndSelectionShareTheRankedCollection(t *testing.T) {
	var filter base.Filter[string]
	filter.SetItems([]string{"alpha", "beta", "alpine"}, func(value string) string { return value })
	root := semanticOwner(t, filter.Element())
	before := semanticRevision(t, root, filter.Element())
	filled, err := root.Invoke(t.Context(), filter.Element().Target(), scene.ActionFill, "alp")
	if err != nil || filled.State != scene.Applied || filled.Value != "alp" || filter.Matched() != 2 {
		t.Fatalf("filter fill = %+v, %v", filled, err)
	}
	if _, err = root.Invoke(t.Context(), filter.Element().Target(), scene.ActionSelect, base.CollectionIndex{ContentRevision: before, Index: 0}); !errors.Is(err, scene.ErrUnavailable) {
		t.Fatalf("pre-filter address = %v", err)
	}
	revision := semanticRevision(t, root, filter.Element())
	read, err := root.Invoke(t.Context(), filter.Element().Target(), base.ActionReadItems, base.CollectionQuery{ContentRevision: revision})
	if err != nil {
		t.Fatal(err)
	}
	page := read.Value.(base.CollectionPage[string])
	selected, err := root.Invoke(t.Context(), filter.Element().Target(), scene.ActionSelect, base.CollectionIndex{ContentRevision: revision, Index: 1})
	current, ok := filter.Current()
	if err != nil || !ok || current != page.Items[1] || selected.Value.(base.CollectionSelection[string]).Item != current {
		t.Fatalf("ranked selection = %+v, %q, %v", selected, current, err)
	}
}

func TestMultiSelectActionsExposeChoiceIdentityAndHonorTheLimit(t *testing.T) {
	multi := base.NewMultiSelect(base.MultiSelectConfig[int]{Same: base.Equal[int]})
	multi.SetOptions([]base.Option[int]{{Label: "one", Value: 1}, {Label: "alias", Value: 1}, {Label: "two", Value: 2}})
	multi.SetLimit(1)
	root := semanticOwner(t, multi.Element())
	result, err := root.Invoke(t.Context(), multi.Element().Target(), scene.ActionSetValue, []int{1, 1})
	if err != nil || result.State != scene.Applied || !slices.Equal(result.Value.([]int), []int{1}) {
		t.Fatalf("set values = %+v, %v", result, err)
	}
	result, err = root.Invoke(t.Context(), multi.Element().Target(), scene.ActionSetValue, []int{1})
	if err != nil || result.State != scene.Unchanged {
		t.Fatalf("repeated chosen value = %+v, %v", result, err)
	}
	for _, values := range [][]int{{1, 2}, {3}} {
		if _, err = root.Invoke(t.Context(), multi.Element().Target(), scene.ActionSetValue, values); !errors.Is(err, scene.ErrUnavailable) {
			t.Fatalf("unavailable values %v = %v", values, err)
		}
	}
	revision := semanticRevision(t, root, multi.Element())
	read, err := root.Invoke(t.Context(), multi.Element().Target(), base.ActionReadItems, base.CollectionQuery{ContentRevision: revision})
	if err != nil {
		t.Fatal(err)
	}
	choices := read.Value.(base.CollectionPage[base.ChoiceState[int]]).Items
	if len(choices) != 3 || !choices[0].Taken || !choices[1].Taken || choices[2].Taken || choices[1].Option.Value != 1 {
		t.Fatalf("choice identities and aliases = %+v", choices)
	}
	observed, err := root.Observe(scene.Query{})
	if err != nil || observed.Nodes[0].Value != "one" {
		t.Fatalf("chosen semantic value = %+v, %v", observed, err)
	}
}

func TestTreeActionsOperateOnCurrentBranches(t *testing.T) {
	tree := base.NewTree(base.Node[string]{Item: "branch", Children: []base.Node[string]{{Item: "leaf"}}})
	root := semanticOwner(t, tree.Element())
	address := base.CollectionIndex{ContentRevision: semanticRevision(t, root, tree.Element())}
	expanded, err := root.Invoke(t.Context(), tree.Element().Target(), scene.ActionExpand, address)
	if err != nil || expanded.State != scene.Applied || expanded.Value != true || len(tree.Rows()) != 2 {
		t.Fatalf("expand = %+v, %v", expanded, err)
	}
	if _, err = root.Invoke(t.Context(), tree.Element().Target(), scene.ActionCollapse, address); !errors.Is(err, scene.ErrUnavailable) {
		t.Fatalf("stale branch address = %v", err)
	}
	address.ContentRevision = semanticRevision(t, root, tree.Element())
	repeated, err := root.Invoke(t.Context(), tree.Element().Target(), scene.ActionExpand, address)
	if err != nil || repeated.State != scene.Unchanged {
		t.Fatalf("repeated expand = %+v, %v", repeated, err)
	}
	leaf := address
	leaf.Index = 1
	if _, err = root.Invoke(t.Context(), tree.Element().Target(), scene.ActionExpand, leaf); !errors.Is(err, scene.ErrUnavailable) {
		t.Fatalf("leaf expansion = %v", err)
	}
	collapsed, err := root.Invoke(t.Context(), tree.Element().Target(), scene.ActionCollapse, address)
	if err != nil || collapsed.State != scene.Applied || collapsed.Value != false || len(tree.Rows()) != 1 {
		t.Fatalf("collapse = %+v, %v", collapsed, err)
	}
}

func TestTableSemanticSortIsExplicitIdempotentAndInvalidatesRowAddresses(t *testing.T) {
	var table base.Table[int]
	table.SetItems([]int{3, 1, 2})
	table.SetSortColumns(singleSortColumn(cmp.Compare[int]))
	root := semanticOwner(t, table.Element())
	before := semanticRevision(t, root, table.Element())
	order := base.SortOrder{Column: 0, Descending: true}
	result, err := root.Invoke(t.Context(), table.Element().Target(), base.ActionSort, order)
	if err != nil || result.State != scene.Applied || !slices.Equal(table.Items(), []int{3, 2, 1}) {
		t.Fatalf("sort = %+v, %v", result, err)
	}
	returned := result.Value.(*base.SortOrder)
	returned.Column = 10
	if column, _, _ := table.Sorted(); column != 0 {
		t.Fatal("sort result aliases owned order")
	}
	result, err = root.Invoke(t.Context(), table.Element().Target(), base.ActionSort, order)
	if err != nil || result.State != scene.Unchanged {
		t.Fatalf("repeated explicit order = %+v, %v", result, err)
	}
	if _, err = root.Invoke(t.Context(), table.Element().Target(), scene.ActionSelect, base.CollectionIndex{ContentRevision: before}); !errors.Is(err, scene.ErrUnavailable) {
		t.Fatalf("pre-sort address = %v", err)
	}
	result, err = root.Invoke(t.Context(), table.Element().Target(), base.ActionClearSort, nil)
	if _, _, sorted := table.Sorted(); err != nil || result.State != scene.Applied || sorted || !slices.Equal(table.Items(), []int{3, 2, 1}) {
		t.Fatalf("clear sort = %+v, %v", result, err)
	}
}

func TestSettingsSemanticActionsKeepAcceptanceSeparateFromDomainChange(t *testing.T) {
	var settings base.Settings[string]
	settings.List().SetItems([]string{"volume"})
	root := semanticOwner(t, settings.Element())
	address := base.CollectionIndex{ContentRevision: semanticRevision(t, root, settings.Element())}
	if _, err := root.Invoke(t.Context(), settings.Element().Target(), scene.ActionIncrement, address); !errors.Is(err, scene.ErrUnavailable) {
		t.Fatalf("read-only setting = %v", err)
	}
	volume := 1
	settings.Change = func(index int, item string, action keymap.Action) scene.ActionResult[bool] {
		if index != 0 || item != "volume" {
			t.Fatal("action reached the wrong domain item")
		}
		if action != base.Increase {
			return scene.ActionResult[bool]{}
		}
		result := scene.ActionResult[bool]{Value: true}
		if volume < 2 {
			volume++
			result.State = scene.Applied
		}
		return result
	}
	for _, expected := range []scene.ActionState{scene.Applied, scene.Unchanged} {
		result, err := root.Invoke(t.Context(), settings.Element().Target(), scene.ActionIncrement, address)
		if err != nil || result.State != expected || result.Value != true || volume != 2 {
			t.Fatalf("setting result = %+v, volume=%d, error=%v", result, volume, err)
		}
	}
	declined, err := root.Invoke(t.Context(), settings.Element().Target(), scene.ActionDecrement, address)
	if err != nil || declined.State != scene.Unchanged || declined.Value != false {
		t.Fatalf("declined value action = %+v, %v", declined, err)
	}
}

func TestViewportScrollActionUsesThePresentedScrollOwner(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	viewport := base.NewViewport(base.ViewportConfig{Content: scenes.Element(&tall{rows: 10})})
	root := semanticOwner(t, viewport.Element())
	prepared, err := root.Prepare(grid.NewSurface(8, 3).View())
	if err != nil {
		t.Fatal(err)
	}
	prepared.Commit(1)
	result, err := root.Invoke(t.Context(), viewport.Element().Target(), base.ActionScroll, base.ScrollRequest{Rows: 4})
	if err != nil || result.State != scene.Applied || result.Value != 4 || viewport.Scroll().Offset() != 4 {
		t.Fatalf("scroll = %+v, %v", result, err)
	}
	viewport.SetContent(scene.Element{})
	if _, err := root.Invoke(t.Context(), viewport.Element().Target(), base.ActionScroll, base.ScrollRequest{Rows: 1}); !errors.Is(err, scene.ErrUnavailable) {
		t.Fatalf("empty viewport scroll = %v", err)
	}
}

func TestFormCancellationActionNamesThePendingAttempt(t *testing.T) {
	cancellations := 0
	form := base.NewForm(base.FormConfig{
		Submit: func(base.SubmissionID) (scene.ActionState, error) { return scene.Started, nil },
		Cancel: func(base.SubmissionID) error { cancellations++; return nil },
	})
	root := semanticOwner(t, form.Element())
	submitted, err := root.Invoke(t.Context(), form.Element().Target(), scene.ActionSubmit, nil)
	if err != nil || submitted.State != scene.Started {
		t.Fatalf("submit = %+v, %v", submitted, err)
	}
	id := submitted.Value.(base.SubmissionID)
	read, err := root.Invoke(t.Context(), form.Element().Target(), base.ActionReadSubmission, nil)
	if err != nil || read.State != scene.Unchanged || read.Value.(base.Submission).ID != id {
		t.Fatalf("pending submission discovery = %+v, %v", read, err)
	}
	cancelled, err := root.Invoke(t.Context(), form.Element().Target(), base.ActionCancel, id)
	if err != nil || cancelled.State != scene.Started || cancellations != 1 || form.Submission().State != base.SubmissionPending {
		t.Fatalf("cancel request = %+v, %v", cancelled, err)
	}
	if !form.Settle(id, base.SubmissionOutcome{Cancelled: true}) {
		t.Fatal("pending cancellation could not settle")
	}
	if _, err := root.Invoke(t.Context(), form.Element().Target(), scene.ActionSubmit, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Invoke(t.Context(), form.Element().Target(), base.ActionCancel, id); !errors.Is(err, scene.ErrUnavailable) || cancellations != 1 {
		t.Fatalf("stale attempt cancellation = %v; calls=%d", err, cancellations)
	}
}
