package codex

import (
	"os"
	"sync"
	"testing"
)

func TestUpsertWithResultPreservesExistingIdentity(t *testing.T) {
	e := newEnv(t)
	auth := sampleAuth(t, sampleOpt{accountID: "shared", userID: "first", email: "first@example.com"})
	first := must(e.store.UpsertWithResult(auth, UpsertOptions{Now: testNow}))
	if !first.Created {
		t.Fatal("first upsert must create an account")
	}
	updated := must(e.store.UpsertWithResult(auth, UpsertOptions{Now: testNow.Add(1)}))
	if updated.Created || updated.Account.ID != first.Account.ID || !updated.Account.CreatedAt.Equal(first.Account.CreatedAt) {
		t.Fatal("repeat upsert must update the same account and creation time")
	}
	if _, err := os.Stat(e.store.CredentialPath(first.Account.ID)); err != nil {
		t.Fatal(err)
	}
	other := sampleAuth(t, sampleOpt{accountID: "shared", userID: "second", email: "second@example.com"})
	second := must(e.store.UpsertWithResult(other, UpsertOptions{Now: testNow}))
	if !second.Created || second.Account.ID == first.Account.ID {
		t.Fatal("different users in the same workspace must create separate accounts")
	}
}

func TestImportBatchCountsActualWrites(t *testing.T) {
	for _, preview := range []bool{false, true} {
		name := "direct"
		if preview {
			name = "preview"
		}
		t.Run(name, func(t *testing.T) {
			im, e := newImporter(t)
			first := sampleAuth(t, sampleOpt{accountID: "shared", userID: "first", email: "first@example.com"})
			second := sampleAuth(t, sampleOpt{accountID: "shared", userID: "second", email: "second@example.com"})
			var result ImportBatchResult
			if preview {
				// Preview flags deliberately do not predict the actual commits.
				items := []ImportCandidate{
					{Preview: PreviewItem{ID: "one", Conflict: ImportConflict{WillUpdate: true}}, Auth: &first},
					{Preview: PreviewItem{ID: "repeat"}, Auth: &first},
					{Preview: PreviewItem{ID: "two"}, Auth: &second},
					{Preview: PreviewItem{ID: "invalid"}},
				}
				result = im.ImportPreviewSelection(ctxBG(), items, []string{"one", "repeat", "two", "invalid"}, false)
			} else {
				result = im.importCandidates(ctxBG(), []importCandidate{{auth: &first}, {auth: &first}, {auth: &second}, {}}, false)
			}
			if result.CreatedCount != 2 || result.UpdatedCount != 1 || len(result.Succeeded) != 3 || len(result.Failures) != 1 {
				t.Fatalf("created=%d updated=%d succeeded=%d failures=%d", result.CreatedCount, result.UpdatedCount, len(result.Succeeded), len(result.Failures))
			}
			if len(must(e.store.LoadIndex()).Accounts) != 2 {
				t.Fatal("expected exactly two managed accounts")
			}
		})
	}
}

func TestImportOfficialWithResultReadsOnly(t *testing.T) {
	im, e := newImporter(t)
	e.writeOfficial(t, sampleAuth(t, sampleOpt{accountID: "official", userID: "official-user"}))
	before := string(e.officialBytes(t))
	first := must(im.ImportOfficialWithResult(true))
	second := must(im.ImportOfficialWithResult(false))
	if !first.Created || second.Created || first.Account.ID != second.Account.ID {
		t.Fatal("official imports must report create then update")
	}
	if string(e.officialBytes(t)) != before {
		t.Fatal("import changed the official auth file")
	}
}

func TestImportWriteFailureDoesNotCountSuccess(t *testing.T) {
	im, e := newImporter(t)
	auth := sampleAuth(t, sampleOpt{accountID: "write-failure"})
	noErr(t, os.MkdirAll(e.store.CredentialPath(ManagedAccountID(auth)), 0700))
	result := im.importCandidates(ctxBG(), []importCandidate{{auth: &auth}}, false)
	if result.CreatedCount != 0 || result.UpdatedCount != 0 || len(result.Succeeded) != 0 || len(result.Failures) != 1 {
		t.Fatal("failed write must not count as an addition or update")
	}
}

func TestUpsertWithResultConcurrentCreatesOnce(t *testing.T) {
	e := newEnv(t)
	auth := sampleAuth(t, sampleOpt{accountID: "concurrent", userID: "same-user"})
	const count = 8
	results := make(chan UpsertResult, count)
	var wg sync.WaitGroup
	for range count {
		wg.Go(func() {
			result, err := e.store.UpsertWithResult(auth, UpsertOptions{Now: testNow})
			if err != nil {
				t.Error(err)
				return
			}
			results <- result
		})
	}
	wg.Wait()
	close(results)
	created, succeeded := 0, 0
	for result := range results {
		succeeded++
		if result.Created {
			created++
		}
	}
	if created != 1 || succeeded != count {
		t.Fatalf("created=%d succeeded=%d", created, succeeded)
	}
}
