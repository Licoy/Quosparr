English · [简体中文](./README_ZH.md) · [繁體中文](./README_ZH_HANT.md) · [한국어](./README_KO.md) · [日本語](./README_JA.md) · [Русский](./README_RU.md) · [Français](./README_FR.md)

<div align="center">

<img src="Resources/AppIcon.png" alt="Quosparr logo" width="128" height="128">

# Quosparr

<sub><i>Named from <b>Quo</b>ta + <b>Sparr</b>ow</i></sub>

A small sparrow perched in your menu bar, keeping watch over your Codex, Claude, and Grok quota.

Quosparr is a native menu bar / system tray app for macOS, Windows, and Linux. It shows Codex, Claude Code, and Grok subscription quota, reset credits, API-equivalent cost, and local sessions, with separate account management for each provider. Global skills, MCP servers and local sessions for all three can be managed in one place.

</div>

## Highlights

**Codex**

- Remaining quota for the 5-hour, weekly, and additional windows, plus the current account, subscription tier, and expiration.
- Reset credit count, status, and expiration time.
- Weekly allowance estimate from the official weekly percent and daily Credits, compared with previous weeks to spot quota cuts.
- Codex Reset Updates: whether Codex rate limits have reset today, with its own refresh interval (data from [Did Codex Reset](https://didcodexreset.com)).
- Provider accounts: browser sign-in, import local `auth.json`, paste token / JSON (including `/auth/session`), import files, or add an API key. Switching after confirmation atomically writes `~/.codex/auth.json`.
- API accounts: a Base URL plus API key (empty Base URL means the official OpenAI API), with an optional model fetched from the site. Switching writes `auth.json` and the `quosparr` provider in `config.toml` (backed up first), and the balance of new-api-style or OpenAI-compatible relays shows in the account list, panel, status bar, floating window, and widget. A key login whose `config.toml` already selects a relay is picked up with that site and model. Editing the account in use can replace its key (empty keeps it) and asks whether to rewrite the config files now; restart Codex afterwards.
- Recent sessions, projects, and status, plus repair of the local session index.

**Claude**

- Claude Code subscription usage for the 5-hour window, the 7-day window, per-model weekly windows, and extra usage. This reading is not a documented public billing API.
- Local token usage, API-equivalent cost (`anthropic-builtin-2026-10-14`), recent sessions, and output speed from Claude Code session logs. Each reply is counted once. Unknown models are not given an exact cost. The logs have no time-to-first-token, so that column is an em dash and speed uses end-to-end time.
- Subscription accounts: sign in through the browser without running the CLI first, import the current login, paste credentials, or import a file; browser sign-in supports cancellation and pasting the callback, adds only to the account library, and requires an explicit switch to use the account in Claude Code. Quosparr does not refresh the current account; run `claude` once if the login expires. Other managed accounts refresh into their library copies only.
- On macOS the official credential stays in the Keychain and is read and written through `/usr/bin/security`. On Windows and Linux it is `<config>/.credentials.json`. A switch replaces only `claudeAiOauth`, keeps MCP tokens, and updates only `oauthAccount` in `.claude.json`. A running Claude Code process is not terminated and may write the previous login back, so the app warns before switching.
- API accounts: a Base URL plus API key, with optional Opus / Sonnet / Haiku / Fable model mapping fetched from the site. Switching writes only `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, and the mapping into `settings.json` (backed up first) and leaves the subscription login untouched; switching back removes them. The relay balance shows wherever Claude quota does. A relay already set in `settings.json` is imported automatically. Editing the account in use can replace its key (empty keeps it) and asks whether to rewrite `settings.json` now; restart Claude Code afterwards.

- **Claude Desktop subscriptions**: bind from either account list, manually sign in to the shown account and organization, then confirm it in Quosparr. Every operation asks you to save your tasks and agree to quit Desktop and save/restore its complete local configuration, including sign-in, settings and local state. Finishing or cancelling binding restores the previous configuration. Linked switching is optional and off by default; the current CLI account can also open its bound Desktop independently. CLI and Desktop results are shown separately, and starting Desktop does not verify the signed-in account.
- Desktop profiles, including Desktop’s native encrypted sign-in data, stay privately in `~/.quosparr/claude-desktop` (protected user-only ACLs on Windows), keep the official Desktop data path, and are never decrypted or included in account exports. The original configuration is preserved separately without importing its account; browser and other-app data are not read. Profiles must share a volume with Desktop data, and interrupted operations have an explicit restore action. API accounts are excluded. Real-device and Desktop-version compatibility acceptance on macOS, Windows and Linux is still pending.

**Grok**

- Included quota, Build / Imagine / Chat breakdown, weekly / monthly periods, prepaid balance, and on-demand usage via the official CLI chat-proxy billing API.
- Multiple OAuth / SuperGrok accounts with isolated sign-in, current-login import, credential paste, refresh, aliases, ordering, and explicit switching.
- Recent sessions from local Grok CLI logs.

**All providers**

- Settings › Model prices sits immediately before Advanced, in the same sidebar group; automatic sync from the public models.dev catalog defaults to every hour, can be disabled, and supports presets or a custom interval from 15 minutes to 7 days, with immediate saving and manual sync available even when automatic sync is off
- Model prices covers reference token rates in USD per million tokens, not all image, video or other modality charges; a catalog reference can differ from the rate actually used when verified context tiers, cache write TTL, Fast or regional rules must be preserved; details show the source and version, and the dated built-in tables below remain the offline baseline; Only OpenAI, Anthropic and xAI entries under their original provider IDs and their built-in prices are shown in fixed Codex / Claude / Grok tabs, starting with Codex, with no other-provider option; third-party relays and other providers’ same-name models are excluded, and models.dev is an aggregated public catalog rather than a live official provider interface; Sync settings opens a native modal from the button beside the tabs; automatic sync, frequency and detailed status are shared by all three providers, changes save immediately, and closing the modal keeps sync tasks running
- API-equivalent costs re-estimate historical local usage at current prices; partially priced totals, days, projects, models and sessions are marked, with a complete list of missing prices or rules; missing prices appear as — and explicitly free prices as $0; quota and reset credits keep their existing meaning
- Price sync only downloads a public catalog through the saved proxy policy and never uploads session logs; offline use keeps built-in prices and the last successfully saved catalog, and sync failures remain visible
- Always-visible `Codex | Claude | Grok` tabs. On macOS, Settings › Menu Bar Item previews five styles: quota bars, countdown, battery (horizontal or vertical), quota rings (values optionally inside the rings) and progress text. The menu bar follows the panel or shows any of the three providers, each on its own; quota bars, countdowns and batteries can also stack two providers in one item. An existing “Codex + Grok” choice stays that pair.
- An optional always-on-top desktop floating window shows the remaining quota of up to three providers in four styles: capsule, card, rings, or battery. Click it to open the panel, drag it to move it, and right-click it for more options. Opacity, position lock, and whether it stays above full-screen apps are configurable.
- API-equivalent cost and token usage for today, the current cycle, the previous cycle, this month, or a custom range.
- Year-to-date token usage chart as a heatmap, line, or bar chart (daily / weekly / cumulative).
- Skills page for each provider's global skills: where each one comes from, whether the CLI loads it, and which other CLIs share it; search, read `SKILL.md`, open its folder, turn it on or off in the CLI's own config (backed up first), copy or move it to another CLI (never overwriting a skill of the same name), move it to the Quosparr skills trash, and spot broken links, nested folders or duplicate names, with a one-click fix for most of them; several skills can be selected to delete or fix together, and every fix lists its risks before you confirm. Settings › Skills has a tab per provider with state and type filters, a detail page per skill (install source, files, frontmatter, links to its GitHub repository or homepage) and a trash where deleted skills can be restored or sent to the system Trash. Settings › Skills can also add skills from a public GitHub repository (URL or `owner/repo`), a local folder or a zip: Quosparr finds skill folders at any depth, leaves out version control, build output and system files, lets you pick the skills and the target CLIs (or the shared `~/.agents/skills` folder), and never overwrites a folder of the same name. Skills added from GitHub remember their source and offer an update check; an update replaces the folder with the repository's latest version and moves the old one to the skills trash.
- Settings › Sessions lists every local session per provider. Search by title, project or ID, filter by project and date range, and open a session to see its model, token breakdown, API-equivalent cost, files and a conversation preview. Selected sessions are moved to the system Trash one by one after you confirm, never deleted outright; official indexes and databases stay untouched, and derived statistics are recalculated from the remaining logs.
- Settings › MCP lists the user-level global MCP servers of each provider (`config.toml` for Codex and Grok, `.claude.json` for Claude). Only a redacted summary of the command, site and structure is shown, never the values of env or headers, and Quosparr does not connect to or launch any server. Selected entries can be deleted together; the file is backed up to `~/.quosparr/backups` first, other settings are kept, and the change applies after you restart the CLI. Codex and Grok servers also have an on/off switch, saved after the same backup: Codex writes `enabled` on the entry, and Grok also updates `disabled_mcp_servers` as its own CLI does. Claude Code keeps MCP on/off per project, so Claude servers have no switch. Project, plugin and enterprise-managed MCP servers are not shown.
- Light, dark, and system appearance; English, 简体中文, 繁體中文, 한국어, 日本語, Русский, Français.
- Built-in updates, and desktop widgets on macOS 14+.

## Screenshots

<table>
  <tr>
    <th colspan="2">Main panel</th>
    <th colspan="2">API-equivalent cost</th>
    <th colspan="2">Subscription quota estimate</th>
  </tr>
  <tr>
    <td colspan="2" align="center" valign="top"><img src="docs/images/1-en.webp?v2" alt="Quosparr Main panel" width="240"></td>
    <td colspan="2" align="center" valign="top"><img src="docs/images/3.webp?v1" alt="Quosparr API-equivalent cost" width="240"></td>
    <td colspan="2" align="center" valign="top"><img src="docs/images/6.webp?v1" alt="Quosparr Subscription quota estimate" width="240"></td>
  </tr>
  <tr>
    <th colspan="3">Provider accounts</th>
    <th colspan="3">Display settings</th>
  </tr>
  <tr>
    <td colspan="3" align="center" valign="top"><img src="docs/images/4.webp?v1" alt="Quosparr Provider accounts" width="370"></td>
    <td colspan="3" align="center" valign="top"><img src="docs/images/5.webp?v1" alt="Quosparr Display settings" width="370"></td>
  </tr>
  <tr>
    <th colspan="6">Windows settings and Grok panel</th>
  </tr>
  <tr>
    <td colspan="6" align="center" valign="top"><img src="docs/images/2.webp?v1" alt="Quosparr Windows settings and Grok panel" width="780"></td>
  </tr>
</table>

The screenshots show the v1 interface on macOS and Windows.

## Installation

Download from [GitHub Releases](https://github.com/Licoy/Quosparr/releases) and pick the package for your system and CPU:

| System | Package | How to install |
| --- | --- | --- |
| macOS 12+ (Apple Silicon / Intel) | `Quosparr-macos-arm64.dmg` / `Quosparr-macos-x86_64.dmg`, or the `.zip` with the same name | Drag `Quosparr.app` into `Applications` |
| Windows 10 / 11 (amd64 / arm64) | `Quosparr.Setup.<version>.<arch>.exe` | Per-user install into `%LOCALAPPDATA%\Programs\Quosparr`; no administrator rights needed |
| Linux (amd64 / arm64) | `quosparr-<version>-linux-<arch>.tar.gz` with `install.sh`, or `quosparr_<version>_<arch>.deb` | Run `sh install.sh` next to the tarball to install into `~/.local`, or install the deb with your package manager |

Every package ships the SQLite runtime for its architecture, so the first launch never downloads anything. On first launch, Quosparr enables launch at login by default; change it in **Control Panel → General → Launch at login**.

Homebrew installation is not available yet; use the DMG or ZIP for now.

If you used CodexRunway before, see the [upgrade guide](docs/upgrade-from-codexrunway.md).

### macOS security prompts

macOS builds are currently ad-hoc signed and not notarized. If macOS says the developer cannot be verified, right-click `Quosparr.app` and choose Open, or go to System Settings → Privacy & Security and click Open Anyway.

If macOS says the app is damaged, it is usually the download quarantine attribute. After moving the app into `Applications`, run:

```bash
xattr -dr com.apple.quarantine /Applications/Quosparr.app
```

### Linux requirements

Linux needs GTK3, AppIndicator, a desktop with tray support, and a working Secret Service session (for example GNOME Keyring or KWallet) to store proxy credentials. The deb declares these dependencies and is updated through the package manager.

## Requirements

- Codex installed and used on this machine is recommended. Codex data comes from `CODEX_HOME` (default `~/.codex`, or `%USERPROFILE%\.codex` on Windows).
- Only the `file` credential store is supported (the default when `cli_auth_credentials_store` is unset in Codex `config.toml`). `keyring`, `auto`, and `ephemeral` are reported as unsupported and block importing and switching accounts.
- The Claude panel reads Claude Code configuration from `CLAUDE_CONFIG_DIR` when set, otherwise `~/.claude`. Subscription usage needs a Claude.ai login; API keys and gateways do not provide it. Browser sign-in does not require the CLI and writes only to the account library until you explicitly switch. Existing CLI logins are added automatically; you can also paste credential JSON or import a file. Real browser authorization and switching still require isolated account acceptance.
- The Grok panel requires the [official Grok CLI](https://docs.x.ai/build/overview):

  ```bash
  curl -fsSL https://x.ai/cli/install.sh | bash
  ```

- Grok accounts and quota support OAuth / SuperGrok and compatible legacy sessions only. Run `grok login --oauth`, sign in from the app, import the current login, or paste credential JSON. API-key-only logins are not added as managed accounts. `GROK_HOME` is used when set, otherwise `~/.grok`.

## Desktop Widgets

Widgets require macOS 14 or later; Windows and Linux do not have them. Four widget types are available: quota overview, token trend, a key metric, and Codex Reset Updates. Each widget can select Codex, Claude, Grok, or a pair: Codex + Claude, Codex + Grok, or Grok + Claude. A widget already set to Codex + Grok keeps that pair. Tapping it opens the matching page.

The snapshot lives at `~/.quosparr/widget-snapshot.json` with `0600` permissions and contains only non-secret derived data: no email, account ID, token, auth JSON, or raw event text. The main app is the only writer; widgets only read it.

## Network Proxy

In **Control Panel → General → Network**, choose **Follow System**, HTTP, or SOCKS5; custom proxies support username and password. The setting covers app-initiated Codex, Claude, and Grok requests, CLI operations, public status and pricing feeds, and software updates. The system proxy is not changed, and browser sign-in pages and already-running external CLIs keep their own settings.

Changes apply only after **Save Proxy Settings**. **Test Connection** fetches the test site (default `https://github.com`, customizable) with the current draft, without sending account credentials or saving the draft. Proxy passwords are stored in the macOS Keychain, Windows Credential Manager, or Linux Secret Service. Invalid settings, unavailable credentials, or a failed custom proxy never fall back to a direct connection.

## Privacy

- Quosparr keeps its own data in `~/.quosparr` and writes no app cache into `~/.codex`. Only non-secret derived data is stored by default.
- Provider account credentials and the explicitly confirmed Desktop profiles described above are controlled exceptions; credentials are stored under `~/.quosparr/accounts/<id>/auth.json` (directory `0700`, file `0600`, written atomically; Windows uses an ACL that only the current user can access). The account index contains no tokens.
- Official `~/.codex/auth.json` is written only for an OAuth refresh or when you confirm an account switch. Refreshing a non-current account updates only its library copy.
- Claude credentials are read from the macOS Keychain through `/usr/bin/security`, or from `<config>/.credentials.json` on Windows and Linux. Managed copies live at `~/.quosparr/accounts/claude-<stable-id>/auth.json` and contain only `claudeAiOauth`. The index `claude-index.json` has no tokens. A switch keeps MCP tokens and replaces only `oauthAccount` in `.claude.json`. The current account is not refreshed by Quosparr.
- API account keys are stored only in the managed copy (`~/.quosparr/accounts/<id>/auth.json` or `claude-api-<id>/auth.json`); the indexes keep the Base URL, models, and cached balance. Balance and model requests go only to the site you entered, through the proxy settings. Edited `config.toml` and `settings.json` files are backed up to `~/.quosparr/backups`; the first write to `settings.json` sorts its keys.
- Grok credentials are read from `$GROK_HOME/auth.json`; managed copies live at `~/.quosparr/accounts/grok-<stable-id>/auth.json`. A Grok switch replaces only OAuth / compatible legacy scopes and keeps API-key and unknown scopes. It is guaranteed only for new sessions; running Grok processes are not terminated and may write the previous account back, so the app warns before switching.
- Session repair only touches `session_index.jsonl` and backs it up first. Session logs are moved to the system Trash only after you select and confirm them in Settings › Sessions; each file is checked again before it moves, nothing is deleted permanently, and official indexes and databases are never rewritten.
- Skill and MCP toggles, fixes and deletions run only after you confirm. Config files are backed up to `~/.quosparr/backups` before any write, deleted skills go to the restorable Quosparr skills trash first, and the MCP list never shows keys or the values of env and headers.
- Adding a skill lists the source first and reads only the skills' folders into memory before anything is written, so a large repository costs download time but not memory; it never runs their scripts; GitHub sources are fetched over HTTPS from GitHub only, through the proxy settings. Where each added skill came from (repository, ref, commit, folder in the repository, or the local path) is kept in `~/.quosparr/skill-sources.json` (`0600`), which holds no secrets.
- API-equivalent cost is computed from local session logs. Session contents are not uploaded.
- Tokens and API keys are never written to logs, self-check output, or issue templates.
- Codex Reset Updates and update checks fetch only public data; no account, token, or session content is sent.

## Data Sources

- **Codex quota / reset credits / official usage**: official ChatGPT / Codex backend APIs, using your local credentials. Official usage belongs to the current account and may lag or be revised.
- **Weekly quota estimate**: unofficial, extrapolated from the weekly percent and daily Credits (1000 Credits ≈ $40, version `credits-usd-2026-08-26`). Only derived Credits and percents are saved in `quota-estimate-history.json`.
- **Codex API-equivalent cost**: local session logs priced with the built-in table `openai-builtin-2026-09-27`; supported current rates come from models.dev with verified built-in rules preserved. Unknown models are not reported as exact costs.
- **Claude Code subscription usage**: an undocumented OAuth usage endpoint for the signed-in Claude Code subscription. It is not described as an official billing API. No browser cookies or Console API bill; Desktop profiles are managed only by the separate, explicitly confirmed feature above.
- **Claude API-equivalent cost**: local Claude Code session logs priced with `anthropic-builtin-2026-10-14`. Replies are counted once by message id. Cache writes are split into 5-minute and 1-hour rates, and Fast mode has its own rates. Unknown models are not reported as exact costs. Days use the local time zone.
- **Grok quota**: only the official CLI chat-proxy `/v1/billing?format=credits` endpoint. No browser cookies, no quota inferred from local logs, and no API billing mixed in.
- **Grok API-equivalent cost**: local `turn_completed` usage priced with `xai-builtin-2026-09-27`, covering input, cached, output, and long-context rates. `costUsdTicks` is subscription accounting and is not used as API-equivalent cost.
- **Codex Reset Updates**: the public feed at [didcodexreset.com/api/status.json](https://didcodexreset.com/api/status.json). Unofficial, advisory only, and may be delayed.
- In the chart, “Official stats (all devices)” belongs to the current account, while “Local logs (all sessions)” scans this machine and may span accounts. They are not a subset of each other and should not be subtracted; Codex daily values for both use UTC days.

## Development

The main app is written in Go with [mygo](https://github.com/egoist/mygo) native UI; the Go and mygo versions are pinned in `go.mod`. The macOS widget extension remains in Swift.

```bash
go test ./...
go vet ./...
go build ./...
```

Run against a temporary HOME and a separate preferences domain so your real accounts and settings are untouched:

```bash
QUOSPARR_TEST_HOME="$(mktemp -d)"
HOME="$QUOSPARR_TEST_HOME" QUOSPARR_PREFS_DOMAIN=com.quosparr.app.v1-test go run .
HOME="$QUOSPARR_TEST_HOME" QUOSPARR_PREFS_DOMAIN=com.quosparr.app.v1-test go run . --self-check
```

The self-check makes no network request and reads no credentials. It currently covers the version, platform, own-directory permissions, locks, and whether Codex auth and the session index exist; other diagnostics are listed as `not ported yet`.

Build a macOS development package (updater disabled; optionally `ARCH=arm64` or `ARCH=x86_64`):

```bash
V1_DEVELOPMENT_PACKAGE=1 bash Scripts/package-v1-app.sh
```

See [docs/v1-release.md](docs/v1-release.md) for Windows / Linux packaging and release checks, and [CONTRIBUTORS.md](CONTRIBUTORS.md) for contribution notes.

## Community

- [LinuxDO](https://linux.do/)

## License

This project follows the repository [LICENSE](LICENSE).
