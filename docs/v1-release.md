# v1 发布

v1 已正式发布（`v1.0.0`、`v1.0.1`），Swift 主应用已从仓库移除，不再提供 Swift 构建或应急回滚；本文件描述后续版本的流水线与门禁，安装升级验收仍须在一次性虚拟机执行

稳定版 `v*` 和 Beta `beta-v*` 标签均执行测试、打包、验签及附件上传，先以 `--draft --latest=false` 创建草稿，Beta 草稿同时标记 `--prerelease`；稳定版在全部资产验签通过后由同一作业公开并设为 latest，mygo 清单与 Sparkle appcast 均从 `/releases/latest/download/` 读取，公开后用户即可收到更新；任一校验失败则不创建或不公开 Release；Beta 保持草稿，固定 `v1-beta` 频道更新及 Homebrew Cask 准备步骤仍在工作流中注释

## 更名与旧版本入口

Quosparr 使用 `com.quosparr.app` 和 `quosparr://`，Git 仓库地址保留；旧名称客户端的 `appcast-<arch>.xml` 只包含版本与下载页 `<link>`，不得包含新应用的 `<enclosure>`；已安装的 Quosparr Swift 版本与 Go 应用同为 `com.quosparr.app`，其 Sparkle feed `quosparr-appcast-<arch>.xml` 指向本次发布的已签名 ZIP，直接升级到 Go 版本；旧名称首次更名安装采用退出旧应用、手动安装、启动迁移的流程

首次启动将 `~/.codex-runway` 缺失的数据补入 `~/.quosparr`，新目录已存在也可接续；同路径新文件优先，两类账号索引按 ID 合并且保留新元数据、当前选择与排序，数据库和 WAL/SHM/journal 按整组保留新版本；在私有暂存目录完成校验后切换，进程中断后恢复原目录或已完成结果，完成标记防止以后再次导入已删除的旧账号，旧源目录始终保留；偏好依次从预览身份 `com.github.quosparr`、`com.github.codex-runway` 和早期 `dev.licoy.codex-runway` 导入缺失项，代理凭据复制到新 service；权限、坏索引、类型冲突等错误显式终止迁移，不以默认配置掩盖

登录项迁移只处理已知旧身份及匹配命令，覆盖 macOS 会话登录项 / 旧 LaunchAgent、Windows Run、Linux autostart；先注册并确认新项启用，再删除旧项，旧项被禁用则不重新启用，等待系统批准时保留旧项并在批准后重启应用完成迁移；macOS 公开 API 无法由新 Bundle ID 直接注销旧应用的 `SMAppService.mainApp`，若系统设置仍列出旧应用，需在“通用 → 登录项”手动移除，不能宣称所有系统记录都已自动清理；旧 Widget 需重新添加，真实登录 / 重启与系统权限仍需验收

## 密钥与版本

产品版本只来自 `mygo.json` 的 `version`；macOS 的 `macos.infoPlist.CFBundleVersion` 是独立递增整数，必须至少为 100，并严格大于全部已发布 appcast 的构建号；宿主、Widget 和 Sparkle appcast 使用同一构建号

GitHub Actions 需要 `MYGO_UPDATE_PRIVATE_KEY`、`SPARKLE_PUBLIC_KEY`、`SPARKLE_PRIVATE_KEY` 三个 secrets；本地脚本将 `MYGO_UPDATE_PRIVATE_KEY` 映射到 mygo v0.6.2 实际接受的 `MYGO_UPDATER_PRIVATE_KEY`；私钥只放受限临时文件或 secrets，不写配置、命令参数、仓库和日志

mygo 与 Sparkle 使用独立密钥；保留现有 `SUPublicEDKey`；`SPARKLE_PRIVATE_KEY` 为 Sparkle `generate_keys -x` 导出的 base64 32 字节种子；appcast 不依赖 Sparkle 工具，由 `go run ./Scripts/release-assets sparkle-appcast` 直接生成，以 Ed25519 对 ZIP 原始字节签名，与 `sign_update` 的结果一致；`legacy-appcast.sh` 要求 Sparkle secret 的公钥等于该字段，生成后再用固定公钥执行 `release-assets verify-sparkle`；Go 宿主不携带 Sparkle 运行库

正式发布前运行 `python3 Scripts/check-v1-build-number.py`，它使用 `gh` 读取全部非 draft release 的两份 appcast，任何网络错误、缺少历史 appcast、不可解析构建号或新构建号不够大都会阻止发布；`--appcasts` 只用于明确指定的隔离测试样本，不是生产查询失败时的后备路径

## 构建与校验

