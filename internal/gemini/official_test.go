package gemini

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestOfficialLoadPrefersKeychainThenFiles(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	files := OfficialFiles(env.home)
	fileCred := credJSON(t, "file", "rf", "", testNow)
	writeFile(t, files[1], fileCred)
	got, err := env.store.Official.Load(ctx)
	if err != nil || !sameJSON(got, fileCred) {
		t.Fatalf("file fallback: %v", err)
	}
	keyCred := credJSON(t, "key", "rk", "", testNow)
	env.keychain.data, env.keychain.has = keyCred, true
	got, err = env.store.Official.Load(ctx)
	if err != nil || !sameJSON(got, keyCred) {
		t.Fatalf("keychain first: %v", err)
	}
}

func TestOfficialInstallWritesOnlyExistingFiles(t *testing.T) {
	env := newTestEnv(t)
	files := OfficialFiles(env.home)
	writeFile(t, files[0], credJSON(t, "old", "r0", "", testNow))
	data := credJSON(t, "new", "r1", "", testNow.Add(time.Hour))
	if err := env.store.Official.Install(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if kc, _ := env.keychain.get(); !sameJSON(kc, data) {
		t.Fatal("keychain not written")
	}
	if got, _ := os.ReadFile(files[0]); !sameJSON(got, data) {
		t.Fatal("existing file not written")
	}
	if _, err := os.Stat(files[1]); !os.IsNotExist(err) {
		t.Fatal("absent CLI file must stay absent")
	}
	if info, err := os.Stat(files[0]); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

func TestOfficialInstallRollsBackOnReadBackMismatch(t *testing.T) {
	env := newTestEnv(t)
	files := OfficialFiles(env.home)
	previous := credJSON(t, "old", "r0", "", testNow)
	env.keychain.data, env.keychain.has = previous, true
	writeFile(t, files[0], previous)
	env.keychain.corruptSaves = 1
	err := env.store.Official.Install(context.Background(), credJSON(t, "new", "r1", "", testNow))
	if err == nil {
		t.Fatal("mismatch must fail")
	}
	if kc, has := env.keychain.get(); !has || !sameJSON(kc, previous) {
		t.Fatal("keychain not restored")
	}
	if got, _ := os.ReadFile(files[0]); !sameJSON(got, previous) {
		t.Fatal("file changed")
	}
}

func TestOfficialInstallRollsBackKeychainWhenFileWriteFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions differ on Windows")
	}
	env := newTestEnv(t)
	files := OfficialFiles(env.home)
	previous := credJSON(t, "old", "r0", "", testNow)
	writeFile(t, files[1], previous)
	dir := filepath.Dir(files[1])
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	err := env.store.Official.Install(context.Background(), credJSON(t, "new", "r1", "", testNow))
	if err == nil {
		t.Fatal("write into a read-only directory must fail")
	}
	if _, has := env.keychain.get(); has {
		t.Fatal("keychain item created by the failed install must be removed")
	}
	if got, _ := os.ReadFile(files[1]); !sameJSON(got, previous) {
		t.Fatal("file changed")
	}
}
