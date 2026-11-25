# Codie Music Player

A native desktop music player for the CODE27 family. It plays the music on this computer, and songs from the sources it ships with, in the app or on a paired codie device. The interface is in English, 简体中文 and 日本語.

简体中文: [README.zh-CN.md](README.zh-CN.md) · 日本語: [README.ja.md](README.ja.md)

This is a fork of [mygo-music](https://github.com/egoist/mygo-music) by egoist. It is written in Go with [MyGo](https://mygo.egoist.dev), with a native interface and no WebView. AGPL-3.0, see [LICENSE](LICENSE).

## Sources

| Source | How | What it plays |
|---|---|---|
| This computer | Built in (`internal/local`) | Folders you add. MP3, FLAC, Ogg, ALAC, AAC and WAV, with their tags, covers and `.lrc` lyrics. |
| NetEase Cloud Music | Plugin (`plugins/netease`) | Search, charts, daily picks, your playlists and liked songs once you sign in. |
| QQ Music | Plugin (`plugins/qqmusic`) | Search and your library, signed in by scanning with WeChat. |
| Bilibili | Plugin (`plugins/bilibili`) | The audio of videos: search, charts, favorites and timed subtitles as lyrics. |
| YouTube Music | Plugin (`plugins/youtube-music`) | Home, charts, new releases, search, and your library once you sign in. |
| Spotify | Plugin (`plugins/mygotify`) | Spotify's catalog and your library, with the audio played from YouTube Music. |

The first run installs the plugins the app ships. A plugin is an ordinary directory of JavaScript (`plugin.json` + `index.js`). Drop one in the app's plugins folder to add another source.

Bilibili, YouTube Music and Spotify play through ffmpeg and yt-dlp. Both ship inside the app, one copy for each platform, so nothing has to be installed first. yt-dlp still needs a JavaScript runtime (Deno, Node or Bun) for YouTube's challenges.

## Play on a codie device

1. Open Settings → Codie, fill in the device's address (`IP:port`) and the 6-digit code its screen shows.
2. Turn on Play on the Codie device. Whatever you play is sent to the device's TF card and played there, and the device switches its source to that card.
3. The device needs its music page open and a TF card in it, on the same network as this computer.
4. A song already on the card plays at once, without uploading again. A song too large to upload in one go is split and played through.

## Build

```bash
go test ./internal/...
scripts/fetch-tools.sh darwin windows-amd64 linux-amd64 linux-arm64
go tool mygo build          # the desktop app, from mygo.config.ts
```

`scripts/fetch-tools.sh` downloads the ffmpeg and yt-dlp each platform ships into `resources/<platform>/bin`. They are fetched at build time and are not part of the source. With no argument it fetches the copy for the machine it runs on.

`go tool mygo build -platform darwin/arm64,windows/amd64,linux/amd64` builds every platform from one machine.

## Web app

The same sources run as a server, with the pages drawn in the browser. It is the desktop's window — sidebar, Home, search and the player along the foot — over HTTP, and it plays in the browser rather than through the machine's own speakers. Signing in to a source that opens its own window still happens in the desktop app; a session saved there is the server's too, since they share a plugin's data when pointed at the same folder.

```bash
cd front && npm install && npm run build && cd ..
rm -rf cmd/web/dist && cp -R front/dist cmd/web/dist
go run ./cmd/web -plugins ./plugins -data ./data   # http://127.0.0.1:8080
```

The login comes from the environment. With neither set, the server makes a password and prints it once as it starts.

| Variable | What it sets |
|---|---|
| `AUTH_USER` | the username, `codie` when unset |
| `AUTH_PASS` | the password; unset, one is made up and logged |
| `LISTEN_ADDR` | where it listens, `:8080` by default |
| `DATA_DIR` | settings and the plugins' data, `data` by default |
| `PLUGINS_DIR` | the plugin folders, `plugins` by default |

### Docker

The image is `ghcr.io/codiebitcoin/codie-music-player`, published on its own and again with each release. ffmpeg and yt-dlp are in the image, so nothing is installed beside it.

```bash
docker build -t codie-music-player:local .
cd deploy && cp env.example .env   # set AUTH_USER and AUTH_PASS
docker compose up -d
```

`deploy/docker-compose.yml` listens on port 8080 and keeps the server's data in `deploy/data`.

## Notes

- The plugins talk to third-party services through those services' own web interfaces. This project stores none of their music and redistributes none of it, and it isn't affiliated with NetEase, Tencent, Bilibili, Google or Spotify.
- Those interfaces change without notice, so a source can stop working until its plugin is updated.
