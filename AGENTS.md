# AGENTS.md

## 项目定位

Quosparr 是 macOS、Windows、Linux 原生菜单栏 / 系统托盘应用，用 Go + mygo/ui 实现，读取本机 Codex / Grok / Claude Code 配置和会话数据，展示配额、reset credits、API 等价成本，并修复本机会话索引。

v1 已正式发布（`v1.0.0`、`v1.0.1`）；macOS 仅保留 Swift WidgetKit 扩展（`Sources/QuosparrWidget`）与系统桥接（`Sources/QuosparrWidgetBridge`）；Swift 主应用、`Package.swift`、`Tests/` 和应急回滚链已移除，不再恢复；后续版本公开发布前仍须完成隔离安装升级验收

## 技术边界

- macOS 最低版本为 12.0，Widget 要求 macOS 14+。
- 默认不引入第三方依赖；标准库和系统框架能解决的问题不要加包。
- Widget 与桥接使用 Swift 6、WidgetKit、SwiftUI、Foundation；Widget 工程由 `WidgetExtension/project.yml` 经 xcodegen 生成，不另建 Xcode project，除非用户明确要求。
- 状态栏应用必须保持 `LSUIElement=true`，不要显示 Dock 图标。
- Go 版本和依赖版本以 `go.mod` 为准，当前固定 Go 1.27.1、mygo v0.6.2；不将框架升级与迁移改动混在一起
- mygo/ui 只负责原生界面与框架服务，不引入 WebView 前端；业务核心不能依赖 UI
- purego 仅用于必要的平台 API、SQLite 和 Swift 桥接边界，按平台拆分并检查 ABI、资源生命周期与错误传播；不用它增加业务抽象
- macOS Go 主应用使用 mygo 更新器，不嵌入 Sparkle 运行库；保留 `SUPublicEDKey`，已安装的同身份 Quosparr Swift 版本通过 `quosparr-appcast-<arch>.xml` 升级到 Go 版本，该 appcast 由 `go run ./Scripts/release-assets sparkle-appcast` 直接生成并以 Ed25519 签名 ZIP，不依赖 Sparkle 工具或 Swift 构建；旧身份 appcast 仅提供手动下载链接，不做跨 bundle ID 自动安装
- 安装包必须随包携带相应架构的 SQLite 运行库，首次运行不得联网下载；Widget 与桥接嵌入后从内到外签名，最终签名之后才生成发布归档

## 隐私与安全

