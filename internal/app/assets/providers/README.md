# Icon assets

Provider SVGs are unmodified files from [LobeHub Icons](https://icons.lobehub.com), npm `@lobehub/icons-static-svg` version **1.95.1**, MIT license

- `codex-color.svg`, `claude-color.svg`, `gemini-color.svg`: original color artwork
- `codex.svg`, `grok.svg`, `claude.svg`, `gemini.svg`: original `currentColor` monochrome artwork
- `LOBEHUB-LICENSE`: upstream MIT notice at git commit `49a2130df7bfa5eb1b088261bff20a37e2967789`

The official [npm archive](https://registry.npmjs.org/@lobehub/icons-static-svg/-/icons-static-svg-1.95.1.tgz) is pinned by SHA-512 integrity `Hw7EPPgVnC4NZLXBfTNJG6hyQgqECfUPC11VVXodPSr1aebKcFxDZlSpxhWwYNdCc6bhxps/x5TtXoPmfKH2ag==`

Interface glyphs in `internal/app/glyphs_gen.go` come from [Remix Icon](https://remixicon.com), npm `remixicon` **4.9.1**, whose included `License` file is **Remix Icon License v1.0 (January 2026)**, preserved in `REMIXICON-LICENSE`; the npm metadata still labels this version Apache-2.0, but the included license file governs this attribution

Regenerate both sets from the repository root with `go run ./Scripts/gen-glyphs`; only generation downloads the pinned official packages, app builds use local embedded assets
