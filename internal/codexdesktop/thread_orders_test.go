package codexdesktop

import "testing"

func TestSidebarObjectOrderSupportsSourcesAndPrepare(t *testing.T) {
	f := newFixture(t)
	doc := f.active(nil)
	doc["thread-project-assignments"] = map[string]any{"t1": map[string]any{"projectId": "p1", "projectKind": "local"}}
	doc["sidebar-project-thread-orders"] = map[string]any{"p1": map[string]any{"sortKey": "created_at", "threadIds": []string{"t1", "missing"}}, "unrelated-project": true}
	f.write(doc)
	t.Run("Sources", func(t *testing.T) {
		sources, err := f.sharer.Sources()
		must(t, err)
		if len(sources) != 1 || sources[0].Threads != 1 {
			t.Fatalf("sources=%+v", sources)
		}
	})
	t.Run("Prepare", func(t *testing.T) {
		plan, err := f.sharer.Prepare(Request{alice.Key(), bob})
		must(t, err)
		if plan.Summary.Threads != 1 {
			t.Fatalf("summary=%+v", plan.Summary)
		}
	})
	t.Run("Apply", func(t *testing.T) {
		plan, err := f.sharer.Prepare(Request{alice.Key(), bob})
		must(t, err)
		_, err = f.sharer.Apply(plan)
		must(t, err)
		orders, _ := decodeObject(f.target().Globals["sidebar-project-thread-orders"])
		if len(orders) != 1 || orders["p1"] == nil {
			t.Fatalf("unrelated order copied: %s", encode(orders))
		}
	})

}

