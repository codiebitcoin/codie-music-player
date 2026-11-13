# Codie Music Player

[mygo-music](https://music.egoist.dev/)（by egoist）的 fork：完整的原生音乐播放器，
四音源插件（网易云 / QQ 音乐 / Bilibili / Spotify），外加 **codie 输出** ——
任何流媒体音源的歌曲都可以推到 codie 设备（TF 卡 + 音乐 API）播放。

## Fork 改动

- `internal/codieout`：codie 设备客户端（LocalAuthGuard 签名、配对、TF 卡上传、
  PLAY+queue，契约见 `docs/m0-auth-report.md`）+ `Sink`（把流媒体 Source 包装成
  PlaybackRemote，复用 mygo-music 的 remote 播放引擎）
- `internal/player`：`Player.Codie` 钩子 —— 流媒体源在 codie 开启时走 remote 通道
- 设置新增 **Codie** 区块：开关、配对（6 位码 → client credential）、状态测试
- `mygo.config.ts`：改名 Codie Music Player / com.codie.musicplayer，移除自动更新与签名

使用：设置 → Codie → 配对设备（IP:端口 + 设备上的 6 位码）→ 打开「在 Codie 设备上播放」。
设备侧需打开音乐页面并插好 TF 卡；音源会自动切到 LOCAL_TF。

## 原项目

AGPL-3.0，见 [LICENSE](LICENSE)。上游：https://github.com/egoist/mygo-music