- 不要在日志、测试输出、README、issue 模板中输出 access token、refresh token、API key。
- Quosparr 自有目录统一使用 `~/.quosparr`，默认只保存非密钥的派生数据，不将应用缓存写入 `~/.codex`。
- macOS 偏好是现有系统 UserDefaults 的兼容例外，写入 `com.quosparr.app`，旧域 `com.github.codex-runway` 和 `dev.licoy.codex-runway` 仅用于一次性导入；Windows / Linux 偏好保存在自有目录，代理密码使用各平台安全存储，不写普通设置
- 多账号凭据与下述明确确认的 Claude Desktop 完整配置档是受控例外，多账号凭据：仅允许存放在 `~/.quosparr/accounts/<id>/auth.json`（目录 `0700`、文件 `0600`，原子写入）。账号索引 `index.json` 不得包含 token。
- Windows 凭据与相应目录、临时写入文件使用仅当前用户可访问的 protected ACL，并关闭权限继承，不能以 chmod 代替实际权限检查
- 读取 `~/.codex/auth.json` 时只使用必要字段。
- 修改官方 `~/.codex/auth.json` 仅限：OAuth refresh 回写、用户主动切号时的原子写入，以及用户编辑当前 API 账号并在询问中选择“保存并应用”时的同一事务写入。
- Codex API 账号（Base URL + API Key）的 Key 与其他 API Key 一样存于账号库托管副本，Base URL、模型与余额缓存只在 `index.json` 行内，不含 Key；用户主动切到中转账号时同一事务写官方 `auth.json`（`auth_mode=apikey`）与 `config.toml` 的 `model_provider = "quosparr"` 和 `[model_providers.quosparr]`，切走时移除该指针与 Quosparr 写入的模型，失败整体回滚；`config.toml` 只做逐行编辑并保留其他内容，写入前备份到 `~/.quosparr/backups`（目录私有、文件 `0600`，保留最近 10 份）；不改 shell rc 或环境变量；官方登录为 API Key 且 `config.toml` 的 `model_provider` 选中带 `base_url` 的服务商时，同步只读该文件，把站点与顶层模型补到尚无站点的对应账号行；从这类账号切走时，若选中的服务商指向同一站点，只移除 `model_provider` 行，保留用户的服务商表；编辑 API 账号可替换 Key（留空保留），行 ID 不变；编辑当前账号且 Key、站点或模型有变时先询问“仅保存 / 保存并应用”，仅保存只改账号库，应用后提示重启 Codex
- 中转余额与模型列表只请求用户填写的站点，经应用代理策略、不直连回退；Key 只放在请求头，错误、日志与缓存结果中不得出现 Key 或响应正文
- 刷新非当前托管账号 token 时只写账号库副本，不得写官方 `auth.json`；刷新当前账号时同步官方 auth 与账号库副本。
- 会话日志在普通扫描、详情读取与索引修复中保持只读；会话修复只写已备份的 `session_index.jsonl`。唯一删除例外是用户在“会话管理”中明确选择并确认的 Codex、Claude Code 或 Grok 会话：只将扫描时归属该会话且删除前复核可信根、路径、非符号链接、类型、文件身份、大小与修改时间均未变化的文件逐项移到系统回收站，不永久删除，不移走项目目录，不扩展枚举同目录文件，逐项报告成功、失败与部分完成。
- 删除操作不直接改写官方 `session_index.jsonl`、全局状态文件或数据库；近期会话与派生统计按现有刷新流程从仍存在的日志重新计算，索引中的孤儿行仅由用户另行确认的会话修复在备份后处理。
- 读取官方 Claude 凭据时只取 `claudeAiOauth` 的必要字段；`mcpOAuth` 不解析内容，只在用户主动切号时原样保留。
- `.claude.json` 的账号流程只读取 `oauthAccount`，切号时只替换这一个键；唯一额外读写例外是用户主动打开 MCP 管理或确认删除时处理顶层用户级 `mcpServers`，不处理 `projects` 下的本地 MCP、项目 `.mcp.json`、插件或托管配置。
- 写官方 Claude 凭据只有用户主动切号。当前账号的 token 不由 Quosparr 刷新。
- Claude API 账号的 Key 只存于 `~/.quosparr/accounts/claude-api-<id>/auth.json`（`{"apiKey": "..."}`，权限同上），`claude-index.json` 只保存 Base URL、Key 哈希提示、模型映射、已应用的五项模型值与对应登录身份的完整 SHA-256 摘要及余额缓存，模型所有权记录不含密钥，仅保存编辑时保留该记录，应用成功后与账号索引一起更新，切走时只清理仍匹配已应用值的模型，显式应用非空默认模型时移除 settings 内的 `env.ANTHROPIC_MODEL` 覆盖，空默认模型与切走操作保留用户手写的该覆盖；用户主动切到 API 账号时只改 `<config>/settings.json` 的 `env.ANTHROPIC_BASE_URL`、`env.ANTHROPIC_AUTH_TOKEN` 及可选模型映射键，写入前备份到 `~/.quosparr/backups`，切回订阅账号时只移除 Quosparr 写入且值仍匹配的键；钥匙串、`.credentials.json` 与 `.claude.json` 不因 API 账号改动；`settings.json` 首次写入会按键名重排（内容不变）；同步时只读 `settings.json`，其中已有而账号库没有的 `ANTHROPIC_BASE_URL` + `ANTHROPIC_AUTH_TOKEN`（或 `ANTHROPIC_API_KEY`）自动导入为 API 账号并设为当前，移除时 `ANTHROPIC_API_KEY` 同样按值匹配；编辑当前 API 账号且 Key、站点或模型有变时同样先询问，仅保存不改 `settings.json`，应用后提示重启 Claude Code；Base URL 末尾的 `/v1` 会去掉（Claude Code 自行拼接 `/v1/messages`），旧版本存下的带 `/v1` 行读取时规范化，`settings.json` 仍是旧写法时按同一站点识别，编辑该账号时可选择改写
- Claude API 账号可选启用 1M 上下文（默认关闭），仅对已填写的默认、Opus 和 Sonnet 模型追加 `[1m]`，不重复追加已有后缀，不改 Haiku / Fable；模型与中转须支持 1M，关闭恢复所填模型且保留手写后缀，修改当前账号时沿用“仅保存 / 保存并应用”确认
- Claude 托管副本在 `~/.quosparr/accounts/claude-<stable-id>/auth.json`（目录 `0700`、文件 `0600`、原子写入；Windows 使用 protected ACL 并关闭继承），只保存 `{"claudeAiOauth": {...}}`。索引 `claude-index.json` 不得包含 token。
- Claude Desktop 订阅联动是完整本地配置的受控例外，只有用户明确确认退出 Desktop 并保存或恢复完整本地配置（登录、设置、本地状态）后执行；始终保持该安装的官方数据路径，不使用自定义 user-data 环境变量或参数，不绕过签名门禁；首次绑定由用户在 Desktop 手动登录所示账号及组织后明确确认，不宣称自动验证登录，完成或取消均恢复之前配置，联动默认关闭且 API 账号不提供；当前 CLI 账号可独立打开已绑定 Desktop；配置档保存在 `~/.quosparr/claude-desktop` 的私有目录，Windows 使用仅当前用户可访问并关闭继承的 protected ACL，包括 Desktop 自行写入的本机原生加密登录和本地状态，Quosparr 不解密、不读取浏览器或其他应用数据、不将其纳入账号导出；原默认配置单独保留，不导入为托管账号；只采用同卷目录重命名，跨卷明确失败，外部目录身份变化拒绝覆盖，事务中断只能由用户主动恢复；CLI 与 Desktop 结果分别报告，回滚失败明确提供恢复入口，不把进程启动当身份验证；这不是官方稳定切号 API，Desktop 版本兼容与三端真机用户验收尚未执行，本次仅允许假 Host、临时目录、Tester 与交叉编译，禁止本机真实 Desktop 实测。
- Claude 会话扫描只读 `projects/**`，不读取 `last-prompt` 等独立缓存；仅在用户打开详情时读取所选会话日志中的对话原文，仅上述用户主动确认的会话删除例外可把已归属同一会话的主日志与配套日志逐项移到系统回收站。
- Skill 管理只扫描全局级别（不含项目级与旧版 Claude `commands/*.md`），按各 CLI 自身的发现规则读取 SKILL.md 的 frontmatter，正文只在用户展开时读取前 256 KB，不跟随指向凭据文件的链接；跨供应商复制到目标 CLI 的原生用户 skills 目录并尊重自定义主目录，同名目标拒绝覆盖，只有可独立删除的用户 skill 能移动，完整复制后才将原件移入自有回收站，归档失败须明确报告部分完成；删除仅限用户 skills 目录，删除前重新校验路径、类型与所属根目录，先移入 Quosparr 自有回收站 `~/.quosparr/skills-trash/<id>/`（目录私有，`meta.json` 为 `0600`，只含非密钥元数据与 `item/` 原件），可在设置中原位恢复（恢复前再次校验原父目录与根目录、不覆盖已有内容），彻底删除与清空经系统回收站移除，跨卷无法移入时直接退到系统回收站并单独提示，不得直接删除或递归删除，符号链接只移除链接本身；诊断中的一键修复须先展示改动与风险并经用户确认：嵌套的 Claude 用户 skill 以顶层外层目录为单位修复：仅当外层目录中扫描到的 skill 全部是未加载的嵌套 skill、互不包含、目录名不重复、所在位置扫描未截断，且各自顶层同名位置空闲（或就是外层本身）时，先逐个完整复制到用户 skills 顶层，再把外层目录（或仅其链接）移入自有回收站，失败时撤回已放置的副本、恢复外层并清理副本；重名时被覆盖的副本可移入回收站，或仅 Codex 按路径停用（Claude、Grok 的开关按名称，不提供）；缺少描述的用户 skill 只在 SKILL.md frontmatter 中补写 `description`，写入前复核文件身份与修改时间，备份为 `~/.quosparr/backups` 下的 `skill-md-`（保留最近 10 份），不改正文与其他字段；失效链接的修复即移除链接本身；列表可多选后批量删除或批量修复，批量操作同样先汇总改动与风险并经一次确认，逐项执行并报告成功与失败数，批量修复对每个 skill 只采用其首选修复，补写描述时使用建议文字；来源识别只读 `~/.agents/.skill-lock.json`、skill 所在 git 仓库的配置、插件清单与 frontmatter，仓库 URL 去除凭据后才展示；系统、bundled、管理员、插件与 claude.ai 同步目录只读；启停只在用户主动操作时写各家官方开关：Codex `config.toml` 的 `[[skills.config]]`（按 SKILL.md 路径）、Claude `settings.json` 的 `skillOverrides`、Grok `config.toml` 的 `[skills].disabled`，写入前分别备份为 `~/.quosparr/backups` 下的 `codex-config-`、`claude-settings-`、`grok-config-`（各保留最近 10 份），只做逐项编辑并保留其他内容
- MCP 管理只读取三家原生用户级配置：Codex 与 Grok 的 `config.toml` 顶层 `mcp_servers`、Claude 用户状态文件顶层 `mcpServers`；尊重 `CODEX_HOME`、`GROK_HOME`、`CLAUDE_CONFIG_DIR`，自定义 Claude 配置目录不得回退读取默认账号的 `.claude.json`。不扫描、删除或启停项目级、插件、兼容导入或管理员托管 MCP，不连接 MCP、不执行命令（含官方 `mcp enable/disable`）、不探测网络；列表与错误仅展示脱敏后的命令、站点和结构摘要，不展示 env、headers 的值、URL userinfo/query/fragment 或响应正文。删除只在用户批量选择并确认后进行，提交前复核文件身份与扫描指纹，一次原子编辑同一配置并备份到 `~/.quosparr/backups`，与对应供应商的账号和 Skill 配置写入共用 Store 锁；校验或备份失败时整批不写入，写入后复核失败明确报告。启停只在用户主动拨动开关时写入单个条目，复用同一套复核、备份与锁：Codex 只改条目表内的 `enabled`（停用时缺失则插入，启用时缺失不写），Grok 按官方 CLI 同时维护条目 `enabled` 与顶层 `disabled_mcp_servers`（名单置空即移除该键，名单中或 `enabled = false` 即视为停用），只替换布尔值并保留注释与其他内容；行内表、点号键、非布尔 `enabled` 或无法改写的名单视为不可切换并说明原因；Claude 的启停按项目记录，不提供开关；删除 Grok 条目时一并移除名单中的同名项。
- 调用 `/usr/bin/security` 使用绝对路径。密钥只经 stdin 传递，超过上限时报错，不退回 argv。stderr 进入日志前只保留退出码和错误类别。

