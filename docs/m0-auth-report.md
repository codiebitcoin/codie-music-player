# M0 Spike 结论：codie 本地鉴权 + 音乐 API 实测（2026-11-12）

设备：Hello（局域网地址，形如 192.168.1.20:8081），实测全通（除 TF 卡上传因设备未插卡暂缓）。

## 鉴权链路（全通 ✅）

1. `GET /v1/api/auth/bootstrap`（免签）→ 拿 `user_id`（签名必填）
2. `POST /v1/api/auth/pair/request`（免签）→ 设备屏幕弹 6 位配对码，返回 `challenge_id`（5 分钟有效；不带 Origin/Referer 的请求不受来源校验限制）
3. `POST /v1/api/auth/pair/confirm` `{pairing_code, challenge_id, auth_scope:"permanent"}` → 返回 **master secret**（`local_secret`）
4. `POST /v1/api/auth/credentials` `{"label":"codie-music-player"}`（master 签名）→ 持久 **client credential**（id + secret）
5. 之后所有请求用 client credential 签名（`X-Local-Session-Id` = credential id）

> client credential 权限：可调 `/data/user/*` 全部（含音乐/媒体），仅 `/admin` 与凭证管理不可用（只能吊销自己）。master secret 只在配对当次使用，不落盘。

## 签名算法（实测通过 ✅）

```
sig = HMAC-SHA256_hex(secret,
      "METHOD|path|timestamp|nonce|payloadMode|bodyHash|userId")
```

请求头：

| Header | 值 |
|---|---|
| `X-Local-Sig` | 上述签名（64 hex） |
| `X-Local-Nonce` | 随机 ≤128 字节（每请求唯一，防重放） |
| `X-Local-Timestamp` | Unix 秒（±300s） |
| `X-Local-Payload-Mode` | `empty` / `json` / `multipart` |
| `X-Local-Body-Hash` | body 的 SHA-256 hex；**multipart 固定 `UNSIGNED-PAYLOAD`**；GET/空 body 传空 body 的 SHA-256 也可 |
| `X-Local-User-Id` | bootstrap 拿到的 user_id |
| `X-Local-Session-Id` | client credential id（master 为空） |

注意：
- `path` 只取 URL path（**不含 query**），query 不参与签名
- 服务端 nonce 防重放：同一 nonce 5 分钟内不可复用（签名必须每次现算）
- `workshop_inner_sdk`（codie-life-connect 的 third_party SDK）已实现同款算法与配对流程，M2 可直接复用或参考

## 音乐 API 实测（设备固件现状）

| 接口 | 结果 |
|---|---|
| `GET /v1/api/data/user/music/storage` | ✅ 200（TF 卡未挂载时 `is_loaded:false`） |
| `GET /v1/api/data/user/music/page` | ✅ 200 |
| `GET /v1/api/data/user/music/playback` | ✅ 200（IDLE / volume 63/100 / play_mode sequence） |
| `POST /v1/api/data/user/music/upload` | ⛔ 503 `TF_CARD_NOT_READY`（设备未插卡，管道本身已验证可达） |
| `GET /v1/api/data/user/media/capabilities` | ⛔ 404——**设备固件未包含 1.6.0 媒体会话路由**（升级固件后可用） |

上传格式（multipart form 字段）：`files[]`（多文件）+ `directory`（默认 `code27/music`）+ `conflict_policy`；单批 ≤200MiB / ≤64 文件；扩展名 `.mp3/.wav/.flac/.aac/.m4a/.ogg` ≤1GB。响应 `uploaded[].name` 即曲目标识。

播放控制 `POST /music/control`：`{request_id, action, ...}`，action ∈ `PLAY/PAUSE/STOP/NEXT/PREVIOUS/SELECT_TRACK/SEEK/SET_VOLUME/SET_PLAY_MODE/SET_SOUND_EFFECT/EDIT_QUEUE/QUERY_STATE`；`SELECT_TRACK` 带 `track_id`。

## TF 卡检测机制

- Android 宿主上报挂载事件（`ExternalStorageCommand.IsLoaded`），Go 后端记录并扫描 `root_path`
- 卡不被识别的排查：插到位 / 格式化 exFAT 或 FAT32 / 重启设备
- 卡就绪后动作：`m0 upload <file>` → `m0 library`（看 track_id 形态）→ `m0 play <id>` → `m0 playback`

## 结论对 PLAN 的影响

1. **M0 鉴权风险消除**：配对+签名+client credential 全链路可用，spike 代码在 `m0/main.go`
2. 通道 A（TF 卡）依赖设备插卡；通道 B（媒体会话）依赖固件升级到 1.6.0——两条通道的可用性都取决于设备状态，架构上保持双通道设计不变
3. 设备地址/端口约定：`http://<ip>:8081`（codie-life-connect 同款默认端口）

---

# 附录：播放闭环实测（2026-11-13，全通 ✅）

设备重启后 IP 会变（DHCP），TF 卡挂载 `/storage/XXXX-XXXX`（卷 UUID，119MB 小卡）。
`m0 upload` → `m0 playq` 全链路打通，设备喇叭实际出声，`PLAYING` 状态进度实时推进（position_ms 1.2s→7.4s）。

## 播放五要素（缺一不可，都是实测踩出来的）

1. **音乐页面开启**（设备屏幕上手动打开；HTTP 无远程开启接口，关闭时播放类 action 一律 409 `MODE_INACTIVE`）
2. **音源 = `LOCAL_TF`**：`GET /music/source` 查看当前值。设备端开音乐页时 Android 可能把音源切到 `BLUETOOTH`（origin:ANDROID），此时 TF 播放命令**被静默忽略**（200 accepted 但 music_session_version 不动）。先 `POST /music/source {"source":"LOCAL_TF"}` 再播
3. **track_id 格式 = `local:` + TF 卡完整路径**，如 `local:/storage/XXXX-XXXX/code27/music/x.m4a`（裸文件名 → `TRACK_NOT_FOUND`）
4. **播放 = `PLAY` + 内联 queue**（不是 SELECT_TRACK 裸 track_id——那只查 Android 当前队列）：
   ```json
   POST /v1/api/data/user/music/control
   {
     "action": "PLAY", "source": "LOCAL_TF", "request_id": "...",
     "expected_queue_revision": 0,
     "queue_entry_id": "<entry uuid>",
     "queue": {
       "storage_root": "/storage/XXXX-XXXX", "title": "...", "auto_advance": true, "revision": 0,
       "entries": [{"id": "<uuid>", "track_id": "local:/storage/.../x.m4a"}],
       "current_entry_id": "<entry uuid>"
     }
   }
   ```
   字段名 **snake_case**（与设备自带音乐页面的行为一致）。新建队列 `expected_queue_revision=0`。
5. **上传 multipart 字段名是 `files` 不是 `files[]`**（Go 原生 multipart 不剥 `[]`，后端取 `form.File["files"]`）

## 其他实测结论

- 上传响应 `uploaded[]{name,size_bytes,path,duration_ms}`，后端自动解析音频时长
- 播放命令全部异步：HTTP 200 只代表已入队下发，真实结果看 `GET /music/playback`（`status/position_ms/applied/error_code`，`music_session_version` 跳变 = Android 真正处理了）
- 当前固件无 `/music/library`、`/media/*`（1.6.0 分支的路由，404）；固件落后于 1.6.0 dev 分支
- 设备重启会换 IP（DHCP），需按 user_id 扫描发现（同一子网可能有多台 codie 设备，不能按 IP 认）
- 参考实现：设备自带音乐页面的行为（队列构造 / 确认状态机 + `/v1/ws/events` 的 `music_playback` 事件）
