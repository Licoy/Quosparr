# AGENTS.md

本文件只记录适用于整个仓库的全局开发规范。

## 本文件的维护规则

- 严禁在本文件写入任何业务逻辑说明，包括但不限于：功能行为、界面流程、用户数据路径与文件格式、读写或删除边界、供应商差异、计价与统计口径、版本历史、迁移与升级事项
- 业务行为一律以实际代码为准：实现、测试与包级 doc 注释是唯一依据；需要固化的业务约束写进对应包的 doc 注释并用测试锁定，不写进本文件
- 文档（含本文件、README、`docs/`）与代码不一致时以代码为准，并在回复中指出不一致之处
- 新增条目前先确认它对整个仓库普遍适用；只对某个包或某项功能成立的规则不属于本文件

## 技术栈

- 主程序为 Go + mygo/ui 原生界面；Swift 只用于 macOS WidgetKit 扩展（`Sources/QuosparrWidget`）与桥接（`Sources/QuosparrWidgetBridge`），不新增 Swift 主应用代码或 SwiftPM 包
- Go 与依赖版本以 `go.mod` 为准；应用身份、平台最低版本与打包配置以 `mygo.json`、`WidgetExtension/project.yml` 为准，不在文档中重复具体数值
- Widget 工程由 `WidgetExtension/project.yml` 经 xcodegen 生成，除非用户明确要求，不另建 Xcode project
- 默认不引入第三方依赖，标准库和系统框架能解决的问题不加包；框架或依赖升级单独提交，不与其他改动混在一起
- 不引入 WebView 前端；purego 仅用于必要的平台 API、动态库与 Swift 桥接边界，按平台拆分并检查 ABI、资源生命周期与错误传播，不用它增加业务抽象

## 代码组织

- 入口为 `main.go`；原生 UI 与系统协调在 `internal/app`，业务模块按领域分包在 `internal/*`
- 业务包不依赖 UI；UI 只经业务包的 API 取结果，不直接解析文件或扫描目录
- 函数保持短小，优先简单结构体与函数，不做多 provider 抽象
- 新代码贴合周边代码的命名、注释密度与写法

## 本地化

- 用户可见文案经 `internal/l10n`（手工维护的 `tables.go`）或现有文案入口，新增 key 必须补齐英文、简体中文、繁体中文、韩文、日文、俄文、法文七种语言
- Widget 使用自带的 `Sources/QuosparrWidget/RunwayWidgetL10n.swift` 与快照类型副本，修改共享 key 或快照结构时与 Go 侧同步，同样保持七语言完整

## 隐私与安全

- 不在日志、测试输出、错误信息、文档与 issue 模板中输出 token、API key 等密钥或完整认证 JSON
- 调用外部命令使用绝对路径，密钥不经命令行参数传递
- 不关闭 TLS 校验

## 测试与本地运行

- 新行为在对应 `internal/*` 包补必要测试；测试只用临时目录，不触碰开发机真实的账号、凭据、偏好、钥匙串、登录项或生产 feed
- `testdata/swift_*` 等冻结 fixture 只读保留，不重新生成
- 手动隔离运行使用临时 HOME 并设置 `QUOSPARR_PREFS_DOMAIN=<测试域>`，结束后退出自行启动的进程；保留现有 `QUOSPARR_DEV_*` 环境钩子
- `Scripts/run-dev-app.sh` 使用真实 HOME、数据与偏好，仅在用户要求时运行
- 自动测试与交叉编译不能替代真机验收

```bash
go vet ./...
go build ./...
go test ./...
go test -race -count=1 ./...
go run . --self-check
bash Scripts/run-dev-app.sh --help
V1_DEVELOPMENT_PACKAGE=1 bash Scripts/package-v1-app.sh
```

## 文档

- README 与源码不包含外部参考应用的名称、链接或代码片段
- 根目录 `README*.md` 七种语言保持相同结构与事实，修改任一语言时同步其余六种；`README_EN.md` 仅作跳转
- 官网源码在 `website/`，七种语言的 `website/locales/*.json` 字段与列表长度保持一致，图标只用 `website/icons/` 中的 Remix Icon SVG
- 本地生成的资产或草稿不得描述为已发布版本或已通过验收

## 提交与推送

- 提交信息沿用 `type(scope): 中文描述` 格式
- 提交源码、测试、依赖锁定文件、必要资源与 fixture；生成的 glyph Go 源文件、`internal/l10n/tables.go` 以及测试用 SQLite、JSONL、压缩样本属于源码或测试资产，不按扩展名批量删除
- 构建与安装包输出、Python 字节码、IDE 用户状态、本地临时目录、发布说明生成结果及签名私钥不进入 Git；清理只针对确认可重新生成的本地文件
- 本地测试通过不等于 Actions 通过，推送后核对 CI 实际结果；发布流程见 `docs/v1-release.md`
