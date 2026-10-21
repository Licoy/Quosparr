# Embedded Windows SQLite

These two runtime resources are the unmodified SQLite 3.53.4 libraries pinned by
`github.com/egoist/mygo v0.6.2/plugins/sqlite/mygo-plugin.json`, including its C ABI
shim; their SHA256 values are recorded beside the files and checked before use

Each Windows executable embeds only its own architecture, using the platform
specific imports in `internal/sqliteruntime`; macOS and Linux retain their
existing bundled dynamic libraries

SQLite is public domain; the mygo shim's MIT license is preserved in `LICENSE`
