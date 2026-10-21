// Package arm64 holds the pinned Windows ARM64 SQLite runtime from mygo v0.6.2.
package arm64

import _ "embed"

const SHA256 = "6f43349dc4d410d490324b62eb4137f9b6c021f6587cc104ec6213d31a8b3c9a"

//go:embed mygo-sqlite3.dll
var Library []byte
