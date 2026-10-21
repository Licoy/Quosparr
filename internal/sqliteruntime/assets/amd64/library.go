// Package amd64 holds the pinned Windows x64 SQLite runtime from mygo v0.6.2.
package amd64

import _ "embed"

const SHA256 = "a9fa4b1eae82d8f9826d6b3cac20e337ebb29b5e859eab3d2852d65b323224f4"

//go:embed mygo-sqlite3.dll
var Library []byte