## 代码约定

- Go 入口为 `main.go`，原生 UI 与系统协调放在 `internal/app`，后台协调放在 `internal/runway`，存储 / 平台 / 网络 / 本地化边界分别放在 `internal/store`、`internal/platform`、`internal/netx`、`internal/l10n`
- Go 业务模块放在 `internal/codex`、`internal/codexapi`、`internal/grok`、`internal/claude`、`internal/usage`、`internal/sessions`、`internal/alerts`、`internal/estimate`、`internal/export`、`internal/resettoday`、`internal/pricing`（models.dev 公开 Token 目录、缓存、同步状态和不可变快照，实际计价由各供应商包保留已核实规则后选择）、`internal/skills`（全局 skill 扫描、来源识别、目录详情与自有回收站，启停写入在各供应商包）、`internal/mcp`（用户级 MCP 配置解析、脱敏摘要、原子删除与启停，供应商包负责路径和共用锁）；UI 经业务 API 读取结果，不直接解析 JSON 或扫描文件
- 用户可见文案通过 `internal/l10n`（手工维护的 `tables.go`）与现有附加文案入口，新增 key 必须补齐英文、简体中文、繁体中文、韩文、日文、俄文、法文
- Widget 使用自带的 `Sources/QuosparrWidget/RunwayWidgetL10n.swift` 与快照类型副本，修改共享 key 或快照 schema 时与 Go 侧保持同步，同样遵守七语言完整性
- 函数保持短小，优先用简单结构体和函数，不做多 provider 抽象。
- 错误信息可以面向用户，但不要包含本地密钥或完整认证 JSON。
- 新行为在对应 `internal/*` 包补必要测试，`testdata/swift_*` 等 Swift 版本冻结 fixture 只读保留、不再重新生成，不能以交叉编译替代 Windows ACL、系统权限或真机安装验收
- 保留 `QUOSPARR_DEV_*` 环境钩子；手动开发运行使用临时 HOME 和 `QUOSPARR_PREFS_DOMAIN=<测试域>`，不触碰开发机真实账号、UserDefaults、Keychain、登录项或生产 feed，结束后退出自行启动的进程

