# quosparr.com

Quosparr 官网的源码。`website/gen` 用 Go 标准库把模板和七种语言的文案预渲染成静态页面，由 GitHub Actions 发布到 GitHub Pages。

## 目录

- `locales/<code>.json`：各语言文案，字段与 `gen/locale.go` 的 `Strings` 一一对应；缺字段、空字段、列表长度与英文不一致都会让构建失败
- `templates/`：页面模板，`index.html` 渲染每种语言的首页，`404.html` 为未找到页面
- `static/`：样式、脚本、站点图标与 OG 分享图；样式与脚本按内容哈希命名后输出
- `static/img/og-<code>.png`：各语言 1200×630 分享图，使用思源宋体排入该语言的 `hero.etymology` 与 `hero.tagline`；修改这两项文案后需同步重新生成
- 分享图由 `Scripts/generate-og-images.py` 用无头 Chrome 渲染：除思源宋体外，素材都取自仓库（文案、`docs/images/panel*.webp`、应用与供应商图标、`icons/`）；本机工具在仓库根目录的 `.env`（已被 Git 忽略）中配置：

  ```bash
  OG_CHROME=<Chrome 或 Chromium 可执行文件>
  OG_MAGICK=<ImageMagick 7 的 magick>
  OG_FONTS=<含 SourceHanSerif-{Heavy,SemiBold,Medium}.ttc 的思源宋体 OTC 目录>
  ```

  然后运行 `python3 Scripts/generate-og-images.py`，结果直接覆盖 `static/img/og-*.png`
- `icons/`：从 Remix Icon v4.9.1 复制的 SVG，构建时内联，不依赖 CDN
- 截图直接取自 `docs/images/*.webp`，供应商图标取自 `internal/app/assets/providers/`
- 首页右侧的主面板由 `gen/panel.go` 生成示例数据、`templates/index.html` 的 `panel` 模板以 HTML 还原，配色随站点深浅色切换，标签名沿用应用文案（`locales/*.json` 的 `panel`）；应用主面板布局或文案变化时同步调整

英文位于 `/`（同时为 `x-default`），其他语言位于 `/zh-hans/`、`/zh-hant/`、`/ko/`、`/ja/`、`/ru/`、`/fr/`。首次访问根路径时按浏览器语言跳转，用户手动选择语言后不再跳转。

## 本地预览

```bash
gh release view --repo Licoy/Quosparr --json tagName,publishedAt,assets > /tmp/quosparr-release.json
go run ./website/gen -release /tmp/quosparr-release.json -out website/dist
python3 -m http.server 8765 -d website/dist
```

省略 `-release` 时按 `mygo.json` 的版本推算安装包文件名，不显示文件大小和发布日期，仅供离线预览。

## 部署

`.github/workflows/website.yml` 在以下情况构建并部署：

- 推送 `main` 且改动了 `website/`、`docs/images/` 或供应商图标
- `Release Draft` 工作流成功结束（稳定版公开后，下载链接随之更新到最新 Release）
- 手动运行

构建会读取最新的正式 Release；任一预期安装包缺失时构建失败，不会发布失效链接。

只改动 `website/` 或 `website.yml` 的推送与 PR 不触发 `ci.yml`，网站生成器的测试在本工作流中运行。

首次启用需要在仓库 **Settings → Pages** 中把 Source 设为 **GitHub Actions**，Custom domain 填写 `quosparr.com` 并开启 Enforce HTTPS；DNS 为 `quosparr.com` 配置指向 GitHub Pages 的 A / AAAA 记录（`www` 可用 CNAME 指向 `licoy.github.io`）。
