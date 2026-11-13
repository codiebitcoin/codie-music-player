Codie Music Player v0.1.25 — 播放链路修复 + 设备管理完善

## 本次修复与新增

- 🔧 **Bilibili 播放修复**：根因是设备端文件缓存滞后于上传 + 播放队列版本号不可从外部获知。
  改为 SELECT_TRACK 无队列方案 + 上传后重试覆盖扫描延迟，B 站/网易云/QQ 全部实测出声
- 🏷 **歌曲按标题命名**：上传到设备的文件改为 `歌名 - 歌手.m4a`，不再是无意义的 hash 名
- ⚡ **同歌秒传**：卡上已有同名文件直接复用，重播不重复下载上传
- 🗑 **清理入口**：播放器底栏音量按钮旁新增「清理 codie 歌曲」按钮（设置 → Codie 里也有），
  一键删除推送到设备的歌曲
- ✂️ **大文件自动分段**：超过单次上传上限的歌用 ffmpeg 无损切成 `_1 _2 _3` 连播，播放器侧无感
- 📢 **失败原因可读**：设备音乐页面没开 / TF 卡未就绪 / 设备离线 / 凭据过期，都给出中文可行动提示
- 🧭 首次启动向导（绑定设备 → 登录音源 → 尝试播放 → 完成）与四音源插件内置延续 v0.1.24

## 下载

| 文件 | 平台 |
|---|---|
| codie-music-player-0.1.25.dmg | macOS (Apple Silicon + Intel) |
| codie-music-player-setup-0.1.25-amd64.exe | Windows 安装器 |
| codie-music-player_0.1.25_amd64.deb | Debian / Ubuntu |
| codie-music-player-0.1.25-1.x86_64.rpm | Fedora / RHEL |
| codie-music-player-0.1.25-linux-amd64.tar.gz | 其他 Linux（含 install.sh）|
| USAGE.pdf | 使用指南 |

## 使用提示

codie 播放需要：设备插 TF 卡、设备上打开音乐页面、与电脑同一局域网。
AGPL-3.0 · 上游 https://github.com/egoist/mygo-music