func orderFixture(kind string, ids []string) any {
	if kind == "legacy" {
		return ids
	}
	value := map[string]any{"threadIds": ids}
	if kind != "unspecified" {
		value["sortKey"] = kind
	}
	return value
}
func TestSidebarOrderMergeAndLegacyNormalization(t *testing.T) {
	for _, sourceKind := range []string{"created_at", "updated_at", "unspecified", "legacy"} {
		for _, targetKind := range []string{"missing", "created_at", "updated_at", "unspecified", "legacy"} {
			t.Run(sourceKind+"/"+targetKind, func(t *testing.T) {
				f := newFixture(t)
				var target map[string]any
				if targetKind != "missing" {
					target = f.globals("p1")
					target["sidebar-project-thread-orders"] = map[string]any{"p1": orderFixture(targetKind, []string{"t2", "target-only"})}
				}
				doc := f.active(target)
				doc["thread-project-assignments"] = map[string]any{
					"t1": map[string]any{"projectId": "p1", "projectKind": "local"},
					"t2": map[string]any{"projectId": "p1", "projectKind": "local"},
				}
				doc["sidebar-project-thread-orders"] = map[string]any{"p1": orderFixture(sourceKind, []string{"t1", "missing", "t2"})}
				f.write(doc)
				sources, err := f.sharer.Sources()
				must(t, err)
				if len(sources) == 0 || sources[0].Threads != 2 {
					t.Fatalf("sources=%+v", sources)
				}
				plan, err := f.sharer.Prepare(Request{alice.Key(), bob})
				must(t, err)
				if plan.Summary.Threads != 2 {
					t.Fatalf("summary=%+v", plan.Summary)
				}
				outcome, err := f.sharer.Apply(plan)
				must(t, err)
				if outcome != Shared {
					t.Fatal(outcome)
				}
				orders, _ := decodeObject(f.target().Globals["sidebar-project-thread-orders"])
				ids := []string{"t2", "target-only", "t1"}
				sortKind := targetKind
				if targetKind == "missing" {
					ids = []string{"t1", "t2"}
					sortKind = sourceKind
				}
				want := map[string]any{"threadIds": ids}
				if sortKind == "created_at" || sortKind == "updated_at" {
					want["sortKey"] = sortKind
				}
				if !sameJSON(orders["p1"], encode(want)) {
					t.Fatalf("order=%s want=%s", orders["p1"], encode(want))
				}
				if f.share() != Unchanged {
					t.Fatal("second merge changed order")
				}
			})
		}
	}
}
func TestSidebarOrderRejectsUnknownSourceAndTargetFormats(t *testing.T) {
	malformed := map[string]any{
		"wrong-sort":    map[string]any{"threadIds": []string{"t1"}, "sortKey": "manual"},
		"null-sort":     map[string]any{"threadIds": []string{"t1"}, "sortKey": nil},
		"wrong-ids":     map[string]any{"threadIds": []any{1}, "sortKey": "created_at"},
		"missing-ids":   map[string]any{"sortKey": "created_at"},
		"null-ids":      map[string]any{"threadIds": nil},
		"unknown-field": map[string]any{"threadIds": []string{"t1"}, "unknown": true},
		"scalar":        true,
	}
	for name, raw := range malformed {
		for _, side := range []string{"source", "target"} {
			t.Run(name+"/"+side, func(t *testing.T) {
				f := newFixture(t)
				target := f.globals("p1")
				doc := f.active(target)
				doc["thread-project-assignments"] = map[string]any{"t1": map[string]any{"projectId": "p1", "projectKind": "local"}}
				doc["sidebar-project-thread-orders"] = map[string]any{"p1": map[string]any{"threadIds": []string{"t1"}}}
				if side == "source" {
					doc["sidebar-project-thread-orders"] = map[string]any{"p1": raw}
				} else {
					target["sidebar-project-thread-orders"] = map[string]any{"p1": raw}
				}
				f.write(doc)
				before := encode(f.read())
				_, err := f.sharer.Prepare(Request{alice.Key(), bob})
				if err != ErrUnknownFormat {
					t.Fatalf("err=%v", err)
				}
				if !sameJSON(before, encode(f.read())) || len(f.backups()) != 0 {
					t.Fatal("invalid order changed files")
				}
			})
		}
	}
}
func TestSidebarOrderKeepsEmptyObjectAndFiltersOtherProjectThreads(t *testing.T) {
	f := newFixture(t)
	doc := f.active(nil)
	for key, value := range f.globals("p1", "p2") {
		doc[key] = value
	}
	doc["thread-project-assignments"] = map[string]any{"t1": map[string]any{"projectId": "p2", "projectKind": "local"}}
	doc["sidebar-project-thread-orders"] = map[string]any{"p1": map[string]any{"sortKey": "updated_at", "threadIds": []string{"t1", "missing"}}}
	f.write(doc)
	plan, err := f.sharer.Prepare(Request{alice.Key(), bob})
	must(t, err)
	_, err = f.sharer.Apply(plan)
	must(t, err)
	orders, _ := decodeObject(f.target().Globals["sidebar-project-thread-orders"])
	if !sameJSON(orders["p1"], encode(map[string]any{"sortKey": "updated_at", "threadIds": []string{}})) {
		t.Fatalf("order=%s", orders["p1"])
	}
}
func TestSidebarTargetOrderChangeAfterPreviewIsRejected(t *testing.T) {
	f := newFixture(t)
	target := f.globals("p1")
	target["sidebar-project-thread-orders"] = map[string]any{"p1": map[string]any{"sortKey": "created_at", "threadIds": []string{"t2"}}}
	doc := f.active(target)
	doc["thread-project-assignments"] = map[string]any{"t1": map[string]any{"projectId": "p1", "projectKind": "local"}}
	f.write(doc)
	plan, err := f.sharer.Prepare(Request{alice.Key(), bob})
	must(t, err)
	doc = f.read()
	target = doc[rendererKey].(map[string]any)["inactive"].(map[string]any)[bob.Key()].(map[string]any)["globals"].(map[string]any)
	target["sidebar-project-thread-orders"] = map[string]any{"p1": map[string]any{"sortKey": "updated_at", "threadIds": []string{"t2"}}}
	f.write(doc)
	_, err = f.sharer.Apply(plan)
	if err != ErrStateChanged {
		t.Fatal(err)
	}
}
