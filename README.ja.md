# Codie Music Player

CODE27 家族のネイティブなデスクトップ音楽プレーヤーです。このコンピュータにある音楽と、アプリに同梱されたソースの曲を再生します。アプリの中でも、ペアリングした codie デバイスでも再生できます。表示言語は英語、简体中国語、日本語です。

English: [README.md](README.md) · 简体中文: [README.zh-CN.md](README.zh-CN.md)

egoist の [mygo-music](https://github.com/egoist/mygo-music) の fork で、Go と [MyGo](https://mygo.egoist.dev) で書かれています。ネイティブな画面で、WebView は使いません。AGPL-3.0。[LICENSE](LICENSE) を参照。

## ソース

| ソース | 方式 | 内容 |
|---|---|---|
| このコンピュータ | 組み込み（`internal/local`） | 追加したフォルダ。MP3、FLAC、Ogg、ALAC、AAC、WAV に対応し、タグ、カバー、`.lrc` の歌詞も読みます。 |
| 网易云音楽 | プラグイン（`plugins/netease`） | 検索、ランキング、毎日のおすすめ。サインインすると自分のプレイリストとお気に入りも使えます。 |
| QQ Music | プラグイン（`plugins/qqmusic`） | 検索と自分のライブラリ。WeChat のスキャンでサインインします。 |
| Bilibili | プラグイン（`plugins/bilibili`） | 動画の音声を再生します。検索、ランキング、お気に入り、時間の付いた字幕を歌詞として表示します。 |
| YouTube Music | プラグイン（`plugins/youtube-music`） | ホーム、ランキング、新着、検索。サインインすると自分のライブラリも使えます。 |
| Spotify | プラグイン（`plugins/mygotify`） | Spotify のカタログと自分のライブラリを閲覧し、音声は YouTube Music から再生します。 |

初回起動時に、アプリに同梱されたプラグインが入ります。プラグインは普通の JavaScript ディレクトリ（`plugin.json` + `index.js`）で、アプリの plugins フォルダに置くとソースが増えます。

Bilibili、YouTube Music、Spotify は ffmpeg と yt-dlp を介して再生します。両方ともアプリに同梱され、プラットフォームごとに一つずつ入っているので、別にインストールする必要はありません。yt-dlp が YouTube の課題を解くには、引き続き JavaScript ランタイム（Deno、Node、Bun のいずれか）が必要です。

## codie デバイスで再生

1. 設定 → Codie を開き、デバイスのアドレス（`IP:ポート`）と、画面に表示される 6 桁のコードを入力します。
2. 「Codie デバイスで再生」をオンにします。以後に再生する曲はデバイスの TF カードに送られて再生され、デバイスのソースもそのカードに切り替わります。
3. デバイス側では音楽ページを開き、TF カードを挿し、このコンピュータと同じネットワークにつないでください。
4. カードにすでにある曲は、再度送らずすぐに再生します。一度に送れない大きな曲は分割して通して再生します。

## ビルド

```bash
go test ./internal/...
scripts/fetch-tools.sh darwin windows-amd64 linux-amd64 linux-arm64
go tool mygo build          # デスクトップアプリ。mygo.config.ts に従う
```

`scripts/fetch-tools.sh` は、各プラットフォームに同梱する ffmpeg と yt-dlp を `resources/<platform>/bin` にダウンロードします。ビルド時に取得するもので、ソースには含まれません。引数なしでは、実行しているマシンの分だけを取得します。

`go tool mygo build -platform darwin/arm64,windows/amd64,linux/amd64` で、一台のマシンから各プラットフォームをビルドできます。

## ウェブ版

同じソースをサーバーとしても動かせます。画面はブラウザに描かれます。デスクトップ版のウィンドウ（サイドバー、ホーム、検索、下部のプレーヤー）を HTTP 越しに出したもので、音はブラウザで鳴り、このマシンのスピーカーは使いません。独自のウィンドウでサインインするソースは、これまでどおりデスクトップ版でサインインします。両方を同じデータフォルダに向ければ、サインイン状態は共有されます。

```bash
cd front && npm install && npm run build && cd ..
rm -rf cmd/web/dist && cp -R front/dist cmd/web/dist
go run ./cmd/web -plugins ./plugins -data ./data   # http://127.0.0.1:8080
```

ログインは環境変数で決まります。どちらも未設定のときは、サーバーがパスワードを一つ作り、起動時に一度だけ表示します。

| 変数 | 内容 |
|---|---|
| `AUTH_USER` | ユーザー名。未設定なら `codie` |
| `AUTH_PASS` | パスワード。未設定なら自動で作り、ログに出します |
| `LISTEN_ADDR` | 待ち受けアドレス。既定は `:8080` |
| `DATA_DIR` | 設定とプラグインのデータ。既定は `data` |
| `PLUGINS_DIR` | プラグインのフォルダ。既定は `plugins` |

### Docker

イメージは `ghcr.io/codiebitcoin/codie-music-player` です。単体で一度公開し、その後のリリースでも毎回公開します。ffmpeg と yt-dlp はイメージに入っているので、別に入れるものはありません。

```bash
docker build -t codie-music-player:local .
cd deploy && cp env.example .env   # AUTH_USER と AUTH_PASS を設定
docker compose up -d
```

`deploy/docker-compose.yml` はポート 8080 で待ち受け、データは `deploy/data` に保存します。

## 注意

- プラグインは各サービス自身のウェブインターフェースを通して第三者のサービスに接続します。このプロジェクトは音楽を保存も配布もせず、NetEase、Tencent、Bilibili、Google、Spotify のいずれとも関係がありません。
- これらのインターフェースは予告なく変わるため、プラグインが更新されるまでソースが動かなくなることがあります。
