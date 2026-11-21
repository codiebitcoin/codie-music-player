# 发布到 CodieHub Workshop

把版本发布到 [codiehub.com](https://codiehub.com) 的 Workshop（CODE27 的应用商店）。
纯手动流程，浏览器里点；配合 ZCode 内置浏览器可以半自动完成。
先按 [RELEASING.md](../RELEASING.md) 把 GitHub release 发好、产物齐备，再走本文。

## 前置条件

- CodieHub 已登录（内置浏览器里登录一次，session 会保留；未登录点 Upload 会弹 "Log in to continue"）
- 四平台产物齐：`~/Desktop/Software/CodieMusicPlayer/版本/<版本号>/`
- 一个 16:9 左右的封面图（PNG，≤5MB）+ 3~5 张应用截图（PNG，每张 ≤5MB）

## 一、打包两个 zip

所有要上传的素材统一放 `~/Desktop/Software/Upload/`：产物 zip、源码 zip、封面 `cover.png`、
`snapshot-01-*.png` 等应用截图。

```bash
mkdir -p ~/Desktop/Software/Upload
# 1) 产物包：所有平台安装器打成一个
cd ~/Desktop/Software/CodieMusicPlayer/版本/<版本号>
zip -qr ~/Desktop/Software/Upload/codie-music-player-<版本号>-resources.zip . -x "*.DS_Store"
# 2) 源码包：git archive 自动排除 .git 历史和未跟踪的构建垃圾
cd ~/Project/github/codie-music-player
git archive HEAD --prefix=codie-music-player-<版本号>/ | tar -x -C /tmp/source-scan
# 扫描敏感信息后再压
grep -rInE "(api[_-]?key|secret|password|token)\s*[:=]\s*[\"'][A-Za-z0-9+/_-]{16,}[\"']" /tmp/source-scan -include="*.go" -include="*.ts" -include="*.md"
grep -rIl "/Users/<你的用户名>" /tmp/source-scan   # 命中的只有 CHANGELOG 的公开仓库链接就没事
cd /tmp/source-scan && zip -qr ~/Desktop/Software/Upload/codie-music-player-<版本号>-source.zip .
```

0.1.26 实测：产物包 79M，源码包 6.7M（294 个跟踪文件）。

## 二、首次发布

入口：顶栏 **Upload**（下拉触发器，`data-testid="nav-upload-entry-trigger"`）→ **Software**。
注意直接访问 `/en/upload` 会落到 Model/Personas/Artwork/voice 那个作品类型页，Software 的表单在 `/en/workshop/upload`。

### Step 1 · Upload File

| 字段 | 填法 |
| --- | --- |
| Distribution | Include source code |
| Resource package | 产物 zip |
| Source code package | 源码 zip |
| Compatible product | Code27 H1（默认） |
| Compatible software version | v1.6.0（默认） |
| Supported/Target platforms | **在输入框敲 `Windows`、`macOS`、`Linux` 后回车**，两组都要 |

> ⚠️ 平台标签的建议芯片有 bug：按钮上写着 Windows/macOS/Linux，实际存进去的却是
> 随机日文标签（Vライバー/海/アンティーク…）。**永远别点芯片，只用手打回车。**
> 手打的值必须在同一挂载页面里一次做完，别刷新。

### Step 2 · Fill Content

| 字段 | 填法 |
| --- | --- |
| 封面 | 上传后弹 16:9 裁剪框，摆好点 Confirm |
| Software name | `Codie Music Player`（≤20 字符） |
| Description | 用 **Import document** 直接导 `.md` 文件（h2/h3 会渲染成标题并自动生成 Contents 目录，参考 GameBuddy 的页面）。注意含 GitHub 仓库链接（Workshop 条款 4.2 要求注明源码可得性） |
| Software tags | 恰好 4 个：Media & Entertainment / Music / Music Player / Streaming |
| video / showcase | 可选；showcase 每张图上传后各弹一次 16:9 裁剪框，逐个 Confirm |

> ⚠️ **换封面会把旧封面图降级进 showcase 列表**。点 showcase 缩略图上的编辑按钮 →
> 对话框里 **Delete image** 删掉它。

最后：**Confirm Publish** → 弹 CODE27 Workshop Terms（首次发布才弹）→ **Agree & Upload**
→ "Published successfully"。产物包大（79M）时要等它传完。

### 发布后核对

作品详情页检查：封面是新图、描述有 Contents 目录、TAG 4/4、
Supported dev platforms 是 `Windows / macOS / Linux`、showcase 是预期的截图、
两个下载按钮（mod / source code）大小正常。

## 三、更新已发布的作品

作品页右上 **Edit** → `/en/workshop/upload?edit_work_id=<作品id>&work_type=software`。

- 草稿在服务器端自动保存，进页面后自动恢复（zip 文件名、标签都在）。
- "Loading unfinished information..." 卡住超过十几秒 → **刷新一次**就好。
- 站点自动保存有防抖，别指望中途状态：**所有改动在同一个页面里一次做完，
  最后 Confirm Publish 一次性提交**。改完立刻刷新可能丢刚改的（会回滚到服务器快照）。
- 其余字段同 Step 2；已发布的 zip 不用重传。

## 四、用 ZCode 内置浏览器自动化时的要点

- **IAB 驱动不了原生文件选择框**。绕法：本地起一个仅回环的 HTTP 服务
  （响应头带 CORS + `Access-Control-Allow-Private-Network: true`，Chromium 允许
  HTTPS 页面 fetch `127.0.0.1`），然后在页面里
  `fetch → new File([buf]) → DataTransfer → input.files 赋值 → dispatch change`。
  表单的槽位都是隐藏 `input[type=file]`，按 `data-testid` 定位最稳
  （`work-upload-home-cover-input`、`work-upload-description-import-input`、
  `work-upload-showcase-images-input`、`*-platforms-input`）。
- **站点按钮会卡 Playwright 的 actionability 检查**（Upload、芯片、Confirm 全超时）。
  按钮一律走 `dom_cua.get_visible_dom()` 拿 ref 再 `dom_cua.click({node_id})`；
  文本框 `locator.fill` 正常。
- Ref 会过期（页面重渲染后失效），每次点击前重新 get_visible_dom。
- 内置浏览器标签页在轮次结束会被释放回用户侧，下一轮先
  `browser.user.openTabs()` 找回再 `claimTab`。

## 五、坑

- **Cloudflare 1015 限流**：短时间反复刷新页面会触发，之后所有 API 返回 429，
  `Retry-After` 实测 2887 秒（48 分钟），同 IP 手动操作一样被拦，编辑页只会显示
  空表单（连 Loading 提示都没有）。限流期间只能干等；平时**别循环刷新**，
  草稿没出来就隔几分钟试一次，一次会话做完所有事。
- 平台芯片存错值（见 Step 1 的警告）。
- 修标签/删图等小改动会被自动保存的中间快照覆盖：以最后一次 Confirm Publish
  的客户端状态为准。

## 六、清理

发布完删掉临时目录：`rm -rf /tmp/source-scan`，以及从 dmg 拷出来做截图的测试 App
（先退出再删）。`~/Desktop/Software/Upload/` 里的素材保留归档，下次更新直接复用。