## 常用命令

```bash
go test ./...
go vet ./...
go build ./...
go test -race -count=1 ./internal/...
go run . --self-check
bash Scripts/run-dev-app.sh
V1_DEVELOPMENT_PACKAGE=1 bash Scripts/package-v1-app.sh
```

`go run . --self-check` 当前只检查基础目录、锁与文件存在性，并明确列出尚未移植的诊断；开发打包必须显式设置 `V1_DEVELOPMENT_PACKAGE=1`，生产默认 `0` 并要求发布环境中的更新签名私钥；Windows / Linux 交叉构建及完整发布校验见 `docs/v1-release.md`

## 文档规则

- README 和源码不要包含外部参考应用名称、链接或代码片段。
- 用户可见文档应说明隐私边界、ad-hoc signed app、开发命令和功能状态。
- 价格表必须带版本日期；未知模型不要估算为精确费用。
- 根目录 `README*.md` 已按 v1 Quosparr 重写：语言切换位于左上角首行，名称、小号斜体的名称由来概述（加粗 **Quo**ta + **Sparr**ow 取字部分）与简介水平居中，不单设名称由来章节，不嵌入视频；安装部分按全新安装说明，旧版升级说明放在 `docs/upgrade-from-codexrunway*.md` 七种语言并由首页链接；七种语言保持相同结构与事实，修改任一语言时同步其余六种；`README_EN.md` 仅作跳转
- 新版本的资产和清单准备完成不代表已公开发布或已通过真机验收
- 官网 `quosparr.com` 源码在 `website/`，由 `go run ./website/gen` 预渲染七种语言静态页，`.github/workflows/website.yml` 部署到 GitHub Pages；文案在 `website/locales/*.json`，七种语言字段与列表长度必须一致，图标只用 `website/icons/` 中的 Remix Icon SVG，下载链接取自最新正式 Release
- 隔离验收按 `docs/v1-acceptance-checklist.md` 与 `docs/v1-isolated-update-proxy.md` 执行，未修改且未重签的历史应用须保留真实 Sparkle 归档验签；不得改生产源或关闭 TLS 校验

## v1 分支说明（从 README 迁入）

以下集中保留原 v1 README 中的平台、安装、更新、迁移、开发与隐私说明；README 是面向用户的精简版本，细节与验收边界以本节和 `docs/v1-*.md` 为准

### 产品与功能状态

Quosparr v1 面向 macOS、Windows、Linux，主程序使用 Go + mygo/ui 原生菜单栏 / 系统托盘，macOS 保留 Swift WidgetKit 扩展；提供 Codex、Grok 与 Claude Code 订阅额度、重置动态、reset credits、API 等价成本、本机会话和各供应商独立的多账号管理

