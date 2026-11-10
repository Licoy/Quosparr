package codexdesktop

import (
	"errors"
	"os"
	"testing"
)

func TestSourcesSkipMalformedCandidateButSurfaceTotalValidationFailure(t *testing.T) {
	f := newFixture(t)
	doc := f.active(nil)
	doc[rendererKey].(map[string]any)["inactive"].(map[string]any)["bad-source"] = map[string]any{"globals": map[string]any{"local-projects": false}, "atoms": map[string]any{}}
	f.write(doc)
	sources, err := f.sharer.Sources()
	must(t, err)
	if len(sources) != 1 || sources[0].Key != alice.Key() {
		t.Fatal("bad candidate blocked valid source")
	}
	doc = f.active(nil)
	delete(doc, "app-server-project-id-by-legacy-project-id-by-host")
	f.write(doc)
	_, err = f.sharer.Sources()
	if !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
}
func TestMalformedTargetFieldFailsExplicitly(t *testing.T) {
	f := newFixture(t)
	target := f.globals("p2")
	target["project-order"] = false
	f.write(f.active(target))
	_, err := f.sharer.Prepare(Request{alice.Key(), bob})
	if !errors.Is(err, ErrUnknownFormat) {
		t.Fatal(err)
	}
}
func TestDirectoryChangedAfterPreviewIsRejected(t *testing.T) {
	f := newFixture(t)
	f.write(f.active(nil))
	p, err := f.sharer.Prepare(Request{alice.Key(), bob})
	must(t, err)
	must(t, os.Remove(f.roots["p1"]))
	_, err = f.sharer.Apply(p)
	if !errors.Is(err, ErrStateChanged) {
		t.Fatal(err)
	}
}
func TestTargetProjectlessThreadIsOwnershipConflict(t *testing.T) {
	f := newFixture(t)
	target := f.globals("p2")
	target["projectless-thread-ids"] = []string{"t1"}
	doc := f.active(target)
	doc["thread-project-assignments"] = map[string]any{"t1": map[string]any{"projectId": "p1", "projectKind": "local"}}
	f.write(doc)
	_, err := f.sharer.Prepare(Request{alice.Key(), bob})
	if !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}
func TestMappingCannotClaimAnotherLegacyProject(t *testing.T) {
	f := newFixture(t)
	target := f.globals("p2")
	target["app-server-project-id-by-legacy-project-id-by-host"].(map[string]any)["local:"+f.sharer.CodexHome].(map[string]any)["p2"] = "db-p1"
	f.write(f.active(target))
	_, err := f.sharer.Prepare(Request{alice.Key(), bob})
	if !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}