```bash
# 开发包不启用更新器，也不生成更新清单
ARCH=arm64 V1_DEVELOPMENT_PACKAGE=1 bash Scripts/package-v1-app.sh
V1_DEVELOPMENT_PACKAGE=1 bash Scripts/package-v1-platform.sh windows/amd64
V1_DEVELOPMENT_PACKAGE=1 bash Scripts/package-v1-platform.sh windows/arm64
V1_DEVELOPMENT_PACKAGE=1 bash Scripts/package-v1-platform.sh linux/amd64
V1_DEVELOPMENT_PACKAGE=1 bash Scripts/package-v1-platform.sh linux/arm64

# 正式 macOS 包，签名密钥需提前放入环境
ARCH=arm64 V1_DEVELOPMENT_PACKAGE=0 bash Scripts/package-v1-app.sh
ARCH=x86_64 V1_DEVELOPMENT_PACKAGE=0 bash Scripts/package-v1-app.sh

# Windows 与 Linux，逐个选择目标
bash Scripts/package-v1-platform.sh windows/amd64
bash Scripts/package-v1-platform.sh windows/arm64
bash Scripts/package-v1-platform.sh linux/amd64
bash Scripts/package-v1-platform.sh linux/arm64
```

macOS 输出目录为 `build/v1-<arm64|x86_64>`，Windows/Linux 为 `build/v1-<os>-<arch>/<os>-<arch>`；`V1_BUILD_DIR` 可指定绝对输出路径；mygo CLI 的临时配置在仓库 `.build` 下创建并清理，避免配置移到 Go 模块外导致构建失败；`V1_CONFIG_PATH` 可选择经过审阅的隔离配置

先构建宿主、桥接与 Widget，写入最终 plist、资源和 SQLite，再从内到外签名；mygo 提前生成的 macOS 更新归档会被删除，最终 tar、DMG、Sparkle ZIP 均从同一份已验签 `.app` 生成；打包后再改资源必须重新执行整条链路

| 目标 | 必需资产 |
|---|---|
| darwin-arm64 | `Quosparr-macos-arm64.zip`、`.dmg`、`quosparr-<version>-darwin-arm64.tar.gz`、`update-darwin-arm64.json`、`quosparr-appcast-arm64.xml`、手动下载提示 `appcast-arm64.xml` |
| darwin-amd64 | `Quosparr-macos-x86_64.zip`、`.dmg`、`quosparr-<version>-darwin-amd64.tar.gz`、`update-darwin-amd64.json`、`quosparr-appcast-x86_64.xml`、手动下载提示 `appcast-x86_64.xml` |
| windows-amd64 / arm64 | `Quosparr.Setup.<version>.<arch>.exe`、`quosparr-<version>-windows-<arch>.tar.gz`、`update-windows-<arch>.json` |
| linux-amd64 / arm64 | `quosparr_<debVersion>_<arch>.deb`、`quosparr-<version>-linux-<arch>.tar.gz`、`update-linux-<arch>.json`、`install.sh` |

Windows 使用 mygo 默认的用户级 NSIS 安装器，安装到 `%LOCALAPPDATA%\Programs\Quosparr`，`RequestExecutionLevel user`，不要求管理员权限；未生成 EXE 安装器时脚本报错；Linux tar 使用用户目录安装器，deb 声明 GTK3、AppIndicator 与密钥存储依赖，`linux.webview` 为 `false` 时不依赖 WebKitGTK，平台校验拒绝含 WebKitGTK 依赖的 deb，由包管理器管理更新；SQLite 运行库缺失或架构错误会阻止验收，应用不依赖首次启动时下载该库

Windows 构建目录保留 mygo 的 `Quosparr Setup <version> <arch>.exe` 名称，发布汇总时将空格替换为点号，再生成校验和，确保文件名与 GitHub 下载资产一致；独立的 `Quosparr.exe` 只属于应用载荷，不单独上传到 Release

Windows 将对应架构的 SQLite DLL 嵌入 EXE，安装器和更新归档不再附带外置 DLL，单独复制 EXE 不会遗漏运行库；首次使用时从内嵌资源原子释放到 `~/.quosparr/runtime/sqlite/<SHA256>/mygo-sqlite3.dll`，目录和文件仅当前用户可访问，加载前检查内容；已有缓存损坏或路径异常会明确报错，不下载替代库、不加载 EXE 旁边的 DLL；缓存缺失时可从内嵌资源重新创建

内嵌文件与 mygo v0.6.2 清单中的 Windows SHA256 一致；该版本 CLI 没有排除单个原生资源的配置，因此打包脚本复制固定版本 CLI 到临时目录，只调整 Windows SQLite 的外置资源收集，构建后清理副本，不修改 Go 模块缓存或升级框架；macOS / Linux 保留原有随包动态库