- v1 已正式发布（`v1.0.0`、`v1.0.1`），现有截图展示 v1 在 macOS 与 Windows 上的界面
- 首次无偏好配置的启动显示设置并保存默认偏好，已有配置或从旧版迁移的用户普通启动时仅驻留菜单栏；主动点击设置或显式传入 `--settings` 才再次打开设置，日常 Dev 重启不附加该参数
- Codex 支持配额窗口、订阅到期、reset credits、周额度推算与历史对比，Grok 展示官方 CLI chat-proxy 额度及 Build / Imagine / Chat 拆分、周期、预付余额和按需使用情况，Claude 展示 Claude Code 订阅用量的 5 小时、7 天、按模型周额度和额外用量，以及与 Codex 共用展示的重置次数
- Codex 与 Claude Code 另支持 API 账号（Base URL + API Key，可从站点获取模型列表；Claude 可分别映射 Opus / Sonnet / Haiku / Fable 模型），在多账号中添加、编辑与切换，并在账号列表、主面板、状态栏、悬浮窗与 Widget 显示中转站余额（new-api 类令牌额度或 OpenAI 兼容账单接口，站点不提供时明确说明，不估算）；Codex 无 Base URL 时为官方 OpenAI API Key
- 三个供应商均保留账号管理、本地 Token 用量图表、API 等价成本与最近会话，图表支持热力图、折线图和柱状图；主面板的“缓存命中分析”默认开启，独立读取本机会话日志，按今日、当前周期、上个周期、本月或自定义日期展示整体、模型与会话的缓存输入命中率，缺失缓存字段的记录不进入分母并单独提示；超长或损坏的日志行按行首类型识别，确定不含用量的直接跳过，可能含用量的跳过并提示统计可能不完整（只计入范围开始后仍有修改的日志），不因单个文件让模块整体失败，无效 Token 计数仍报错；Codex 另有会话索引修复与公开重置动态。主面板另有“Skills”模块（偏好键 `showsSkills` 仅 Go 版写入，默认开启时省略），列出三家全局 skill 的来源、状态与共享情况，可搜索、查看 SKILL.md、打开目录、启停与移到回收站，并诊断失效链接、嵌套与重名等问题，可按问题一键修复或多选后批量删除、修复（均先说明风险再确认执行）；设置窗口在“通知提醒”之后单独一组提供“Skill 管理”，按 Codex / Claude / Grok 分页，可按状态、位置、共享 / 链接 / GitHub / 问题 / 可删除 / 可启停筛选与排序，详情页展示安装来源（skills CLI 锁文件、git、插件市场或 frontmatter）、仓库与主页链接、文件列表、frontmatter 与诊断，末页为可恢复的 Skill 回收站。macOS 状态栏在设置“外观与菜单”之后单独一页“状态栏组件”，含预览、额度条 / 倒计时 / 电池 / 圆环 / 进度文本五种样式、服务商排列与显示选项：三个供应商可分别显示为独立组件，额度条、倒计时与电池可将两个供应商上下叠放在同一组件，圆环与进度文本始终逐个显示，电池分横向 / 竖向两种绘制，圆环可选“数值置于圆环内”（默认关闭，每个周期单独一环、环内数值不带 %，余额仍显示在环旁）；偏好对象 `statusBarLayout` 仅 Go 版写入、默认省略，`statusBarProviders` 同步保存前两个供应商供旧版读取，额度周期未选择时电池沿用旧 `statusBarBatteryScope`，选择后回写该键；Windows / Linux 托盘仍只显示应用图标；Widget 最多同时显示两个供应商，旧值 `both` 仍是 Codex + Grok；三端另有可选的置顶桌面悬浮窗（偏好对象 `floatingWindow` 仅 Go 版写入，默认省略），样式独立于状态栏，提供胶囊、卡片、圆环、电池四种，最多显示三个供应商，可调不透明度、锁定位置与是否显示在全屏应用上方，并可按前台应用以白名单 / 黑名单显隐（桌面在前台时始终显示并置顶；白名单下名单外应用到前台时默认取消置顶并退到其前台窗口之下、保持显示，可改为隐藏（`floatingWindow.hideOffList`），黑名单仍为隐藏；名单来自系统已安装应用或手动选择的文件，前台检测为 250ms 轮询，macOS 用 NSWorkspace、Windows 用前台窗口进程与 AUMID、Linux 仅 X11，Wayland 下不可用）；Windows 后端不支持透明内容，卡片铺满窗口并由 DWM 圆角，全屏检测为 2 秒轮询，Linux Wayland 下置顶与定位由合成器决定，三端真机交互仍待验收
- 主应用支持浅色、深色、跟随系统，以及英文、简体中文、繁体中文、韩文、日文、俄文、法文；显示字体可选“默认”（系统字体）或本机已安装字体族，作用于主面板与设置窗口，偏好键 `fontFamily` 仅 Go 版写入，菜单栏图标保持系统字体
- 自动测试和交叉编译不能替代三端安装、桌面与迁移验收，待验收项见 [验收清单](docs/v1-acceptance-checklist.md) 与 [发布说明](docs/v1-release.md)

### 平台与安装

