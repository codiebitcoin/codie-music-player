# Codie Music Player

CODE27 家族的原生桌面音乐播放器。播放这台电脑上的音乐，以及它随带的音源里的歌——在应用里，或者推到配对的 codie 设备上播放。界面有英文、简体中文和日文。

English: [README.md](README.md) · 日本語: [README.ja.md](README.ja.md)

本项目是 egoist 的 [mygo-music](https://github.com/egoist/mygo-music) 的 fork，用 Go 与 [MyGo](https://mygo.egoist.dev) 写成，原生界面，没有 WebView。遵循 AGPL-3.0，见 [LICENSE](LICENSE)。

## 音源

| 音源 | 方式 | 内容 |
|---|---|---|
| 本机 | 内置（`internal/local`） | 添加本机文件夹。支持 MP3、FLAC、Ogg、ALAC、AAC 与 WAV，连同标签、封面和 `.lrc` 歌词。 |
| 网易云音乐 | 插件（`plugins/netease`） | 搜索、榜单、每日推荐；登录后还有自己的歌单和喜欢的歌。 |
| QQ 音乐 | 插件（`plugins/qqmusic`） | 搜索和自己的音乐库，用微信扫码登录。 |
| Bilibili | 插件（`plugins/bilibili`） | 播放视频的音轨：搜索、榜单、收藏夹，带时间的字幕当歌词。 |
| YouTube Music | 插件（`plugins/youtube-music`） | 首页、榜单、新上架、搜索；登录后还有自己的音乐库。 |
| Spotify | 插件（`plugins/mygotify`） | 浏览 Spotify 的曲库和自己的音乐库，音频从 YouTube Music 播放。 |

首次运行会装上应用随带的插件。插件是普通的 JavaScript 目录（`plugin.json` + `index.js`），放进应用的 plugins 文件夹就多一个音源。

Bilibili、YouTube Music 和 Spotify 要经过 ffmpeg 和 yt-dlp 才能播放。两者都打进了应用，每个平台一份，不用另外安装。yt-dlp 解 YouTube 的校验仍然需要一个 JavaScript 运行时（Deno、Node 或 Bun）。

## 推到 codie 设备

1. 打开设置 → Codie，填设备地址（`IP:端口`）和设备屏幕上的 6 位配对码。
2. 打开「在 Codie 设备上播放」。之后播放的歌都会传到设备的 TF 卡上播放，设备也会把音源切到这张卡。
3. 设备需要打开音乐页面、插好 TF 卡，并且和这台电脑在同一个局域网。
4. 卡上已经有的歌直接播放，不再上传一次。一次传不完的大文件会自动切开连着播。

## 构建

```bash
go test ./internal/...
scripts/fetch-tools.sh darwin windows-amd64 linux-amd64 linux-arm64
go tool mygo build          # 桌面应用，按 mygo.config.ts
```

`scripts/fetch-tools.sh` 把各个平台随包的 ffmpeg 和 yt-dlp 下到 `resources/<platform>/bin`。它们在构建时下载，不进源码。不带参数时，只下当前这台机器的那一份。

`go tool mygo build -platform darwin/arm64,windows/amd64,linux/amd64` 在一台机器上打出各个平台。

## 说明

- 插件通过各服务自己的网页接口访问第三方服务。本项目不存储、不分发它们的音乐，也与网易、腾讯、哔哩哔哩、Google 或 Spotify 无关。
- 这些接口会不打招呼地改变，某个音源可能会失效，直到它的插件更新。
