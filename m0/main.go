// M0 spike：验证 codie 本地鉴权 + TF 卡音乐上传/播放闭环。
//
// 用法：
//
//	m0 pair-request              # 设备屏幕弹出 6 位配对码
//	m0 pair-confirm <code>       # 换取 master secret，并签发 codie-music-player 客户端凭据
//	m0 status                    # 签名请求：音乐库/存储/播放状态 + 媒体会话能力
//	m0 library                   # 列出 TF 卡曲库（看 track_id 形态）
//	m0 upload <file>             # multipart 上传到 TF 卡 directory=codie-music-player
//	m0 play <track_id>           # SELECT_TRACK 播放
//	m0 playback                  # 查询播放状态
//	m0 stop                      # 停止
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	addr        = "http://192.168.1.160:8081"
	stateFile   = ".m0-state.json"
	credsFile   = ".m0-creds.json"
	uploadDir   = "codie-music-player"
	credLabel   = "codie-music-player"
	musicPrefix = "/v1/api/data/user/music"
)

// state 保存配对挑战。
type state struct {
	ChallengeID string `json:"challenge_id"`
}

// creds 保存可长期使用的凭据（优先客户端凭据，其次 master secret）。
type creds struct {
	Secret    string `json:"secret"`     // 签名密钥（客户端凭据 secret 或 master secret）
	SessionID string `json:"session_id"` // 客户端凭据 ID；master 为空
	UserID    string `json:"user_id"`
	Kind      string `json:"kind"` // client | master
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: m0 <pair-request|pair-confirm|status|library|upload|play|playback|stop> [args]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "pair-request":
		err = doPairRequest()
	case "pair-confirm":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: m0 pair-confirm <6位配对码>")
			os.Exit(2)
		}
		err = doPairConfirm(os.Args[2])
	case "status":
		err = doStatus()
	case "sys":
		_, err = signedJSON("GET", "/v1/api/data/user/system", nil, "")
		return
	case "library":
		err = doLibrary()
	case "playq":
		// PLAY + queue 构造（复刻设备 AI 的 playMusicResource 流程）
		// usage: m0 playq <local:/storage/.../x.m4a> [more ids...]
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: m0 playq <track_id...>")
			os.Exit(2)
		}
		err = doPlayQueue(os.Args[2:])
	case "upload":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: m0 upload <file>")
			os.Exit(2)
		}
		err = doUpload(os.Args[2])
	case "play":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: m0 play <track_id>")
			os.Exit(2)
		}
		err = doControl("SELECT_TRACK", map[string]any{"track_id": os.Args[2]})
	case "playback":
		_, err = signedJSON("GET", musicPrefix+"/playback", nil, "")
		return
	case "qs":
		err = doControl("QUERY_STATE", map[string]any{"source": "LOCAL_TF"})
	case "get":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: m0 get <path>")
			os.Exit(2)
		}
		_, err = signedJSON("GET", os.Args[2], nil, "")
		return
	case "post":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: m0 post <path> <json>")
			os.Exit(2)
		}
		_, err = signedJSON("POST", os.Args[2], []byte(os.Args[3]), "application/json")
		return
	case "stop":
		err = doControl("STOP", nil)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

// ---- 无签名请求（配对流程） ----