项目仓库为 [Licoy/Quosparr](https://github.com/Licoy/Quosparr)，安装包按系统和 CPU 架构选择，从 GitHub Releases 下载；不把已生成的本地资产或草稿描述为已发布版本

| 系统 | 安装包 | 安装方式 |
| --- | --- | --- |
| macOS 12+ Apple Silicon / Intel | `Quosparr-macos-arm64.dmg` / `Quosparr-macos-x86_64.dmg`，或同名 `.zip` | 退出旧实例后将 `Quosparr.app` 放入 `Applications` |
| Windows 10 / 11 amd64 / arm64 | `Quosparr Setup <version> <arch>.exe` | 用户级安装到 `%LOCALAPPDATA%\Programs\Quosparr`，不要求管理员身份 |
| Linux amd64 / arm64 | `quosparr-<version>-linux-<arch>.tar.gz` 与 `install.sh`，或 `quosparr_<Debian-version>_<arch>.deb` | tar.gz 使用可写用户目录，deb 通过包管理器安装到 `/opt/quosparr` |

- Linux 需要 GTK3、AppIndicator、支持托盘的桌面，以及用于代理凭据的可用 Secret Service 会话；纯原生界面不加载 WebKitGTK，`mygo.json` 的 `linux.webview` 为 `false`，deb 与 install.sh 不依赖也不检查它，引入网页视图前须先改回；缺少桌面库时应明确提示，deb 通过包管理器更新，只读或受限目录需要在可写位置手动安装
- macOS 包目前为 ad-hoc signed，未公证，首次打开可能需要在系统“隐私与安全性”中授权；Developer ID 签名与公证单独推进
- Homebrew cask 生成器已准备，外部 `Licoy/homebrew-tap` 尚未同步新名称，当前工作流也暂停了 cask 准备与分发；恢复并发布后才使用 `brew install --cask licoy/tap/quosparr`，此前使用 DMG，详见 [Homebrew 同步说明](docs/v1-homebrew.md)
- 普通卸载保留本地数据，显式 `--zap` 只清理 Quosparr 数据，不清理保留的旧应用目录或官方 Codex / Grok 文件

### 更新与 Swift 用户迁移

- 三端使用 mygo 原生更新器检查、下载、验签、安装和重启，自动检查遵循保存的偏好，开启时每次启动后检查一次、运行期间每小时检查一次（失败后 1 小时重试），默认不自动下载安装；开发包禁用更新器
- 稳定版标签 `v<version>` 在全部资产验签通过后由 Actions 自动公开并设为 latest；Beta 候选标签 `beta-v<version>` 只生成标记 prerelease 且 `latest=false` 的草稿，固定 `v1-beta` 频道更新和 Homebrew 阶段仍暂停
- 恢复 Beta 分发后使用独立的 `v1-beta` 频道与不可变候选 Release，二者均为 prerelease 且 `latest=false`，不改生产 latest；稳定版标签 `v<version>` 必须匹配 `mygo.json` 的版本
- 新身份是 `com.quosparr.app`、`quosparr://` 和 `~/.quosparr`，旧名称用户需退出旧应用后手动安装；旧 Sparkle feed 只提供下载页入口，不能跨 bundle ID 自动安装；同身份 Quosparr Swift 版本经独立 feed 在线升级到 Go 版本
- 首次启动复制旧应用数据并导入缺失偏好，保留原目录和已有 Quosparr 数据；新目录已存在时也补入缺失文件与账号 ID，新账号元数据、当前选择和数据库整组优先
- 迁移采用暂存事务并支持中断恢复，完成标记防止已删除的旧账号重新导入；引用的代理凭据在系统凭据库内复制，可能需要授权，迁移本身不修改官方 Codex / Grok 登录文件
- 新身份不自动继承系统权限，安装后重新添加桌面组件并检查登录项；已识别旧登录项在新项启用确认后清理，保留用户禁用选择，macOS 遗留的旧 SMAppService 注册可能仍需用户在系统登录项中手动移除
- 旧名称的账号包仍可导入，新导出采用 `quosparr-accounts`；Swift 主应用源码已移除，不提供 Swift 回滚包

### 官方 CLI 目录与凭据

- Codex 账号、会话与统计使用 `CODEX_HOME`，默认 `~/.codex`，Windows 默认 `%USERPROFILE%\.codex`
- 从 Codex `config.toml` 读取 `cli_auth_credentials_store`，未设置或 `file` 受支持，`keyring`、`auto`、`ephemeral` 明确报不支持并阻止官方凭据导入与切号
- 后台同步不恢复缺失的登录文件，刷新 token 不覆盖外部切号、退出登录或并发凭据变更；会话目录读取失败时在备份、替换索引前终止修复
- Grok 依赖官方 CLI，使用 `GROK_HOME` 或默认 `~/.grok`；多账号与额度只支持 OAuth / SuperGrok 及兼容 legacy session，可通过 `grok login --oauth`、应用内登录、导入当前登录或粘贴凭据添加，API Key 单独登录不加入托管账号
- Grok 官方凭据为 `$GROK_HOME/auth.json`，托管副本为 `~/.quosparr/accounts/grok-<stable-id>/auth.json`，无 token 的索引为 `~/.quosparr/accounts/grok-index.json`
- Grok 切号只替换 OAuth / 兼容 legacy scope，保留 API Key 与未知 scope，仅保证新会话使用新账号；不强制终止外部 Grok 进程，切换前提示旧进程可能写回旧账号
- Claude 使用 `CLAUDE_CONFIG_DIR`，默认 `~/.claude`。macOS 官方凭据在钥匙串条目 `Claude Code-credentials`（设置了 `CLAUDE_CONFIG_DIR` 时 service 名带目录哈希后缀），通过 `/usr/bin/security` 读写；Windows / Linux 为 `<config>/.credentials.json`。托管副本为 `~/.quosparr/accounts/claude-<stable-id>/auth.json`，索引为 `claude-index.json`
- Claude 切号只替换凭据里的 `claudeAiOauth`，保留 `mcpOAuth`，并只替换 `.claude.json` 的 `oauthAccount`。当前账号不刷新，过期时提示运行一次 `claude`。非当前账号由 Quosparr 刷新，结果只写账号库副本。应用内浏览器登录无需先运行或安装 CLI，支持取消和粘贴回调；可信身份与两种 token 仅事务写入账号库，不自动切号，不改官方凭据、`.claude.json` 或 `settings.json`，与实际官方订阅相同且已有托管记录的登录只报告已存在，不替换托管副本；账号库缺失该账号时，仅事务镜像已复核的官方凭据及身份，保持官方真实到期状态，不采用新 OAuth 凭据覆盖官方权威，仍返回已存在；该分支按已复核的实际 `settings.json` 反映当前账号，未启用 API 时同步官方订阅为当前，实际选用 API 时保留对应 API 当前标记，避免把官方 token 当作非当前刷新；重复的非当前账号保留别名、排序、创建时间与历史缓存，更新 token 与到期时间；浏览器等待和网络交换不占账号模块锁，提交前复核官方登录、身份来源和取消状态，失败恢复原凭据与索引并明确报告回滚失败；刷新时自动导入并跟随官方当前登录（`oauthAccount` 缺失时按 token 调用一次 profile 识别身份，无法识别时不入库），外部登出后清除当前标记，只写账号库副本。不强制终止外部 Claude Code 进程
- macOS 钥匙串写入经 `security -i` 标准输入，单行上限 4095 字节，超限时拒绝而不改用 argv；写入后读回逐字节比对，不一致时回滚到原条目；该条目不存在时写钥匙串，不新建明文 `.credentials.json`

### 开发运行与 Widget

使用 `go.mod` 固定的 Go 1.27.1 和 mygo v0.6.2，默认隔离运行示例：

```bash
QUOSPARR_TEST_HOME="$(mktemp -d)"
HOME="$QUOSPARR_TEST_HOME" QUOSPARR_PREFS_DOMAIN=com.quosparr.app.v1-test go run . --self-check
V1_DEVELOPMENT_PACKAGE=1 bash Scripts/package-v1-app.sh
# 可选 ARCH=arm64 或 ARCH=x86_64
```

- Go 自检不请求网络、不解析凭据，只报告版本 / 平台、自有目录权限与可写性、锁状态、Codex auth 与会话索引是否存在；已有自有目录会创建并移除临时写入探针
- 自检另报告 Claude 配置目录和凭据文件是否存在，不解析、不读取钥匙串内容。脱敏认证摘要、token 有效性、会话 dry-run 摘要、Grok CLI 版本、Grok 认证状态与身份、Claude CLI 版本与认证身份仍明确列为 `not ported yet`，不能描述为完整诊断
- Go 桌面开发入口为 `bash Scripts/run-dev-app.sh`：以正式身份 `com.quosparr.app` 构建开发包（禁用更新器，`--widget` 时含 Widget，`--tier-badges` 另开窗口预览三家全部订阅徽标的浅色 / 深色样式，即 `QUOSPARR_DEV_TIER_BADGES=1`，仅开发包或测试偏好域生效）并在前台使用真实 HOME、数据与偏好运行，需先退出已安装实例；`mygo dev` 的 `.dev` 身份与正式应用冲突且无数据，不作为开发入口；隔离运行使用临时 HOME 加 `QUOSPARR_PREFS_DOMAIN=<测试域> go run .`；系统集成与升级验收使用一次性 VM
- macOS 开发包输出到 `build/v1-<arch>/darwin-<goarch>/Quosparr.app` 和 `Quosparr-macos-<arch>-dev.zip`，目前保留正式 bundle 身份；正式打包默认 `V1_DEVELOPMENT_PACKAGE=0`，签名私钥通过发布环境提供
- Widget 要求 macOS 14+，保留四类组件、供应商选择和深链，Windows / Linux 不提供 WidgetKit；现有桌面组件的升级接续仍需实际验收
- Widget 快照为 schema 1，位于 `~/.quosparr/widget-snapshot.json`，权限 `0600`，只含非密钥派生数据，不含邮箱、账号 ID、token、完整认证 JSON 或事件原文；主程序是唯一写入者，组件只读

### 代理、隐私与统计口径

- 代理支持跟随系统、HTTP、SOCKS5 及用户名 / 密码，覆盖应用发起的 Codex / Grok / Claude 请求、CLI 操作、公开状态 / 价格源和软件更新；不修改系统代理，浏览器登录页与已经运行的外部 CLI 沿用各自设置
- 代理修改仅在“保存代理设置”后生效，进行中的更新保持启动时配置；“测试连接”只用当前草稿访问测试站点（默认 `https://github.com`，可自定义并保存为偏好，任何非 407 / 5xx 响应视为连通），不发送账号凭据、不保存草稿；错误配置、凭据不可用或自定义代理失败不能自动直连
- 非密钥派生数据保存在 `~/.quosparr`，Windows / Linux 偏好在自有目录；代理密码分别保存在 macOS Keychain、Windows Credential Manager、Linux Secret Service
- API 等价成本默认来自本机会话日志，不上传会话内容；周额度推算只保存 `quota-estimate-history.json` 中的派生 Credits 与占用率，不保存 token 或密钥
- 图表“官方统计（多端）”属于当前账号，可能延迟或修订；“本机日志（全部本机会话）”可能跨账号，不能视为包含关系或相减；Codex 两种按日统计均按 UTC 日界比较
- Codex 内置 API 价格表版本为 `openai-builtin-2026-09-27`，共享公开目录采用 models.dev 快照版本，仅完整支持的场景采用其费率，缺失的必要价或不明确的阶梯边界保留为未定价，已核实的内置特殊规则仍优先用于对应场景，未知模型不报告精确费用；订阅额度推算使用 `credits-usd-2026-08-26`，1000 Credits 约对应 40 美元，属于非官方推算
- 模型价格设置位于高级之前并与其同组，仅展示 models.dev 中 providerID 精确为 `openai`、`anthropic`、`xai` 的三家原始服务商条目及对应内置价格，使用与 Skill / MCP 管理一致的固定 Codex / Claude / Grok tabs，默认 Codex，每页默认显示该商全部模型，切换时清空搜索与筛选、关闭详情并重置分页，不提供其他厂商入口，不展示第三方代理或其他服务商的同名模型；models.dev 是聚合公开目录而非供应商官方实时接口，通用下载与缓存接口保持不变，`modelPricing` 偏好默认 `{autoSync: true, syncIntervalSeconds: 3600}`，自动同步可关闭，频率范围为 15 分钟至 7 天，修改立即保存且失败回滚；同步设置由 tabs 右侧“频率设置”按钮打开原生弹窗，设置共同适用于三家，开关、频率、完整时间与版本状态在弹窗内即时保存，主界面保留简短状态与错误，关闭弹窗不取消同步任务，离开该设置页关闭弹窗；手动同步不依赖自动开关，UI 只经业务 actions 读取不可变目录与同步状态，不扫描文件、不联网
- 模型价格首版只覆盖 USD / 百万 Token 的参考价，详情须说明目录参考价与实际采用来源、版本和规则差异；已核实的长上下文、缓存写入 TTL、Fast 与地区规则保持结构化并经七语言渲染；missing 与明确免费必须区分，未知或缺少必要规则不得精确估价，不能把 `price-source:*` 来源信息当未定价警告
- 历史用量按当前价格重新估算，价格变更必须使派生成本缓存失效；总额、每日、项目、模型、会话按 `ConfidencePartiallyPriced` 或未定价 Token 数显示部分成本，完整缺失清单不受摘要表前 8 行限制，扫描不完整与价格缺失分别提示；不改订阅 Credits、quota 或 reset credits 口径，目录网络仅取公开数据，不上传会话内容，离线保留内置价格与最后成功缓存，同步错误必须可见
- Grok 额度只使用官方 CLI chat-proxy `/v1/billing?format=credits`，不读取浏览器 Cookie、不从本地日志伪造额度、不混入 API 账单；API 等价成本使用本地 `turn_completed` 用量和 `xai-builtin-2026-09-27`，区分输入、缓存、输出及长上下文价格，`costUsdTicks` 不作为 API 等价成本
- Claude 订阅用量来自非公开的 OAuth usage 端点，界面写作 “Claude Code 订阅用量”，不写成官方额度接口。重置次数取自同一请求附带 `cedar_ember=1` 返回的 `cedar_ember` 块（不加 `skip_spend`，以保留额外用量），只读展示、不代为使用；账号未参与、API 账号或该块缺失、格式异常时面板仍显示该模块并计为 0，Widget 只在有该块时携带重置次数。API 等价成本使用本地会话日志和 `anthropic-builtin-2026-10-14`，按 message id 去重，缓存写入分 5 分钟与 1 小时，Fast 档单独计价，未知模型不给精确费用。日界用本地时区。Claude 日志没有首 token 时间，速率按端到端耗时计算

### 提交与推送边界

- 提交源代码、测试、依赖锁定文件、必要资源与 fixture；生成的 glyph Go 源文件、`internal/l10n/tables.go` 和兼容测试的 SQLite、JSONL、压缩样本属于源码或测试资产，不按扩展名批量删除
- 构建目录、安装包输出、Python 字节码、IDE 用户状态、本地临时目录、发布说明生成结果及签名私钥不进入 Git；清理只针对确认可重新生成的本地文件
- 推送 `main` / `v1` 触发普通 CI，开发打包无需发布 Secrets；只改动 `website/` 或 `website.yml` 的推送与 PR 不触发 CI，只由 `website.yml` 测试与部署官网；版本标签触发签名打包与 Release（稳定版自动公开，Beta 为草稿），Release 不重跑测试而是等待同一提交的 CI 通过（标签提交及其之前只改官网的提交跳过，等待最近一个 CI 覆盖的提交），需要 `MYGO_UPDATE_PRIVATE_KEY`、`SPARKLE_PUBLIC_KEY`、`SPARKLE_PRIVATE_KEY`，私钥必须匹配配置公钥
- 本地测试通过不等于线上 Actions 已通过，推送后需核对 Windows / Linux 原生安装与 SQLite 验证、macOS 双架构打包的实际结果
