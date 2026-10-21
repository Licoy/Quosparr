// release-assets signs final, post-processed archives using mygo v0.6.2's
// Ed25519-over-SHA256 protocol, and writes the Sparkle feed of the Swift
// releases; private keys are read only from the environment.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type config struct {
	Version string                                             `json:"version"`
	Updates struct{ PublicKey, GitHub, TagPrefix, URL string } `json:"updates"`
}
type manifest struct {
	Version   string `json:"version"`
	Notes     string `json:"notes"`
	Date      string `json:"date"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	Signature string `json:"signature"`
	SHA256    string `json:"sha256"`
}

var targets = []string{"darwin-arm64", "darwin-amd64", "windows-amd64", "windows-arm64", "linux-amd64", "linux-arm64"}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) > 0 && args[0] == "verify-sparkle" {
		return verifySparkle(args[1:])
	}
	if len(args) > 0 && args[0] == "sparkle-appcast" {
		return sparkleAppcast(args[1:])
	}
	if len(args) == 0 {
		return errors.New("usage: release-assets sign|verify|notes -config mygo.json -dir dist [-targets target,...] [-notes file] | sparkle-appcast | verify-sparkle")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	cfgPath := flags.String("config", "mygo.json", "configuration")
	dir := flags.String("dir", "dist", "flat asset directory")
	selected := flags.String("targets", strings.Join(targets, ","), "targets")
	notesPath := flags.String("notes", "", "Markdown release notes for the update window")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	var notes string
	if args[0] == "notes" {
		data, err := os.ReadFile(*notesPath)
		if err != nil {
			return err
		}
		notes = strings.TrimSpace(string(data))
	}
	var cfg config
	if err := readJSON(*cfgPath, &cfg); err != nil {
		return err
	}
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?(?:\+[A-Za-z0-9.-]+)?$`).MatchString(cfg.Version) {
		return errors.New("invalid product version")
	}
	var pub []byte
	var err error
	if args[0] != "verify-payload" {
		pub, err = base64.StdEncoding.DecodeString(strings.TrimSpace(cfg.Updates.PublicKey))
		if err != nil || len(pub) != ed25519.PublicKeySize {
			return errors.New("invalid updates.publicKey")
		}
	}
	for _, target := range strings.Split(*selected, ",") {
		if !validTarget(target) {
			return fmt.Errorf("unsupported target: %s", target)
		}
		switch args[0] {
		case "verify-payload":
			err = verifyArchive(filepath.Join(*dir, archiveName(cfg.Version, target)), target)
		case "sign":
			err = sign(cfg, *dir, target)
		case "verify":
			err = verify(cfg, *dir, target)
		case "notes":
			err = setNotes(*dir, target, notes)
		default:
			return errors.New("command must be sign, verify, notes, or verify-payload")
		}
		if err != nil {
			return fmt.Errorf("%s: %w", target, err)
		}
		fmt.Printf("%s %s verified\n", args[0], target)
	}
	return nil
}
func validTarget(target string) bool {
	for _, t := range targets {
		if t == target {
			return true
		}
	}
	return false
}
func archiveName(version, target string) string {
	return "quosparr-" + version + "-" + target + ".tar.gz"
}
func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
func digest(path string) ([]byte, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, f)
	return hash.Sum(nil), size, err
}
func signingKey(pub []byte) (ed25519.PrivateKey, error) {
	text := os.Getenv("MYGO_UPDATE_PRIVATE_KEY")
	if text == "" {
		text = os.Getenv("MYGO_UPDATER_PRIVATE_KEY")
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("missing or invalid mygo signing key")
	}
	private := ed25519.PrivateKey(key)
	if !ed25519.PublicKey(pub).Equal(private.Public()) {
		return nil, errors.New("mygo signing key does not match configuration")
	}
	return private, nil
}
func sign(cfg config, dir, target string) error {
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cfg.Updates.PublicKey))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("invalid update public key")
	}
	key, err := signingKey(pub)
	if err != nil {
		return err
	}
	name := archiveName(cfg.Version, target)
	sum, size, err := digest(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	prefix := cfg.Updates.TagPrefix
	if prefix == "" {
		prefix = "v"
	}
	base := strings.TrimSuffix(cfg.Updates.URL, "/")
	if base == "" {
		base = "https://github.com/" + cfg.Updates.GitHub + "/releases/download/" + url.PathEscape(prefix+cfg.Version)
	}
	m := manifest{Version: cfg.Version, Date: time.Now().UTC().Format(time.RFC3339), URL: base + "/" + url.PathEscape(name), Size: size, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, sum)), SHA256: hex.EncodeToString(sum)}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "update-"+target+".json"), append(data, '\n'), 0644)
}
// setNotes adds the release notes the update window shows; mygo signs only
// the archive digest, so the manifest signature stays valid
func setNotes(dir, target, notes string) error {
	path := filepath.Join(dir, "update-"+target+".json")
	var m manifest
	if err := readJSON(path, &m); err != nil {
		return err
	}
	m.Notes = notes
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
func verify(cfg config, dir, target string) error {
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cfg.Updates.PublicKey))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("invalid update public key")
	}
	var m manifest
	if err := readJSON(filepath.Join(dir, "update-"+target+".json"), &m); err != nil {
		return err
	}
	if m.Version != cfg.Version {
		return errors.New("manifest version mismatch")
	}
	u, err := url.Parse(m.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || filepath.Base(u.Path) != archiveName(cfg.Version, target) {
		return errors.New("invalid archive URL or target")
	}
	prefix := cfg.Updates.TagPrefix
	if prefix == "" {
		prefix = "v"
	}
	if cfg.Updates.URL == "" && m.URL != "https://github.com/"+cfg.Updates.GitHub+"/releases/download/"+prefix+cfg.Version+"/"+archiveName(cfg.Version, target) {
		return errors.New("archive release tag mismatch")
	}
	if cfg.Updates.URL != "" && m.URL != strings.TrimSuffix(cfg.Updates.URL, "/")+"/"+archiveName(cfg.Version, target) {
		return errors.New("archive channel mismatch")
	}
	path := filepath.Join(dir, filepath.Base(u.Path))
	sum, size, err := digest(path)
	if err != nil {
		return err
	}
	if size != m.Size || size <= 0 {
		return errors.New("archive length mismatch")
	}
	if m.SHA256 != hex.EncodeToString(sum) {
		return errors.New("archive checksum mismatch")
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil || !ed25519.Verify(pub, sum, sig) {
		return errors.New("invalid archive signature")
	}
	return verifyArchive(path, target)
}

// Sparkle signs the archive bytes, while mygo signs their SHA256 digest
func verifySparkle(args []string) error {
	flags := flag.NewFlagSet("verify-sparkle", flag.ContinueOnError)
	cfgPath := flags.String("config", "mygo.json", "configuration")
	file := flags.String("file", "", "archive")
	signature := flags.String("signature", "", "public signature")
	if err := flags.Parse(args); err != nil {
		return err
	}
	var cfg struct {
		MacOS struct {
			InfoPlist struct{ SUPublicEDKey string }
		}
	}
	if err := readJSON(*cfgPath, &cfg); err != nil {
		return err
	}
	pub, err := base64.StdEncoding.DecodeString(cfg.MacOS.InfoPlist.SUPublicEDKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("invalid Sparkle public key")
	}
	sig, err := base64.StdEncoding.DecodeString(*signature)
	if err != nil {
		return errors.New("invalid Sparkle signature")
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, data, sig) {
		return errors.New("Sparkle signature does not match SUPublicEDKey")
	}
	return nil
}
