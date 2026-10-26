[English](./README.md) · 简体中文 · [繁體中文](./README_ZH_HANT.md) · [한국어](./README_KO.md) · [日本語](./README_JA.md) · [Русский](./README_RU.md) · [Français](./README_FR.md)

<div align="center">

<img src="Resources/AppIcon.png" alt="Quosparr logo" width="128" height="128">

# Quosparr

<sub><i>名称取自 <b>Quo</b>ta（额度）+ <b>Sparr</b>ow（麻雀）</i></sub>

一只停在菜单栏里的小麻雀，替你守着 Codex、Claude 与 Grok 的额度。

Quosparr 是面向 macOS、Windows、Linux 的原生菜单栏 / 系统托盘应用，展示 Codex、Claude Code 与 Grok 订阅额度、reset credits、API 等价成本与本机会话，三个供应商各自独立管理账号，并可集中管理三家的全局 Skill、MCP 服务与本机会话。

</div>

## 亮点

**Codex**

- 查看 5 小时、每周与附加额度窗口的剩余额度，以及当前账号、订阅等级和到期时间
- 查看 reset credits 数量、状态与过期时间
- 根据官方周用量百分比与每日 Credits 推算本周额度，并与往周对比，便于发现额度缩水
- Codex 重置动态：查看今天 Codex 速率限制是否已重置，可单独设置刷新间隔（数据来自 [Did Codex Reset](https://didcodexreset.com)）
- 服务商账号：浏览器登录、导入本机 `auth.json`、粘贴 token / JSON（含 `/auth/session`）、导入文件或添加 API Key；确认后原子写入 `~/.codex/auth.json` 完成切号
- API 账号：Base URL + API Key（Base URL 留空即官方 OpenAI API），可从站点获取模型列表选择模型；切换时写入 `auth.json` 与 `config.toml` 中的 `quosparr` 服务商（写入前备份），new-api 类或 OpenAI 兼容中转站的余额显示在账号列表、面板、状态栏、悬浮窗与 Widget；已在 `config.toml` 选用中转站的 Key 登录会自动带上该站点与模型；编辑当前账号时可替换 Key（留空保留），并选择是否立即改写配置文件，改写后需重启 Codex
- 查看最近会话、项目与状态，并修复本机会话索引

**Claude**

- 查看 Claude Code 订阅用量的 5 小时窗口、7 天窗口、按模型的周额度和额外用量。这个读数不是公开的计费接口
- 从 Claude Code 本机会话日志统计 Token 用量、API 等价成本（`anthropic-builtin-2026-10-14`）、最近会话和输出速率。同一条回复只计一次。未知模型不给精确费用。日志没有首 token 时间，该列显示“—”，速率按端到端耗时计算
- 订阅账号可直接通过浏览器登录，无需先运行 CLI，也可导入当前登录、粘贴凭据或从文件导入；浏览器登录支持取消和粘贴回调，仅加入账号库，需显式切换才在 Claude Code 中使用；Quosparr 不刷新当前账号，登录过期时运行一次 `claude`；其他托管账号只把刷新结果写入账号库副本
- macOS 上官方凭据留在钥匙串，经 `/usr/bin/security` 读写。Windows 与 Linux 使用 `<config>/.credentials.json`。切号只替换 `claudeAiOauth`，保留 MCP 令牌，并只更新 `.claude.json` 里的 `oauthAccount`。正在运行的 Claude Code 不会被结束，可能写回旧登录，切换前会提示
- API 账号：Base URL + API Key，可从站点获取模型列表并分别映射 Opus / Sonnet / Haiku / Fable 模型。切换时只把 `ANTHROPIC_BASE_URL`、`ANTHROPIC_AUTH_TOKEN` 与模型映射写入 `settings.json`（写入前备份），订阅登录保持不变；切回订阅账号时移除这些键。中转站余额显示在所有展示 Claude 额度的位置。已写在 `settings.json` 中的中转配置会自动导入。编辑当前账号时可替换 Key（留空保留），并选择是否立即改写 `settings.json`，改写后需重启 Claude Code

- **Claude Desktop 订阅账号**：两个账号列表均可绑定，首次须在 Desktop 手动登录所示账号及组织后回到 Quosparr 明确确认；每次操作均须先保存任务并同意退出 Desktop，保存或恢复其完整本地配置，包括登录、设置及本地状态；完成或取消绑定均恢复之前的配置；联动切号默认关闭，当前 CLI 账号也能单独打开已绑定 Desktop；CLI 与 Desktop 结果分别报告，启动成功不代表已验证账号
- Desktop 配置档及其本机原生加密登录数据私有保存在 `~/.quosparr/claude-desktop`，Windows 使用仅当前用户可访问的 protected ACL，保留官方数据路径，Quosparr 不解密且不纳入账号导出；原配置单独保留，不导入其中账号，不读取浏览器或其他应用数据；配置档与 Desktop 数据须在同一卷，中断后可主动恢复；不支持 API 账号，macOS、Windows、Linux 真机与 Desktop 版本兼容验收仍待完成

**Grok**

- 通过官方 CLI chat-proxy 计费接口查看包含额度、Build / Imagine / Chat 拆分、周 / 月周期、预付余额和按需用量
- 多个 OAuth / SuperGrok 账号：隔离登录、导入当前登录、粘贴凭据、刷新、别名、排序与显式切号
- 查看本地 Grok CLI 日志中的最近会话

**三个供应商通用**

- 设置中的“模型价格”位于“高级”之前并与其同组；默认每小时自动同步 models.dev 公开目录，可关闭，可选预设或自定义 15 分钟至 7 天频率，修改立即保存，关闭自动同步后仍可手动同步
- 模型价格列出美元 / 百万 Token 的参考费率，不代表图像、视频等全部模态费用；目录参考价与实际采用价可能因已核实的上下文阶梯、缓存写入时长、Fast 或地区计费规则不同，详情显示来源与版本，以下带日期的内置表保留为离线基线；仅按固定 Codex / Claude / Grok tabs 展示 OpenAI、Anthropic、xAI 三家原始服务商条目与对应内置价格，默认 Codex，不提供其他厂商入口，不展示第三方代理或其他服务商的同名模型，models.dev 是聚合公开目录而非供应商官方实时接口；tabs 右侧“频率设置”按钮打开原生弹窗，自动同步、频率与详细状态共同适用于三家，修改立即保存，关闭弹窗不取消同步任务
- API 等价成本按当前价格重新估算历史本机用量；总额、每日、项目、模型与会话的部分成本均有标记，并完整列出缺少的费率或规则；缺价显示 —，明确免费显示 $0，订阅额度与 reset credits 维持原有口径
- 价格同步仅经已保存的代理策略下载公开目录，不上传会话日志；离线保留内置价格与最后成功保存的目录，同步失败会明确显示
- 常驻 `Codex | Claude | Grok` 标签切换。macOS 设置中的“状态栏组件”可预览额度条、倒计时、电池（横向或竖向）、圆环进度（数值可置于环内）、进度文本五种样式；菜单栏可跟随主面板，或分别显示三个供应商中的任意几个，额度条、倒计时与电池还可将两个供应商上下叠放在同一组件中。已经保存的 “Codex + Grok” 仍是这一对
- 可选的置顶桌面悬浮窗，最多显示三个供应商的剩余额度，提供胶囊、卡片、圆环、电池四种样式；单击打开主面板，拖动可移动，右键查看更多选项，可调不透明度、锁定位置，并选择是否显示在全屏应用上方
- 查看今天、本周期、上周期、本月或自定义范围的 API 等价成本与 Token 用量
- 年度 Token 用量图表，支持热力图、折线图和柱状图（按日 / 按周 / 累计）
- Skills 页面列出各供应商的全局 skill，显示来源、是否被 CLI 加载以及与哪些 CLI 共享；可搜索、查看 `SKILL.md`、打开所在目录、在 CLI 自己的配置中启停（写入前备份）、复制或移动到其他 CLI（同名时不覆盖）、移到 Quosparr 的 Skill 回收站，并提示失效链接、嵌套与重名等问题，多数问题可一键修复，也可多选后批量删除或修复，执行前都会列出风险并请你确认；设置中的“Skill 管理”按供应商分页，可按状态与类型筛选，详情页展示安装来源、文件、frontmatter 及 GitHub 仓库或主页链接，回收站中的 skill 可恢复或移到系统回收站
- 设置中的“会话管理”按供应商列出本机全部会话，可按标题、项目或 ID 搜索，按项目与日期范围筛选，详情页显示模型、Token 明细、API 等价成本、相关文件与对话预览；选中的会话确认后逐个移到系统回收站，不会永久删除，官方索引与数据库保持不变，派生统计随后按剩余日志重新计算
- 设置中的“MCP 管理”按供应商列出用户级全局 MCP 服务（Codex 与 Grok 的 `config.toml`、Claude 的 `.claude.json`），只显示脱敏后的命令、站点与结构摘要，不展示 env 与 headers 的值，也不连接或启动任何服务；可多选后删除，删除前先备份到 `~/.quosparr/backups` 并保留其他配置，重启对应 CLI 后生效。Codex 与 Grok 的服务另有启停开关，同样先备份再写入：Codex 写条目的 `enabled`，Grok 与其官方 CLI 一致，同时更新 `disabled_mcp_servers`；Claude Code 按项目记录 MCP 启停，因此 Claude 服务不提供开关。项目级、插件与企业托管的 MCP 不显示
- 支持浅色、深色、跟随系统外观，以及 English、简体中文、繁體中文、한국어、日本語、Русский、Français
- 内置在线更新，macOS 14+ 提供桌面组件

## 截图

<table>
  <tr>
    <th colspan="2">主面板</th>
    <th colspan="2">API 等价成本</th>
    <th colspan="2">订阅额度推算</th>
  </tr>
  <tr>
    <td colspan="2" align="center" valign="top"><img src="docs/images/1.webp?v2" alt="Quosparr 主面板" width="240"></td>
    <td colspan="2" align="center" valign="top"><img src="docs/images/3.webp?v1" alt="Quosparr API 等价成本" width="240"></td>
    <td colspan="2" align="center" valign="top"><img src="docs/images/6.webp?v1" alt="Quosparr 订阅额度推算" width="240"></td>
  </tr>
  <tr>
    <th colspan="3">服务商账号</th>
    <th colspan="3">显示设置</th>
  </tr>
  <tr>
    <td colspan="3" align="center" valign="top"><img src="docs/images/4.webp?v1" alt="Quosparr 服务商账号" width="370"></td>
    <td colspan="3" align="center" valign="top"><img src="docs/images/5.webp?v1" alt="Quosparr 显示设置" width="370"></td>
  </tr>
  <tr>
    <th colspan="6">Windows 设置与 Grok 面板</th>
  </tr>
  <tr>
    <td colspan="6" align="center" valign="top"><img src="docs/images/2.webp?v1" alt="Quosparr Windows 设置与 Grok 面板" width="780"></td>
  </tr>
</table>

截图展示的是 v1 在 macOS 与 Windows 上的界面

## 安装

请从 [GitHub Releases](https://github.com/Licoy/Quosparr/releases) 下载，并按系统和 CPU 架构选择安装包：

| 系统 | 安装包 | 安装方式 |
| --- | --- | --- |
| macOS 12+（Apple Silicon / Intel） | `Quosparr-macos-arm64.dmg` / `Quosparr-macos-x86_64.dmg`，或同名 `.zip` | 将 `Quosparr.app` 拖入 `Applications` |
| Windows 10 / 11（amd64 / arm64） | `Quosparr.Setup.<version>.<arch>.exe` | 用户级安装到 `%LOCALAPPDATA%\Programs\Quosparr`，不需要管理员权限 |
| Linux（amd64 / arm64） | `quosparr-<version>-linux-<arch>.tar.gz` 与 `install.sh`，或 `quosparr_<version>_<arch>.deb` | 在 tar.gz 同目录运行 `sh install.sh` 安装到 `~/.local`，或用包管理器安装 deb |

各安装包都随包携带对应架构的 SQLite 运行库，首次启动不会联网下载。首次启动时默认开启开机自启，可在 **控制台 → 通用 → 开机自启** 中修改

暂未提供 Homebrew 安装，目前请使用 DMG 或 ZIP 安装

之前使用过 CodexRunway？请参阅[升级指南](docs/upgrade-from-codexrunway_ZH.md)

### macOS 安全提示

macOS 包目前为 ad-hoc signed，未经公证。若提示“无法验证开发者”，请右键点击 `Quosparr.app` 选择“打开”，或在“系统设置 → 隐私与安全性”中点击“仍要打开”

若提示应用已损坏，通常是下载隔离属性导致。将应用放入 `Applications` 后执行：

```bash
xattr -dr com.apple.quarantine /Applications/Quosparr.app
```

### Linux 依赖

Linux 需要 GTK3、AppIndicator、支持托盘的桌面，以及可用的 Secret Service 会话（如 GNOME Keyring 或 KWallet）用于保存代理凭据。deb 已声明这些依赖，并通过包管理器更新

## 使用前提

- 建议本机已安装并使用 Codex。Codex 数据来自 `CODEX_HOME`，默认 `~/.codex`，Windows 默认 `%USERPROFILE%\.codex`
- 仅支持 `file` 凭据存储（Codex `config.toml` 中未设置 `cli_auth_credentials_store` 时即为该方式）；`keyring`、`auto`、`ephemeral` 会提示不支持，并阻止导入与切号
- Claude 面板从 `CLAUDE_CONFIG_DIR`（未设置时为 `~/.claude`）读取 Claude Code 配置，订阅用量需要 Claude.ai 登录，API Key 或网关不提供这份用量；浏览器登录无需安装或先运行 CLI，在你显式切换前只写账号库；已有 CLI 登录会自动加入，也可粘贴凭据 JSON 或从文件导入；真实浏览器授权和切号仍需隔离账号验收
- 使用 Grok 面板前需要安装 [官方 Grok CLI](https://docs.x.ai/build/overview)：

  ```bash
  curl -fsSL https://x.ai/cli/install.sh | bash
  ```

- Grok 账号与额度仅支持 OAuth / SuperGrok 及兼容的 legacy session。可执行 `grok login --oauth`、在应用内登录、导入当前登录或粘贴凭据 JSON；仅 API Key 的登录不会加入托管账号。设置了 `GROK_HOME` 时使用该目录，否则使用 `~/.grok`

## 桌面组件

桌面组件需要 macOS 14 或更高版本，Windows 与 Linux 不提供。共有四类组件：额度总览、Token 趋势、关键指标和 Codex 重置动态；每个组件都可在“编辑小组件”中选择 Codex、Claude、Grok，或一对组合：Codex + Claude、Codex + Grok、Grok + Claude。已经设成 Codex + Grok 的组件保持这一对。点击后跳转到对应页面

快照位于 `~/.quosparr/widget-snapshot.json`，权限为 `0600`，只包含非密钥派生数据，不含邮箱、账号 ID、token、认证 JSON 或事件原文；主程序是唯一写入者，组件只读

## 网络代理

在 **控制台 → 通用 → 网络** 中选择 **跟随系统**、HTTP 或 SOCKS5，自定义代理支持用户名和密码。设置覆盖应用发起的 Codex、Claude 与 Grok 请求、CLI 操作、公开状态与价格源以及软件更新；不修改系统代理，浏览器登录页和已经运行的外部 CLI 沿用各自设置

修改仅在点击 **保存代理设置** 后生效。**测试连接** 使用当前草稿访问测试站点（默认 `https://github.com`，可自定义），不发送账号凭据，也不保存草稿。代理密码分别保存在 macOS 钥匙串、Windows 凭据管理器和 Linux Secret Service 中。配置错误、凭据不可用或自定义代理失败时不会自动改为直连

## 隐私

- Quosparr 的自有数据保存在 `~/.quosparr`，不向 `~/.codex` 写入应用缓存，默认只保存非密钥的派生数据
- 服务商账号凭据与上文明确确认的 Desktop 配置档为受控例外，账号凭据保存在 `~/.quosparr/accounts/<id>/auth.json`（目录 `0700`、文件 `0600`，原子写入；Windows 使用仅当前用户可访问的 ACL），账号索引不包含 token
- 官方 `~/.codex/auth.json` 仅在 OAuth 刷新回写或你确认切号时写入；刷新非当前账号只更新账号库副本
- Claude 凭据在 macOS 上经 `/usr/bin/security` 读取钥匙串，在 Windows 与 Linux 上读取 `<config>/.credentials.json`。托管副本位于 `~/.quosparr/accounts/claude-<stable-id>/auth.json`，只含 `claudeAiOauth`。索引 `claude-index.json` 不含 token。切号保留 MCP 令牌，并只替换 `.claude.json` 的 `oauthAccount`。Quosparr 不刷新当前账号
- API 账号的 Key 只保存在托管副本（`~/.quosparr/accounts/<id>/auth.json` 或 `claude-api-<id>/auth.json`），索引只保存 Base URL、模型与余额缓存。余额与模型请求只发往你填写的站点，并遵循代理设置。被修改的 `config.toml` 与 `settings.json` 会先备份到 `~/.quosparr/backups`；首次写入 `settings.json` 时会按键名重新排序
- Grok 凭据从 `$GROK_HOME/auth.json` 读取，托管副本位于 `~/.quosparr/accounts/grok-<stable-id>/auth.json`。Grok 切号只替换 OAuth / 兼容 legacy scope，保留 API Key 与未知 scope，且仅保证新会话使用新账号；已运行的 Grok 进程不会被终止，可能写回旧账号，因此切换前会给出提示
- 会话修复只处理 `session_index.jsonl`，写入前先备份。会话日志只有在你于“会话管理”中选中并确认后才会移到系统回收站，移动前逐个复核文件未变化，不会永久删除，也不改写官方索引或数据库
- Skill 与 MCP 的启停、修复和删除都在你确认后才执行：配置文件写入前先备份到 `~/.quosparr/backups`，删除的 skill 先进入可恢复的 Quosparr Skill 回收站，MCP 列表不显示密钥以及 env、headers 的值
- API 等价成本基于本机会话日志计算，不上传会话内容
- token 与 API Key 不会写入日志、自检输出或 issue 模板
- Codex 重置动态与更新检查只读取公开数据，不发送账号、token 或会话内容

## 数据来源

- **Codex 额度 / reset credits / 官方用量**：使用本机凭据请求 ChatGPT / Codex 官方后端接口；官方用量属于当前账号，可能延迟或修订
- **周额度推算**：非官方，根据周用量百分比与每日 Credits 推算（1000 Credits ≈ 40 美元，版本 `credits-usd-2026-08-26`），`quota-estimate-history.json` 只保存派生的 Credits 与占用率
- **Codex API 等价成本**：基于本机会话日志，使用内置价格表 `openai-builtin-2026-09-27`，支持的当前费率来自 models.dev，并保留已核实的内置规则；未知模型不报告精确费用
- **Claude Code 订阅用量**：来自未公开的 OAuth usage 端点，只表示当前 Claude Code 订阅，不写成官方计费接口。不读取浏览器 Cookie 或 Console API 账单，Desktop 配置档仅由上述独立且明确确认的功能管理
- **Claude API 等价成本**：基于 Claude Code 本机会话日志和价格表 `anthropic-builtin-2026-10-14`。同一条回复按消息 ID 只计一次。缓存写入分为 5 分钟和 1 小时，Fast 档单独计价。未知模型不报告精确费用。按日统计使用本地时区
- **Grok 额度**：只使用官方 CLI chat-proxy `/v1/billing?format=credits` 接口；不读取浏览器 Cookie、不从本地日志伪造额度，也不混入 API 账单
- **Grok API 等价成本**：基于本地 `turn_completed` 用量和价格表 `xai-builtin-2026-09-27`，区分输入、缓存、输出及长上下文价格；`costUsdTicks` 属于订阅额度记账，不作为 API 等价成本
- **Codex 重置动态**：来自公开接口 [didcodexreset.com/api/status.json](https://didcodexreset.com/api/status.json)，非官方，仅供参考，可能延迟
- 图表中的“官方统计（多端）”属于当前账号，“本机日志（全部本机会话）”扫描本机且可能跨账号；两者不是包含关系，也不能相减，Codex 两种按日统计均按 UTC 日界比较

## 开发

主程序使用 Go 与 [mygo](https://github.com/egoist/mygo) 原生界面编写，Go 与 mygo 版本以 `go.mod` 为准；macOS 桌面组件扩展仍使用 Swift

```bash
go test ./...
go vet ./...
go build ./...
```

开发运行请使用临时 HOME 和独立偏好域，避免触碰真实账号与设置：

```bash
QUOSPARR_TEST_HOME="$(mktemp -d)"
HOME="$QUOSPARR_TEST_HOME" QUOSPARR_PREFS_DOMAIN=com.quosparr.app.v1-test go run .
HOME="$QUOSPARR_TEST_HOME" QUOSPARR_PREFS_DOMAIN=com.quosparr.app.v1-test go run . --self-check
```

自检不请求网络、不读取凭据，目前覆盖版本、平台、自有目录权限、锁状态，以及 Codex auth 与会话索引是否存在；其余诊断会标记为 `not ported yet`

构建 macOS 开发包（禁用更新器，可选 `ARCH=arm64` 或 `ARCH=x86_64`）：

```bash
V1_DEVELOPMENT_PACKAGE=1 bash Scripts/package-v1-app.sh
```

Windows / Linux 打包与发布校验见 [docs/v1-release.md](docs/v1-release.md)，贡献说明见 [CONTRIBUTORS.md](CONTRIBUTORS.md)

## 社区支持

- [LinuxDO](https://linux.do/)

## 许可证

本项目遵循仓库中的 [LICENSE](LICENSE)
