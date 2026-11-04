# v1 Homebrew 安装信息

本仓库提供 `Scripts/generate-v1-homebrew.py`，从最终校验通过的两份 DMG 生成候选 cask，版本取自 `mygo.json`，下载名称分别为 `Quosparr-macos-arm64.dmg` 和 `Quosparr-macos-x86_64.dmg`

```bash
python3 Scripts/generate-v1-homebrew.py /absolute/path/to/dist mygo.json /absolute/path/to/dist/quosparr.rb
ruby -c /absolute/path/to/dist/quosparr.rb
```

生成器拒绝 Beta 版本、开发 bundle identity、缺失或空 DMG，稳定发布工作流只在六目标资产验证完成后生成 `quosparr.rb` 并随 release 附件交付；候选文件不会自动提交、推送或修改 [Licoy Homebrew Tap](https://github.com/Licoy/homebrew-tap)，本轮未同步远端 tap

新名称 cask 尚未发布到外部 tap，以下安装命令需在维护者发布后使用；发布稳定版后，将候选文件与 tap 的 `Casks/quosparr.rb` 对照，复核版本、两架构 SHA256 和下载 URL，再由维护者完成外部 tap 的更新；Beta 使用独立 prerelease 渠道，不更新稳定 cask

```bash
brew install --cask licoy/tap/quosparr
brew upgrade --cask --greedy quosparr
brew uninstall --cask quosparr
```

默认卸载仅退出并移除应用，保留设置、账号副本、统计与小组件快照；显式 `brew uninstall --cask --zap quosparr` 才清理 `~/.quosparr`、Quosparr 应用支持目录和偏好域，保留更名前的目录及偏好域，候选 cask 不清理 `~/.codex`、`~/.grok` 或任何会话目录

应用最低支持 macOS 12，小组件要求 macOS 14；Homebrew 安装位置或当前用户权限阻止应用内安装时，使用上述 `brew upgrade` 或从稳定 release 下载 DMG 覆盖安装

本轮仅校验生成器、cask 语法和资产命名，未执行真实 Homebrew 安装、升级、卸载或远端 tap 推送
