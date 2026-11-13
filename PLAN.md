# codie-music-player 项目计划

> 让 codie 设备播放来自 **网易云 / QQ音乐 / Spotify / Bilibili** 的音乐。
> 参考 [mygo-music](https://music.egoist.dev/)（egoist 的原生 Go 音乐播放器）的插件化音源思路，复用其插件生态，输出端接到 codie 设备。

---

## 1. 目标

- 一个 Go 单二进制桌面应用（mygo 框架，与 codie-life-connect 同技术栈）
- 跨平台搜歌 / 队列 / 歌单，一键推送到 codie 设备播放
- 音源全部通过 **mygo-music 插件协议兼容运行时** 接入（JS 插件，零改动复用现成插件）
- 私有仓库、个人使用

非目标（暂不做）：
- 本地播放（codie 是唯一输出设备）
- 移动端 / 多用户
- 音源插件本身的开发维护（只做兼容层，除非必要不 fork 插件）

## 2. 调研结论

### 2.1 参考项目：mygo-music（music.egoist.dev）

- Go + mygo 框架原生播放器，**AGPL-3.0**，无 WebView
- 音源 = 插件：`plugin.json`（声明权限域名）+ 单文件 `index.js`，从 GitHub 安装、每日检查更新
- 插件提供：搜索 / 流地址（`stream`）/ 歌词 / 登录（扫码、web cookie）/ 歌单 / 电台
- 插件协议有公开定义：[hubandad/mygo-music-netease](https://github.com/hubandad/mygo-music-netease) 仓库内的 `PLUGIN_API.md`

### 2.2 四个音源：全部有现成插件

| 平台 | 插件（首选） | 备注 |
|---|---|---|
| 网易云 | [hubandad/mygo-music-netease](https://github.com/hubandad/mygo-music-netease)（MIT） | 自有插件；扫码/web 登录；standard/exhigh/lossless → MP3/FLAC/OGG |
| QQ音乐 | [Lorpaves/mygo-music-qqmusic](https://github.com/Lorpaves/mygo-music-qqmusic) | 微信扫码登录、无损；备选 Amamiya23 / Guyu23 / dddaniel1 版 |
| Spotify | [xingkaixin/mygo-music-spotify](https://github.com/xingkaixin/mygo-music-spotify) | 备选 starriv（Connect/Soloist 播放）、sounfury（经 YT Music） |
| Bilibili | [GentleM4dman/mygo-music-bilibili](https://github.com/GentleM4dman/mygo-music-bilibili) | 搜索/排行榜/收藏/评论/扫码登录；音频为 m4a |

（"shopify" 按 **Spotify** 理解。）

### 2.3 codie 播放通道（clt01_embed_backend 1.6.0，已实测代码）

**通道 A：TF 卡音乐库（主路径）**
- `POST /v1/api/data/user/music/upload` — multipart `files[]` + `relative_paths` + `directory` + `conflict_policy`；单批 ≤200MiB、≤64 个文件；**扩展名白名单 `.mp3/.wav/.flac/.aac/.m4a/.ogg`**，单文件 ≤1GB，默认目录 `code27/music`
- `POST /v1/api/data/user/music/control` — 播放控制；`GET /music/library|playback|page|storage` 状态查询
- ✅ 格式全覆盖四平台（mp3/flac/ogg/m4a），**免转码**；持久化，设备端曲库化管理

**通道 B：临时媒体会话（备选）**
- `POST /v1/api/data/user/media/sessions` — multipart `file` + `request`(JSON)，202 异步受理；会话动作 `pause/resume/seek/set_volume/set_muted/set_layout`
- ⚠️ 音频仅收 **MP3/WAV ≤64MB**（其余格式需 ffmpeg 转码）；临时存储总预算 256MB、非持久、并发上传 ≤2
- 用途：无 TF 卡设备回退、试听、限时播放（`max_duration_ms`）

**鉴权（Phase 0 必须打通）**
- 上述接口均在 `/v1/api` 组的 `LocalAuthGuard()` 之下：签名校验（method + path + timestamp + nonce + payload mode + body hash），凭证经 `POST /v1/api/auth/credentials`（master credential）创建
- codie-life-connect 的 tool-integration WS 签名**不能**直接调这些接口，需独立的本地客户端凭证

**状态同步**
- 轮询 `GET /v1/api/data/user/music/playback`，或订阅 `GET /v1/ws/events`（后端有 music 事件流 `ws_events_music.go`）

## 3. 架构

```
┌────────────────────────────────────────────────┐
│ codie-music-player（Go 单二进制，mygo 桌面应用）    │
│                                                │
│  UI：搜索 / 平台切换 / 队列 / 正在播放 / 设备状态  │
│                                                │
│  Plugin Host：mygo-music 插件协议兼容运行时       │
│   （goja 跑 index.js + 权限域名拦截 + storage）   │
│   ├─ netease（hubandad）   ├─ qq（Lorpaves）     │
│   ├─ bilibili（GentleM4dman）└─ spotify（xingkaixin）│
│                                                │
│  Codie Connector                               │
│   ├─ LocalAuthGuard 请求签名                    │
│   ├─ 通道A：下载→TF卡上传→播放控制（主）           │
│   └─ 通道B：媒体会话 + ffmpeg 转码（备）           │
│                                                │
│  缓存层：本地缓存目录 + 内容寻址去重 + LRU 清理     │
└────────────────┬───────────────────────────────┘
                 │ signed HTTP / WS
                 ▼
        clt01_embed_backend（codie 设备）
```

### 播放流水线

1. 插件 `search()` → 选曲入队
2. 插件 `stream(id, quality)` → 拿直链（每次现取，防过期）
3. 下载到本地缓存（SHA-256 命名，命中则跳过）
4. 通道 A：`POST /music/upload`（`conflict_policy` 处理重复）→ `POST /music/control` 播放指定文件
5. 状态同步（轮询/WS）→ 曲目播完 → 队列推进（可预取下一首：下载+上传提前进行）
6. 后台任务：TF 卡同步清理（LRU / 手动"清空 codie 缓存目录"）

### 关键设计决策

- **不 fork mygo-music 宿主**（AGPL 传染），自研轻量宿主只实现插件协议——协议定义来自 MIT 仓库的 `PLUGIN_API.md`，干净
- **通道 A 优先**：免转码、格式白名单全覆盖；通道 B 只在无 TF 卡 / 试听场景启用（带 ffmpeg 依赖）
- **插件即子模块/第三方**：插件以"用户安装"方式拉取（GitHub repo / ZIP / 本地目录），与 mygo-music 生态兼容，不做硬编码
- 音质默认选 MP3 320k 档（上传小、兼容最好），无损档手动开启

## 4. 里程碑

| 阶段 | 内容 | 验收 |
|---|---|---|
| **M0 鉴权 Spike** | 摸清 LocalAuthGuard 签名算法与凭证创建流程；手动 curl 完成一次"上传 mp3 → control 播放"闭环 | 设备喇叭出声 |
| **M1 插件运行时** | goja 宿主 + 插件协议（search/stream/lyrics/login/storage/权限域名）；跑通 netease 插件搜索与取流 | 命令行能搜能下 |
| **M2 播放闭环** | Codie Connector 通道 A + 队列引擎 + 状态同步 | CLI `play <关键字>` 全自动播放 |
| **M3 桌面 UI** | mygo 界面：搜索/队列/正在播放/登录（扫码 UI）；接入 qq / bilibili / spotify 插件 | 四平台可搜可播 |
| **M4 打磨** | 缓存与 TF 卡清理策略、通道 B（媒体会话+转码）、WS 事件同步、开机自启、打包（NSIS/dmg） | 日常可用 |

## 5. 风险与待验证项

| # | 风险 | 缓解 |
|---|---|---|
| 1 | **LocalAuthGuard 凭证获取**：master credential 从哪来（配对流程？config 页签发？） | M0 第一件事：读 `local_auth.go` + `auth.VerifyLocalSignature`，必要时走 `/v1/api/auth/credentials` 签发 client credential |
| 2 | 设备端解码：白名单收 `.ogg/.flac/.m4a` ≠ 硬件解码器全支持（尤其 Spotify OGG） | M0/M2 用样例文件逐格式实测；不行则该平台降档 MP3 或通道 B 转码 |
| 3 | 插件协议版本兼容（宿主 v0.1.6 起） | 锁插件版本 + 协议版本探测；必要时给单插件打补丁 |
| 4 | QQ / Bilibili 登录态与音质（未登录仅试听） | 支持扫码登录 UI；默认 MP3 320k |
| 5 | 上传延迟影响切歌体验 | 预取下一首（下载+上传并行播放中）；本地缓存避免重复上传 |
| 6 | 平台条款（非官方接口） | 私有仓库、个人使用、不分发音频 |

## 6. 仓库布局（拟）

```
codie-music-player/
├── PLAN.md
├── main.go                 # mygo 应用入口
├── internal/
│   ├── pluginhost/         # goja 插件运行时 + 权限/存储
│   ├── sources/            # 插件管理（安装/更新/启用）
│   ├── codie/              # LocalAuthGuard 签名 + 通道A/B 客户端
│   ├── player/             # 队列/流水线/状态同步
│   └── cache/              # 内容寻址缓存 + 清理
├── ui/                     # mygo 前端
└── scripts/                # 构建/打包
```