func doPairRequest() error {
	code := 0
	body, _ := json.Marshal(map[string]any{})
	resp, raw, err := call("POST", "/v1/api/auth/pair/request", body, "application/json")
	if err != nil {
		return err
	}
	code = resp.StatusCode
	if resp.StatusCode != 200 {
		return fmt.Errorf("pair/request HTTP %d: %s", code, trunc(raw))
	}
	var r struct {
		Success bool `json:"success"`
		Data    struct {
			ChallengeID string `json:"challenge_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &r); err != nil || !r.Success {
		return fmt.Errorf("pair/request bad response: %s", trunc(raw))
	}
	if err := writeJSON(stateFile, state{ChallengeID: r.Data.ChallengeID}); err != nil {
		return err
	}
	fmt.Println("配对窗口已在设备弹出，challenge_id:", r.Data.ChallengeID)
	fmt.Println("请看设备屏幕上的 6 位配对码，然后执行: m0 pair-confirm <code>")
	return nil
}

func doPairConfirm(code string) error {
	var st state
	if err := readJSON(stateFile, &st); err != nil {
		return fmt.Errorf("先执行 m0 pair-request: %w", err)
	}
	body, _ := json.Marshal(map[string]string{
		"pairing_code": code,
		"challenge_id": st.ChallengeID,
		"auth_scope":   "permanent",
	})
	resp, raw, err := call("POST", "/v1/api/auth/pair/confirm", body, "application/json")
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("pair/confirm HTTP %d: %s", resp.StatusCode, trunc(raw))
	}
	var pr struct {
		Data struct {
			LocalSecret string `json:"local_secret"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &pr); err != nil || pr.Data.LocalSecret == "" {
		return fmt.Errorf("pair/confirm 未返回 local_secret: %s", trunc(raw))
	}

	// user_id 来自免签 bootstrap
	userID := ""
	if resp, raw, err = call("GET", "/v1/api/auth/bootstrap", nil, ""); err == nil && resp.StatusCode == 200 {
		var b struct {
			Data struct {
				UserID string `json:"user_id"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &b) == nil {
			userID = b.Data.UserID
		}
	}
	if userID == "" {
		return fmt.Errorf("bootstrap 未返回 user_id（设备未登录？）")
	}

	// 用 master secret 签发持久客户端凭据
	c := &creds{Secret: pr.Data.LocalSecret, UserID: userID, Kind: "master"}
	body, _ = json.Marshal(map[string]string{"label": credLabel})
	resp, raw, err = signedCall(c, "POST", "/v1/api/auth/credentials", body, "application/json")
	if err != nil {
		return err
	}
	if resp.StatusCode == 200 {
		var cr struct {
			Data struct {
				ID     string `json:"id"`
				Secret string `json:"secret"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &cr) == nil && cr.Data.ID != "" {
			c = &creds{Secret: cr.Data.Secret, SessionID: cr.Data.ID, UserID: userID, Kind: "client"}
			fmt.Printf("已签发客户端凭据: id=%s label=%s\n", cr.Data.ID, credLabel)
		}
	} else {
		fmt.Printf("签发客户端凭据失败（退回 master secret）: HTTP %d %s\n", resp.StatusCode, trunc(raw))
	}
	if err := writeJSON(credsFile, c); err != nil {
		return err
	}
	fmt.Printf("凭据已保存（kind=%s），做一次签名验证…\n", c.Kind)

	// 验证：音乐存储状态 + 媒体能力
	for _, path := range []string{musicPrefix + "/storage", "/v1/api/data/user/media/capabilities"} {
		resp, raw, err := signedCall(c, "GET", path, nil, "")
		if err != nil {
			return err
		}
		fmt.Printf("GET %s -> %d %s\n", path, resp.StatusCode, trunc(raw))
	}
	return nil
}

// ---- 签名请求（音乐闭环） ----

func doStatus() error {
	c, err := loadCreds()
	if err != nil {
		return err
	}
	for _, path := range []string{musicPrefix + "/storage", musicPrefix + "/page", musicPrefix + "/playback"} {
		resp, raw, err := signedCall(c, "GET", path, nil, "")
		if err != nil {
			return err
		}
		fmt.Printf("GET %s -> %d %s\n", path, resp.StatusCode, trunc(raw))
	}
	return nil
}

func doLibrary() error {
	c, err := loadCreds()
	if err != nil {
		return err
	}
	resp, raw, err := signedCall(c, "GET", musicPrefix+"/library", nil, "")
	if err != nil {
		return err
	}
	fmt.Printf("HTTP %d\n%s\n", resp.StatusCode, trunc2k(raw))
	return nil
}

func doUpload(file string) error {
	c, err := loadCreds()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	// 手工构造 multipart：files（后端读 form.File["files"]，注意不带 []）+ directory + conflict_policy
	var buf bytes.Buffer
	boundary := "m0" + strconv.FormatInt(time.Now().UnixNano(), 10)
	mw := func(name string) {
		buf.WriteString("--" + boundary + "\r\nContent-Disposition: form-data; name=\"" + name + "\"\r\n\r\n")
	}
	mw("directory")
	buf.WriteString(uploadDir + "\r\n")
	mw("conflict_policy")
	buf.WriteString("overwrite\r\n")
	buf.WriteString("--" + boundary + "\r\nContent-Disposition: form-data; name=\"files\"; filename=\"" + filepath.Base(file) + "\"\r\nContent-Type: application/octet-stream\r\n\r\n")
	buf.Write(data)
	buf.WriteString("\r\n--" + boundary + "--\r\n")

	resp, raw, err := signedCall(c, "POST", musicPrefix+"/upload", buf.Bytes(), "multipart/form-data; boundary="+boundary)
	if err != nil {
		return err
	}
	fmt.Printf("HTTP %d\n%s\n", resp.StatusCode, trunc2k(raw))
	return nil
}

func doControl(action string, extra map[string]any) error {
	c, err := loadCreds()
	if err != nil {
		return err
	}
	payload := map[string]any{"action": action, "request_id": fmt.Sprintf("m0-%d", time.Now().UnixNano())}
	for k, v := range extra {
		payload[k] = v
	}
	body, _ := json.Marshal(payload)
	resp, raw, err := signedCall(c, "POST", musicPrefix+"/control", body, "application/json")
	if err != nil {
		return err
	}
	fmt.Printf("HTTP %d\n%s\n", resp.StatusCode, trunc2k(raw))
	return nil
}

// doPlayQueue 走 HTTP 契约的 PLAY + 内联 queue 播放：
// 先 GET /music/storage 拿 storage_root，再把曲目作为队列条目直接下发。
func doPlayQueue(trackIDs []string) error {
	c, err := loadCreds()
	if err != nil {
		return err
	}
	resp, raw, err := signedCall(c, "GET", musicPrefix+"/storage", nil, "")
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("storage HTTP %d: %s", resp.StatusCode, trunc(raw))
	}
	var st struct {
		Data struct {
			IsLoaded  bool   `json:"is_loaded"`
			RootPath  string `json:"root_path"`
			RootFiles []struct {
				Name string `json:"name"`
				Path string `json:"path"`
			} `json:"music_files"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &st); err != nil || !st.Data.IsLoaded {
		return fmt.Errorf("TF 卡未就绪: %s", trunc(raw))
	}
	available := map[string]bool{}
	byName := map[string]string{} // 文件名 -> local:/完整路径
	for _, f := range st.Data.RootFiles {
		available["local:"+f.Path] = true
		byName[f.Name] = "local:" + f.Path
	}
	entries := make([]map[string]any, 0, len(trackIDs))
	for _, id := range trackIDs {
		if !strings.HasPrefix(id, "local:/") { // 允许只传文件名
			if p, ok := byName[id]; ok {
				id = p
			} else {
				return fmt.Errorf("曲目不在 TF 卡上: %s", id)
			}
		}
		if !available[id] {
			return fmt.Errorf("曲目不在 TF 卡上: %s", id)
		}
		entries = append(entries, map[string]any{"id": randHex(16), "track_id": id})
	}
	first := entries[0]["id"].(string)
	rev := 0 // 无现存队列时 revision 为 0（与前端空队列默认一致）
	payload := map[string]any{
		"action": "PLAY", "source": "LOCAL_TF",
		"request_id":            fmt.Sprintf("m0-%d", time.Now().UnixNano()),
		"expected_queue_revision": rev,
		"queue_entry_id":          first,
		// 字段名与设备自带音乐页面完全一致（snake_case）
		"queue": map[string]any{
			"storage_root": st.Data.RootPath, "title": "codie-music-player", "auto_advance": true, "revision": rev,
			"entries": entries, "current_entry_id": first,
		},
	}
	body, _ := json.Marshal(payload)
	resp, raw, err = signedCall(c, "POST", musicPrefix+"/control", body, "application/json")
	if err != nil {
		return err
	}
	fmt.Printf("PLAY HTTP %d %s\n", resp.StatusCode, trunc(raw))
	for i := 0; i < 4; i++ {
		time.Sleep(2 * time.Second)
		resp, raw, err = signedCall(c, "GET", musicPrefix+"/playback", nil, "")
		if err != nil {
			return err
		}
		fmt.Printf("[%ds] %s\n", (i+1)*2, strings.TrimSpace(string(raw)))
		if resp.StatusCode != 200 {
			break
		}
	}
	return nil
}

// ---- 签名核心：X-Local-Sig = HMAC-SHA256(secret, "METHOD|path|ts|nonce|payloadMode|bodyHash|userId") ----

func signedJSON(method, path string, body []byte, contentType string) (*http.Response, error) {
	c, err := loadCreds()
	if err != nil {
		return nil, err
	}
	resp, raw, err := signedCall(c, method, path, body, contentType)
	fmt.Printf("HTTP %d %s\n", resp.StatusCode, trunc2k(raw))
	return resp, err
}

func signedCall(c *creds, method, path string, body []byte, contentType string) (*http.Response, []byte, error) {
	if contentType == "" && len(body) > 0 {
		contentType = "application/json"
	}
	payloadMode := "empty"
	bodyHash := sha256Hex(nil)
	if len(body) > 0 {
		if strings.HasPrefix(contentType, "multipart/form-data") {
			payloadMode = "multipart"
			bodyHash = "UNSIGNED-PAYLOAD"
		} else {
			payloadMode = "json"
			bodyHash = sha256Hex(body)
		}
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randHex(16)
	canonical := strings.ToUpper(method) + "|" + path + "|" + ts + "|" + nonce + "|" + payloadMode + "|" + bodyHash + "|" + c.UserID
	mac := hmac.New(sha256.New, []byte(c.Secret))
	mac.Write([]byte(canonical))
	sig := hex.EncodeToString(mac.Sum(nil))

	req, _ := http.NewRequest(method, addr+path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("X-Local-Sig", sig)
	req.Header.Set("X-Local-Nonce", nonce)
	req.Header.Set("X-Local-Timestamp", ts)
	req.Header.Set("X-Local-Payload-Mode", payloadMode)
	req.Header.Set("X-Local-Body-Hash", bodyHash)
	req.Header.Set("X-Local-User-Id", c.UserID)
	req.Header.Set("X-Local-Session-Id", c.SessionID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	resp.Body.Close()
	return resp, raw, nil
}

func call(method, path string, body []byte, contentType string) (*http.Response, []byte, error) {
	req, _ := http.NewRequest(method, addr+path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	resp.Body.Close()
	return resp, raw, nil
}

// ---- 小工具 ----

func loadCreds() (*creds, error) {
	var c creds
	if err := readJSON(credsFile, &c); err != nil {
		return nil, fmt.Errorf("未配对，先跑 m0 pair-request / pair-confirm: %w", err)
	}
	return &c, nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func writeJSON(path string, v any) error {
	data, _ := json.MarshalIndent(v, "", "  ")
	return os.WriteFile(path, data, 0o600)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func trunc(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		return s[:300]
	}
	return s
}

func trunc2k(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 2000 {
		return s[:2000] + "…"
	}
	return s
}
