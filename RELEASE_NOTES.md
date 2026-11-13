Codie Music Player v0.1.24 — 在 codie 设备上播放全平台音乐

基于 mygo-music（by egoist）的完整原生播放器 fork，四个音源开箱即用：
网易云音乐 / QQ 音乐 / Bilibili / MyGotify(Spotify 曲库，经 YouTube Music 出流)。

## 本版特性

- 🎛 codie 输出：设置 → Codie 绑定设备（输入 IP 即可，端口默认 8081，
  设备屏 6 位码配对），任何流媒体歌曲一键推到 codie 播放
- 🧭 首次启动向导：绑定机器 → 登录音源 → 尝试播放 → 完成
- 📦 四音源插件内置，首次启动自动安装
- 🖥 猫头 + 播放徽标应用图标

## 下载

| 文件 | 平台 |
|---|---|
| codie-music-player-0.1.24-universal.dmg | macOS (Apple Silicon + Intel) |
| codie-music-player-setup-0.1.24-amd64.exe | Windows 安装器 |
| codie-music-player-0.1.24-linux-amd64.deb | Debian / Ubuntu |
| codie-music-player-0.1.24-linux-amd64.rpm | Fedora / RHEL |
| codie-music-player-0.1.24-linux-amd64.tar.gz | 其他 Linux（含 install.sh）|

## 使用提示

- codie 播放需要：设备插 TF 卡、打开设备上的音乐页面；音源会自动切到 LOCAL_TF
- MyGotify 免登录即可搜索播放；网易云/QQ/B站 用各自 App 扫码登录解锁会员曲库与个人歌单
- AGPL-3.0，上游 https://github.com/egoist/mygo-music
