package codexruntime

import (
	"os"
	"path/filepath"
)

// The official home entry can link into the current user's private temporary directory.
func trustedSocket(path string) (string, os.FileInfo, error) {
	if !trustedParents(path) {
		return "", nil, failure("connection", "untrusted_socket")
	}
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil || !parent.IsDir() || !trusted(parent) {
		return "", nil, failure("connection", "untrusted_socket")
	}
	link, err := os.Lstat(path)
	if err != nil || !owned(link) {
		return "", nil, failure("connection", "untrusted_socket")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", nil, failure("connection", "untrusted_socket")
	}
	if !trustedParents(resolved) {
		return "", nil, failure("connection", "untrusted_socket")
	}
	parent, err = os.Stat(filepath.Dir(resolved))
	if err != nil || !parent.IsDir() || !trusted(parent) {
		return "", nil, failure("connection", "untrusted_socket")
	}
	info, err := os.Lstat(resolved)
	if err != nil || info.Mode()&os.ModeSocket == 0 || !trusted(info) {
		return "", nil, failure("connection", "untrusted_socket")
	}
	return resolved, info, nil
}

func trustedParents(path string) bool {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil || !trustedDirectory(info) {
			return false
		}
		if dir == filepath.Dir(dir) {
			return true
		}
	}
}

func trustedDirectoryPath(path string) bool {
	resolved, err := filepath.EvalSymlinks(path)
	return err == nil && trustedParents(filepath.Join(path, "entry")) && trustedParents(filepath.Join(resolved, "entry"))
}
