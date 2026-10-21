# Cursor 额度监控

实现状态：实验性接入，默认关闭。在主面板的 Cursor 栏位、“设置 → 服务商账号 → Cursor”或“设置 → 显示 → Cursor”开启，只读当前桌面登录。服务商账号页显示当前账号、套餐和监控状态，并可手动刷新；主面板、状态栏与悬浮窗支持 Cursor。账号切换、会话、Skills、MCP 与 Widget 仍只支持原有供应商。

## 数据与隐私边界

- 仅读取 `<Cursor 用户数据目录>/User/globalStorage/state.vscdb` 中的 `cursorAuth/accessToken`、`cachedEmail`、`stripeMembershipType`、`activeDeploymentId`。SQLite 只读打开，包含已提交的 WAL 数据；缺文件不创建。
- 采用 macOS、Windows、Linux 的默认目录，支持继承的 `VSCODE_PORTABLE`、`VSCODE_APPDATA`、`APPDATA`、`XDG_CONFIG_HOME`。单独以 `--user-data-dir` 启动的编辑器目录暂不自动发现。
- 不读浏览器 Cookie、refresh token、对话或工作区；不复制凭据到账号库，不刷新或写回官方登录。关闭开关会取消请求并清除内存状态。
- 加密凭据、仅内存登录、隔离部署暂不支持。隔离部署 token 不发送到生产域名。
- 网络请求经过应用代理策略，不自动直连回退。固定访问 `https://api2.cursor.sh/aiserver.v1.DashboardService/`，禁用 Cookie 和重定向，错误中不包含凭据、响应正文或本地认证 JSON。

## 接口口径

依据 2026-10-21 安装的 Cursor 3.26.13 桌面协议：Connect unary POST，裸 protobuf，空请求体。读取 `GetPlanInfo`、`GetCurrentPeriodUsage`，非合并套餐另读 `GetAggregatedUsageEvents`（空请求体即当前账期）；合并套餐另读 `GetClientVisibleCreditGrants`。这些是桌面内部接口，没有稳定的兼容承诺。

`plan_usage` 按官方用量页口径显示 Cursor 模型（`autoPercentUsed`）与其他模型（`apiPercentUsed`）两个百分比，不设总池：`totalPercentUsed` 与 `totalSpend` 含服务商赠送的免费超额（`bonusSpend`），不展示；合并套餐只显示共享百分比。

金额口径：已花为本账期用量事件 `totalCents` 按池汇总（tier 2 或 composer / cursor-grok 开头的模型计入 Cursor 模型，tier 1 计入其他模型，Bot / sand 用量不计入），总额为已花 ÷ 服务端百分比、取整到 10 美元，界面标注“估算”。百分比为 0、达到 99.5% 或无已花时不反推，只沿用本次运行中同一账号上次的估算，不持久化；`includedSpend` / `limit` 只是付费包含额度（用尽后的用量记为赠送的 `bonusSpend`），不作为任何池的上限。额外消费与 credit grants 单列，不能相加为订阅剩余额度。百分比与重置时间只用服务端值，估算总额不参与百分比计算；缺少百分比或用量时不补出 100% 可用额度。金额按整数 cents 保存，只有月度周期借用账单截止日，未指定周期不推算重置日，周额度不使用月账单截止日期。主面板显示数据时间，失败后保留的旧数据明确标记，状态栏与悬浮窗不展示旧百分比。

后台请求最短间隔 5 分钟；用量事件请求失败（鉴权与限流除外）只跳过金额估算，不影响百分比；读取登录偶发失败或请求超时保留旧数据并标记，不清除 `Retry-After` 与最短间隔；手动刷新可以提前，但必须遵守服务端 `Retry-After`。请求完成后重新读取登录指纹（按账号邮箱，token 正常轮换不算切号），外部切号、退出或关闭开关后丢弃旧响应；Cursor 以外的刷新操作不强制请求 Cursor。未开启监控时不读取凭据；隐藏且其他展示位置未使用 Cursor 时不轮询。

## 验证与待验收

自动化使用临时 SQLite 与合成 protobuf/HTTP 响应，覆盖 WAL、加密/隔离拒绝、字段缺失、未知字段、限流、认证失败、重定向、外部切号、退出、关闭开关、七语言布局及 CLI 管理边界。测试不读取真实 Cursor 登录。

2026-10-21 已在本机 macOS 的 Team 账号上验证最小 Bearer + Connect 请求成功获取套餐与额度，并与官方用量页及其 `get-current-period-usage` JSON 逐字段核对（Cursor 模型、其他模型百分比，账期与套餐内金额一致），并完成服务商账号页的入口、当前账号展示及手动刷新验证，未添加机器标识。其它套餐响应、与官方面板的逐项核对及三平台完整安装验收尚未完成。

隔离验收时，在测试系统登录 Cursor，然后手动开启监控，对照 Cursor 用量页核对池、百分比、金额与重置日；测试切号、退出、代理失败、关闭开关和重新开启。不得因接口拒绝而读取浏览器 Cookie、修改真实登录文件或关闭 TLS 校验。
