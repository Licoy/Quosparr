[English](./README.md) · [简体中文](./README_ZH.md) · 繁體中文 · [한국어](./README_KO.md) · [日本語](./README_JA.md) · [Русский](./README_RU.md) · [Français](./README_FR.md)

<div align="center">

<img src="Resources/AppIcon.png" alt="Quosparr logo" width="128" height="128">

# Quosparr

<sub><i>名稱取自 <b>Quo</b>ta（額度）+ <b>Sparr</b>ow（麻雀）</i></sub>

一隻停在選單列裡的小麻雀，替你守著 Codex、Claude 與 Grok 的額度。

Quosparr 是適用於 macOS、Windows、Linux 的原生選單列 / 系統匣應用程式，顯示 Codex、Claude Code 與 Grok 訂閱額度、reset credits、API 等價成本與本機工作階段，三個供應商各自獨立管理帳號，並可集中管理三家的全域 Skill、MCP 服務與本機工作階段。

</div>

## 亮點

**Codex**

- 查看 5 小時、每週與附加額度視窗的剩餘額度，以及目前帳號、訂閱等級和到期時間。
- 查看 reset credits 數量、狀態與到期時間。
- 根據官方週用量百分比與每日 Credits 推算本週額度，並與前幾週比較，方便發現額度被調降。
- Codex 重設動態：查看今天 Codex 速率限制是否已重置，可單獨設定重新整理間隔（資料來自 [Did Codex Reset](https://didcodexreset.com)）。
- 服務商帳號：瀏覽器登入、匯入本機 `auth.json`、貼上 token / JSON（含 `/auth/session`）、匯入檔案或新增 API Key。確認後以原子方式寫入 `~/.codex/auth.json` 完成切換帳號。
- API 帳號：Base URL + API Key（Base URL 留空即官方 OpenAI API），可從站點取得模型清單選擇模型；切換時寫入 `auth.json` 與 `config.toml` 中的 `quosparr` 服務商（寫入前備份），new-api 類或 OpenAI 相容中轉站的餘額顯示在帳號清單、面板、狀態列、懸浮視窗與 Widget；已在 `config.toml` 選用中轉站的 Key 登入會自動帶上該站點與模型；編輯目前帳號時可替換 Key（留空保留），並選擇是否立即改寫設定檔，改寫後需重新啟動 Codex
- 查看最近的工作階段、專案與狀態，並修復本機工作階段索引。

**Claude**

- 查看 Claude Code 訂閱用量的 5 小時視窗、7 天視窗、按模型的週額度與額外用量。這個讀數不是公開的計費介面。
- 從 Claude Code 本機工作階段日誌統計 token 用量、API 等價成本（`anthropic-builtin-2026-10-14`）、最近工作階段與輸出速率。同一則回覆只計一次。未知模型不給精確費用。日誌沒有首 token 時間，該欄顯示「—」，速率按端到端耗時計算。
- 訂閱帳號可直接透過瀏覽器登入，無須先執行 CLI，也可匯入目前登入、貼上憑證或從檔案匯入；瀏覽器登入支援取消和貼上回呼，僅加入帳號庫，需明確切換才在 Claude Code 中使用；Quosparr 不重新整理目前帳號，登入過期時執行一次 `claude`；其他託管帳號只把重新整理結果寫入帳號庫複本
- macOS 上官方憑證留在鑰匙圈，經 `/usr/bin/security` 讀寫。Windows 與 Linux 使用 `<config>/.credentials.json`。切換只替換 `claudeAiOauth`，保留 MCP 權杖，並只更新 `.claude.json` 裡的 `oauthAccount`。正在執行的 Claude Code 不會被結束，可能寫回舊登入，切換前會提示。
- API 帳號：Base URL + API Key，可從站點取得模型清單並分別映射 Opus / Sonnet / Haiku / Fable 模型。切換時只把 `ANTHROPIC_BASE_URL`、`ANTHROPIC_AUTH_TOKEN` 與模型映射寫入 `settings.json`（寫入前備份），訂閱登入保持不變；切回訂閱帳號時移除這些鍵。中轉站餘額顯示在所有呈現 Claude 額度的位置。已寫在 `settings.json` 中的中轉設定會自動匯入。編輯目前帳號時可替換 Key（留空保留），並選擇是否立即改寫 `settings.json`，改寫後需重新啟動 Claude Code

- **Claude Desktop 訂閱帳號**：兩個帳號清單皆可綁定，首次須在 Desktop 手動登入所示帳號及組織後回到 Quosparr 明確確認；每次操作皆須先儲存工作並同意結束 Desktop，儲存或還原完整本機設定，包括登入、設定及本機狀態；完成或取消綁定皆還原先前的設定；連動切換預設關閉，目前 CLI 帳號也能單獨開啟已綁定 Desktop；CLI 與 Desktop 結果分別回報，啟動成功不代表已驗證帳號
- Desktop 設定檔及其本機原生加密登入資料私有保存在 `~/.quosparr/claude-desktop`，Windows 使用僅目前使用者可存取的 protected ACL，保留官方資料路徑，Quosparr 不解密且不納入帳號匯出；原設定單獨保留，不匯入其中帳號，不讀取瀏覽器或其他應用程式資料；設定檔與 Desktop 資料須在同一磁碟區，中斷後可主動還原；不支援 API 帳號，macOS、Windows、Linux 真機與 Desktop 版本相容驗收仍待完成

**Grok**

- 透過官方 CLI chat-proxy 計費介面，查看包含額度、Build / Imagine / Chat 拆分、週 / 月週期、預付餘額和按需使用量。
- 多個 OAuth / SuperGrok 帳號：隔離登入、匯入目前登入、貼上憑證、重新整理、別名、排序與明確切換帳號。
- 查看本機 Grok CLI 日誌中的最近工作階段。

**三個供應商通用**

- 設定中的「模型價格」位於「進階」之前並與其同組；預設每小時自動同步 models.dev 公開目錄，可關閉，可選預設或自訂 15 分鐘至 7 天頻率，修改立即儲存，關閉自動同步後仍可手動同步
- 模型價格列出美元 / 百萬 Token 的參考費率，不代表圖像、影片等全部模態費用；目錄參考價與實際採用價可能因已核實的上下文階梯、快取寫入時長、Fast 或地區計費規則不同，詳情顯示來源與版本，以下帶日期的內建表保留為離線基線；僅依固定 Codex / Claude / Grok tabs 顯示 OpenAI、Anthropic、xAI 三家原始供應商條目與對應內建價格，預設 Codex，不提供其他廠商入口，不顯示第三方代理或其他供應商的同名模型，models.dev 是彙整公開目錄而非供應商官方即時介面；tabs 右側「頻率設定」按鈕開啟原生彈窗，自動同步、頻率與詳細狀態共同適用於三家，修改立即儲存，關閉彈窗不取消同步工作
- API 等價成本依目前價格重新估算歷史本機用量；總額、每日、專案、模型與工作階段的部分成本均有標記，並完整列出缺少的費率或規則；缺價顯示 —，明確免費顯示 $0，訂閱額度與 reset credits 維持原有口徑
- 價格同步僅經已儲存的代理策略下載公開目錄，不上傳工作階段日誌；離線保留內建價格與最後成功儲存的目錄，同步失敗會明確顯示
- 常駐的 `Codex | Claude | Grok` 頁籤。macOS 設定中的「狀態欄元件」可預覽額度條、倒計時、電池（橫向或直向）、圓環進度（數值可置於環內）、進度文本五種樣式；選單列可跟隨主面板，或分別顯示三個供應商中的任意幾個，額度條、倒計時與電池還可將兩個供應商上下疊放在同一元件中。已經儲存的「Codex + Grok」仍是這一對。
- 可選的置頂桌面懸浮窗，最多顯示三個供應商的剩餘額度，提供膠囊、卡片、圓環、電池四種樣式；按一下開啟主面板，拖曳可移動，按右鍵查看更多選項，可調整不透明度、鎖定位置，並選擇是否顯示在全螢幕應用程式上方。
- 查看今天、本週期、上週期、本月或自訂範圍的 API 等價成本與 token 用量。
- 年度 token 用量圖表，支援熱力圖、折線圖和長條圖（每日 / 每週 / 累計）。
- Skills 頁面列出各供應商的全域 skill，顯示來源、是否被 CLI 載入以及與哪些 CLI 共用；可搜尋、檢視 `SKILL.md`、打開所在目錄、在 CLI 自己的設定中啟停（寫入前備份）、複製或移動到其他 CLI（同名時不覆寫）、移到 Quosparr 的 Skill 回收站，並提示失效連結、巢狀與重名等問題，多數問題可一鍵修復，也可多選後批次刪除或修復，執行前都會列出風險並請你確認；設定中的「Skill 管理」依供應商分頁，可依狀態與類型篩選，詳情頁顯示安裝來源、檔案、frontmatter 及 GitHub 儲存庫或首頁連結，回收站中的 skill 可復原或移到系統資源回收筒。
- 設定中的「工作階段管理」依供應商列出本機全部工作階段，可依標題、專案或 ID 搜尋，依專案與日期範圍篩選，詳情頁顯示模型、Token 明細、API 等價成本、相關檔案與對話預覽；選取的工作階段經確認後逐一移到系統資源回收筒，不會永久刪除，官方索引與資料庫保持不變，衍生統計隨後依剩餘日誌重新計算。
- 設定中的「MCP 管理」依供應商列出使用者層級的全域 MCP 服務（Codex 與 Grok 的 `config.toml`、Claude 的 `.claude.json`），只顯示脫敏後的命令、站點與結構摘要，不顯示 env 與 headers 的值，也不連線或啟動任何服務；可多選後刪除，刪除前先備份到 `~/.quosparr/backups` 並保留其他設定，重新啟動對應 CLI 後生效。Codex 與 Grok 的服務另有啟用開關，同樣先備份再寫入：Codex 寫入項目的 `enabled`，Grok 與其官方 CLI 一致，同時更新 `disabled_mcp_servers`；Claude Code 依專案記錄 MCP 啟用狀態，因此 Claude 服務不提供開關。專案層級、外掛與企業託管的 MCP 不會顯示。
- 支援淺色、深色、跟隨系統外觀，以及 English、简体中文、繁體中文、한국어、日本語、Русский、Français。
- 內建線上更新，macOS 14+ 提供桌面小工具。

## 截圖

<table>
  <tr>
    <th colspan="2">主面板</th>
    <th colspan="2">API 等價成本</th>
    <th colspan="2">訂閱額度推算</th>
  </tr>
  <tr>
    <td colspan="2" align="center" valign="top"><img src="docs/images/1.webp?v2" alt="Quosparr 主面板" width="240"></td>
    <td colspan="2" align="center" valign="top"><img src="docs/images/3.webp?v1" alt="Quosparr API 等價成本" width="240"></td>
    <td colspan="2" align="center" valign="top"><img src="docs/images/6.webp?v1" alt="Quosparr 訂閱額度推算" width="240"></td>
  </tr>
  <tr>
    <th colspan="3">服務商帳號</th>
    <th colspan="3">顯示設定</th>
  </tr>
  <tr>
    <td colspan="3" align="center" valign="top"><img src="docs/images/4.webp?v1" alt="Quosparr 服務商帳號" width="370"></td>
    <td colspan="3" align="center" valign="top"><img src="docs/images/5.webp?v1" alt="Quosparr 顯示設定" width="370"></td>
  </tr>
  <tr>
    <th colspan="6">Windows 設定與 Grok 面板</th>
  </tr>
  <tr>
    <td colspan="6" align="center" valign="top"><img src="docs/images/2.webp?v1" alt="Quosparr Windows 設定與 Grok 面板" width="780"></td>
  </tr>
</table>

截圖顯示的是 v1 在 macOS 與 Windows 上的介面。

## 安裝

請從 [GitHub Releases](https://github.com/Licoy/Quosparr/releases) 下載，並依系統與 CPU 架構選擇安裝包：

| 系統 | 安裝包 | 安裝方式 |
| --- | --- | --- |
| macOS 12+（Apple Silicon / Intel） | `Quosparr-macos-arm64.dmg` / `Quosparr-macos-x86_64.dmg`，或同名 `.zip` | 將 `Quosparr.app` 拖入 `Applications` |
| Windows 10 / 11（amd64 / arm64） | `Quosparr.Setup.<version>.<arch>.exe` | 以使用者層級安裝到 `%LOCALAPPDATA%\Programs\Quosparr`，不需要管理員權限 |
| Linux（amd64 / arm64） | `quosparr-<version>-linux-<arch>.tar.gz` 與 `install.sh`，或 `quosparr_<version>_<arch>.deb` | 在 tar.gz 所在目錄執行 `sh install.sh` 安裝到 `~/.local`，或用套件管理員安裝 deb |

每個安裝包都隨附對應架構的 SQLite 執行階段函式庫，首次啟動時不會連網下載任何內容。首次啟動時預設開啟開機自動啟動，可在 **控制台 → 通用 → 開機自動啟動** 中修改。

目前尚未提供 Homebrew 安裝，請先使用 DMG 或 ZIP 安裝。

之前使用過 CodexRunway？請參閱[升級指南](docs/upgrade-from-codexrunway_ZH_HANT.md)。

### macOS 安全性提示

macOS 版本目前為 ad-hoc signed，尚未經過公證。若 macOS 提示無法驗證開發者，請在 `Quosparr.app` 上按右鍵並選擇「打開」，或前往「系統設定 → 隱私權與安全性」點選「仍要打開」。

若提示應用程式已損壞，通常是下載時附加的隔離屬性所致。將應用程式移入 `Applications` 後執行：

```bash
xattr -dr com.apple.quarantine /Applications/Quosparr.app
```

### Linux 需求

Linux 需要 GTK3、AppIndicator、支援系統匣的桌面環境，以及可用的 Secret Service 工作階段（例如 GNOME Keyring 或 KWallet）以儲存代理憑證。deb 已宣告這些相依套件，並透過套件管理員更新。

## 使用前提

- 建議本機已安裝並使用 Codex。Codex 資料來自 `CODEX_HOME`，預設為 `~/.codex`，Windows 上預設為 `%USERPROFILE%\.codex`。
- 僅支援 `file` 憑證儲存方式（Codex `config.toml` 中未設定 `cli_auth_credentials_store` 時即為此方式）。`keyring`、`auto`、`ephemeral` 會顯示不支援，並阻止匯入與切換帳號。
- Claude 面板從 `CLAUDE_CONFIG_DIR`（未設定時為 `~/.claude`）讀取 Claude Code 設定，訂閱用量需要 Claude.ai 登入，API Key 或閘道不提供這份用量；瀏覽器登入無須安裝或先執行 CLI，在你明確切換前只寫帳號庫；已有 CLI 登入會自動加入，也可貼上憑證 JSON 或從檔案匯入；真實瀏覽器授權和切換仍需隔離帳號驗收
- 使用 Grok 面板前需要安裝[官方 Grok CLI](https://docs.x.ai/build/overview)：

  ```bash
  curl -fsSL https://x.ai/cli/install.sh | bash
  ```

- Grok 帳號與額度僅支援 OAuth / SuperGrok 及相容的 legacy session。可執行 `grok login --oauth`、在應用程式內登入、匯入目前登入，或貼上憑證 JSON。僅使用 API Key 的登入不會加入託管帳號。若已設定 `GROK_HOME` 則使用該目錄，否則使用 `~/.grok`。

## 桌面小工具

桌面小工具需要 macOS 14 或更新版本，Windows 與 Linux 不提供。共有四種小工具：額度總覽、token 趨勢、關鍵指標和 Codex 重設動態。每個小工具都可在「編輯小工具」中選擇 Codex、Claude、Grok，或一對組合：Codex + Claude、Codex + Grok、Grok + Claude。已經設成 Codex + Grok 的小工具保持這一對。點一下即可前往對應頁面。

快照位於 `~/.quosparr/widget-snapshot.json`，權限為 `0600`，只包含非機密的衍生資料，不含電子郵件、帳號 ID、token、認證 JSON 或事件原文。主程式是唯一的寫入者，小工具只讀取。

## 網路代理

在 **控制台 → 通用 → 網路** 中選擇 **跟隨系統**、HTTP 或 SOCKS5，自訂代理支援使用者名稱與密碼。此設定涵蓋應用程式發起的 Codex、Claude 與 Grok 請求、CLI 操作、公開的狀態與價格來源，以及軟體更新。不會修改系統代理，瀏覽器登入頁面與已在執行的外部 CLI 沿用各自的設定。

修改須在點選 **儲存代理設定** 後才會生效。**測試連線** 以目前的草稿存取測試站點（預設 `https://github.com`，可自訂），不會傳送帳號憑證，也不會儲存草稿。代理密碼分別儲存在 macOS 鑰匙圈、Windows 認證管理員或 Linux Secret Service 中。設定無效、憑證無法取得或自訂代理失敗時，絕不會自動改為直接連線。

## 隱私

- Quosparr 的自有資料存放在 `~/.quosparr`，不會在 `~/.codex` 中寫入應用程式快取。預設只儲存非機密的衍生資料。
- 服務商帳號憑證與上文明確確認的 Desktop 設定檔為受控例外，帳號憑證儲存在 `~/.quosparr/accounts/<id>/auth.json`（目錄 `0700`、檔案 `0600`，以原子方式寫入；Windows 使用僅限目前使用者存取的 ACL）。帳號索引不含 token。
- 官方 `~/.codex/auth.json` 只會在 OAuth 重新整理回寫，或你確認切換帳號時寫入。重新整理非目前帳號只會更新帳號庫中的副本。
- Claude 憑證在 macOS 上經 `/usr/bin/security` 讀取鑰匙圈，在 Windows 與 Linux 上讀取 `<config>/.credentials.json`。託管副本位於 `~/.quosparr/accounts/claude-<stable-id>/auth.json`，只含 `claudeAiOauth`。索引 `claude-index.json` 不含 token。切換保留 MCP 權杖，並只替換 `.claude.json` 的 `oauthAccount`。Quosparr 不重新整理目前帳號。
- API 帳號的 Key 只保存在託管副本（`~/.quosparr/accounts/<id>/auth.json` 或 `claude-api-<id>/auth.json`），索引只保存 Base URL、模型與餘額快取。餘額與模型請求只發往你填寫的站點，並遵循代理設定。被修改的 `config.toml` 與 `settings.json` 會先備份到 `~/.quosparr/backups`；首次寫入 `settings.json` 時會依鍵名重新排序
- Grok 憑證從 `$GROK_HOME/auth.json` 讀取，託管副本位於 `~/.quosparr/accounts/grok-<stable-id>/auth.json`。Grok 切換帳號只會取代 OAuth / 相容 legacy scope，並保留 API Key 與未知 scope。此操作僅保證新的工作階段使用新帳號；已在執行的 Grok 行程不會被終止，且可能寫回舊帳號，因此切換前應用程式會顯示警告。
- 工作階段修復只處理 `session_index.jsonl`，寫入前會先備份。工作階段日誌只有在你於「工作階段管理」中選取並確認後才會移到系統資源回收筒，移動前逐一複核檔案未變更，不會永久刪除，也不改寫官方索引或資料庫。
- Skill 與 MCP 的啟停、修復和刪除都在你確認後才執行：設定檔寫入前先備份到 `~/.quosparr/backups`，刪除的 skill 先進入可復原的 Quosparr Skill 回收站，MCP 清單不顯示金鑰以及 env、headers 的值。
- API 等價成本依本機工作階段日誌計算，不會上傳工作階段內容。
- token 與 API Key 絕不會寫入日誌、自我檢查輸出或 issue 範本。
- Codex 重設動態與更新檢查只讀取公開資料，不會傳送帳號、token 或工作階段內容。

## 資料來源

- **Codex 額度 / reset credits / 官方用量**：使用本機憑證請求 ChatGPT / Codex 官方後端介面。官方用量屬於目前帳號，可能延遲或被修訂。
- **週額度推算**：非官方，依週用量百分比與每日 Credits 外推（1000 Credits ≈ $40，版本 `credits-usd-2026-08-26`）。`quota-estimate-history.json` 只儲存衍生的 Credits 與占用率。
- **Codex API 等價成本**：依本機工作階段日誌，使用內建價格表 `openai-builtin-2026-09-27` 計算；支援的目前費率來自 models.dev，並保留已核實的內建規則。未知模型不會回報精確費用。
- **Claude Code 訂閱用量**：來自未公開的 OAuth usage 端點，只表示目前的 Claude Code 訂閱，不寫成官方計費介面。不讀取瀏覽器 Cookie 或 Console API 帳單，Desktop 設定檔僅由上述獨立且明確確認的功能管理。
- **Claude API 等價成本**：依 Claude Code 本機工作階段日誌與價格表 `anthropic-builtin-2026-10-14` 計算。同一則回覆按訊息 ID 只計一次。快取寫入分為 5 分鐘與 1 小時，Fast 檔單獨計價。未知模型不報告精確費用。按日統計使用本地時區。
- **Grok 額度**：僅使用官方 CLI chat-proxy 的 `/v1/billing?format=credits` 端點。不讀取瀏覽器 Cookie，不從本機日誌推算額度，也不混入 API 帳單。
- **Grok API 等價成本**：依本機 `turn_completed` 用量與價格表 `xai-builtin-2026-09-27` 計算，區分輸入、快取、輸出及長上下文費率。`costUsdTicks` 屬於訂閱額度記帳，不作為 API 等價成本。
- **Codex 重設動態**：來自公開來源 [didcodexreset.com/api/status.json](https://didcodexreset.com/api/status.json)。非官方，僅供參考，可能延遲。
- 圖表中的「官方統計（多端）」屬於目前帳號，「本機日誌（全部本機工作階段）」掃描本機，可能跨越多個帳號。兩者並非包含關係，不應相減；Codex 兩種每日數值皆以 UTC 日界計算。

## 開發

主程式以 Go 與 [mygo](https://github.com/egoist/mygo) 原生介面撰寫，Go 與 mygo 的版本以 `go.mod` 為準；macOS 小工具延伸功能仍使用 Swift。

```bash
go test ./...
go vet ./...
go build ./...
```

請使用暫時的 HOME 與獨立的偏好設定網域來執行，以免動到真實帳號與設定：

```bash
QUOSPARR_TEST_HOME="$(mktemp -d)"
HOME="$QUOSPARR_TEST_HOME" QUOSPARR_PREFS_DOMAIN=com.quosparr.app.v1-test go run .
HOME="$QUOSPARR_TEST_HOME" QUOSPARR_PREFS_DOMAIN=com.quosparr.app.v1-test go run . --self-check
```

自我檢查不會發出網路請求，也不讀取憑證。目前涵蓋版本、平台、自有目錄權限、鎖定狀態，以及 Codex auth 與工作階段索引是否存在；其餘診斷會標示為 `not ported yet`。

建置 macOS 開發包（停用更新器，可選用 `ARCH=arm64` 或 `ARCH=x86_64`）：

```bash
V1_DEVELOPMENT_PACKAGE=1 bash Scripts/package-v1-app.sh
```

Windows / Linux 打包與發佈檢查請見 [docs/v1-release.md](docs/v1-release.md)，貢獻說明請見 [CONTRIBUTORS.md](CONTRIBUTORS.md)。

## 社群支援

- [LinuxDO](https://linux.do/)

## 授權條款

本專案遵循儲存庫中的 [LICENSE](LICENSE)。