普通 CI 在 Windows 与 Linux 的 amd64、arm64 原生运行器上执行完整 Go 测试、构建开发安装包、核对载荷，并加载 SQLite 执行参数化查询；Windows 在一次性 Actions VM 内验证静默安装、临时 HOME 下的 `--self-check` 和卸载，并将 EXE 单独复制到空目录，通过 `QUOSPARR_DEV_VERIFY_SQLITE=1` 检查内嵌库解包和真实 SQLite 查询，删除缓存后再检查离线重建；Linux 用 `dpkg-deb` 解包后运行相同基础诊断；两者均不启动桌面界面、不登录真实账号；macOS 继续运行全量 race 和六目标交叉构建

版本标签的 Release 不重跑测试，`ci-gate` 等待同一提交的 push CI 完成，通过后才签名打包；推送 main 的 CI 按提交分组、不被后续推送取消。标签须指向已推送到 main 的提交；CI 失败时先重跑 CI 的失败 job，再重跑 Release

`V1_DEVELOPMENT_PACKAGE=1` 明确关闭更新器，不需要发布密钥且不生成更新清单；开发包也必须通过可执行文件 / SQLite 架构、资源完整性和安装器结构校验，Linux 额外逐文件比对 deb 与归档载荷；`verify-payload` 仅验证包内容，不能代替正式发布的 `verify` 签名门禁；默认正式构建缺少 `MYGO_UPDATE_PRIVATE_KEY` 时仍直接失败

本地生成安装包和交叉编译不能证明目标系统原生步骤已通过，修改后的 Actions 必须在上传同一版本代码后实际执行；这些自动检查也不能替代真实桌面的 OAuth、托盘、系统钥匙串、权限提示及升级验收

合并各目标的平面资产，并用 `cmp` 确认两个 Linux `install.sh` 一致后保留一份同名脚本，然后运行

```bash
bash Scripts/legacy-appcast.sh /absolute/path/to/dist mygo.json v<version>
python3 Scripts/release-checksums.py write /absolute/path/to/dist
bash Scripts/verify-v1-release.sh /absolute/path/to/dist mygo.json
```

校验先核对所有资产的 SHA256，再核对 mygo 归档签名、版本、长度、目标与 URL，验证 Sparkle ZIP 公钥签名后才解包或挂载；归档禁止路径穿越、重复条目和符号链接循环；每个可执行文件和 SQLite 都检查真实 Mach-O/PE/ELF 架构；macOS 还验证可执行权限、桥接、Widget、1024px icns、深度签名及 tar/ZIP/DMG 内容一致；Windows EXE 内必须包含 256px 图标，NSIS 通过 `7z` 解包后与已验签 tar 逐文件比较；Linux deb 的架构、版本、依赖和 payload 与 tar 比较

## Beta 与 latest

稳定版使用 `v<version>` 与生产 `/releases/latest/download/`；Beta 的 `mygo.json` 版本必须有 prerelease 后缀，并用 `V1_UPDATE_CHANNEL=beta` 生成；Beta 包内的 `updates.url` 固定为 `https://github.com/Licoy/Quosparr/releases/download/v1-beta`

当前 Beta 候选只保存为 `beta-v<version>` 的 prerelease 草稿，不更新固定的 `v1-beta` 渠道；恢复分发时，候选与固定频道都必须 `latest=false`；mygo v0.6.2 的 GitHub `tagPrefix` 查询会排除 prerelease，所以不能用 prefix 查询代替这个固定渠道；生产 `/latest/` 与 Sparkle appcast 入口均不受 Beta 影响

Homebrew Cask 准备目前暂停，恢复后仍须在全部资产校验通过后同步 DMG 的版本与 SHA256，默认卸载仅移除应用，自有配置仅在用户主动 `--zap` 时删除

## CLI 数据与认证边界

Go 主应用的 Codex 账号、统计与会话修复统一使用 `CODEX_HOME`，默认 `~/.codex`，Windows 默认 `%USERPROFILE%\.codex`；Grok 继续使用 `GROK_HOME` 或相应用户目录下的 `.grok`；自有数据始终保存在 `.quosparr`

Codex 的 `config.toml` 未指定 `cli_auth_credentials_store` 或指定为 `file` 时，使用文件凭据；`keyring`、`auto`、`ephemeral` 会显示七语言的不支持提示并阻止读取旧 `auth.json`、导入官方凭据和切号，不擅自更改 CLI 配置或访问其系统密钥项；托管账号副本仍可独立保留和刷新

后台同步不重建缺失或损坏的 Codex 官方登录文件，退出登录后刷新只保留托管副本；Grok 刷新写回前在认证锁内重新核对当前账号及原始文件，外部切号、退出登录或同账号 token 轮换时停止写回；目录遍历和文件读取错误显式传播，会话修复必须在完整扫描成功后才备份和替换索引
