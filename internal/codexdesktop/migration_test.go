package codexdesktop

import "testing"

// Machine settings stay at the document root when account-scoped globals move.
func moveAccountFieldsOnly(doc map[string]any, next string) {
	machine := map[string]any{}
	for _, key := range []string{"theme", "window-placement"} {
		if v, ok := doc[key]; ok {
			machine[key] = v
			delete(doc, key)
		}
	}
	moveOwner(doc, next)
	for key, v := range machine {
		doc[key] = v
	}
}
func TestPreviewAllowsAccountOnlyMigrationWithMachineGlobalsUnchanged(t *testing.T) {
	for _, startOwner := range []string{alice.Key(), bob.Key()} {
		t.Run(startOwner, func(t *testing.T) {
			f := newFixture(t)
			doc := f.active(f.globals("p2"))
			doc["window-placement"] = map[string]any{"x": 100, "y": 200}
			if startOwner == bob.Key() {
				moveAccountFieldsOnly(doc, bob.Key())
			}
			f.write(doc)
			plan, err := f.sharer.Prepare(Request{alice.Key(), bob})
			must(t, err)
			doc = f.read()
			next := bob.Key()
			if startOwner == bob.Key() {
				next = alice.Key()
			}
			moveAccountFieldsOnly(doc, next)
			f.write(doc)
			outcome, err := f.sharer.Apply(plan)
			must(t, err)
			if outcome != Shared || !hasProject(f.target(), "p1") || !hasProject(f.target(), "p2") {
				t.Fatal("account migration corrupted projects")
			}
			final := f.read()
			if final["theme"] != "dark" {
				t.Fatal("machine theme lost")
			}
			if final["window-placement"] == nil {
				t.Fatal("window placement lost")
			}
		})
	}
}
func TestPreviewAllowsUnrelatedSettingsButRejectsProjectlessChanges(t *testing.T) {
	for _, related := range []bool{false, true} {
		t.Run(map[bool]string{false: "setting", true: "projectless"}[related], func(t *testing.T) {
			f := newFixture(t)
			f.write(f.active(f.globals("p2")))
			plan, err := f.sharer.Prepare(Request{alice.Key(), bob})
			must(t, err)
			doc := f.read()
			target := doc[rendererKey].(map[string]any)["inactive"].(map[string]any)[bob.Key()].(map[string]any)["globals"].(map[string]any)
			if related {
				target["projectless-thread-ids"] = []string{"t1"}
			} else {
				target["theme"] = "new-theme"
			}
			f.write(doc)
			_, err = f.sharer.Apply(plan)
			if related {
				if err != ErrStateChanged {
					t.Fatal(err)
				}
			} else {
				must(t, err)
				if !sameJSON(f.target().Globals["theme"], encode("new-theme")) {
					t.Fatal("target setting overwritten")
				}
			}
		})
	}
}
