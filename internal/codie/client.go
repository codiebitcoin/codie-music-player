// Package codie: signed HTTP client for a clt01_embed_backend device (LocalAuthGuard),
// plus the music pipeline (upload → source switch → PLAY+queue) proven in the M0 spike.
package codie

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
	PayloadEmpty     = "empty"
	PayloadJSON      = "json"
	PayloadMultipart = "multipart"
	bodyHashUnsigned = "UNSIGNED-PAYLOAD"

	apiMusic = "/v1/api/data/user/music"
	apiMedia = "/v1/api/data/user/media"
)

// Creds: a persisted client credential (preferred) or the master secret.
type Creds struct {
	Addr      string `json:"addr"`       // host:port
	Secret    string `json:"secret"`     // signing key (client secret or master secret)
	SessionID string `json:"session_id"` // client credential id; empty for master
	UserID    string `json:"user_id"`
	Kind      string `json:"kind"` // client | master
	DeviceID  string `json:"device_id,omitempty"`
	Name      string `json:"name,omitempty"`
	PairedAt  int64  `json:"paired_at,omitempty"`
}

// APIError is a non-2xx device response.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("codie: %s: %s (HTTP %d)", e.Code, e.Message, e.Status)
	}
	return fmt.Sprintf("codie: %s (HTTP %d)", e.Message, e.Status)
}

type Client struct {
	Creds *Creds
	HTTP  *http.Client
}

func New(creds *Creds) *Client {
	return &Client{Creds: creds, HTTP: &http.Client{Timeout: 120 * time.Second}}
}

func baseURL(addr string) string { return "http://" + strings.TrimSpace(addr) }

// Call performs a signed request. mode: PayloadEmpty/PayloadJSON/PayloadMultipart
// (multipart bodies pass contentType with the boundary; body hash is UNSIGNED-PAYLOAD).
// Returns the raw response body.
func (c *Client) Call(method, path string, body []byte, mode, contentType string) ([]byte, error) {
	cr := c.Creds
	if contentType == "" && len(body) > 0 && mode != PayloadMultipart {
		contentType = "application/json"
	}
	payloadMode, bodyHash := PayloadEmpty, sha256Hex(nil)
	if mode == PayloadMultipart {
		payloadMode, bodyHash = PayloadMultipart, bodyHashUnsigned
	} else if len(body) > 0 {
		payloadMode, bodyHash = PayloadJSON, sha256Hex(body)
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randHex(16)
	signPath := path
	if i := strings.IndexByte(signPath, '?'); i >= 0 {
		signPath = signPath[:i] // query is not signed
	}
	if cr.UserID == "" {
		if uid, err := c.bootstrapUserID(); err == nil {
			cr.UserID = uid
		}
	}
	mac := hmac.New(sha256.New, []byte(cr.Secret))
	mac.Write([]byte(strings.ToUpper(method) + "|" + signPath + "|" + ts + "|" + nonce + "|" + payloadMode + "|" + bodyHash + "|" + cr.UserID))

	req, err := http.NewRequest(method, baseURL(cr.Addr)+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("X-Local-Sig", hex.EncodeToString(mac.Sum(nil)))
	req.Header.Set("X-Local-Nonce", nonce)
	req.Header.Set("X-Local-Timestamp", ts)
	req.Header.Set("X-Local-Payload-Mode", payloadMode)
	req.Header.Set("X-Local-Body-Hash", bodyHash)
	req.Header.Set("X-Local-User-Id", cr.UserID)
	req.Header.Set("X-Local-Session-Id", cr.SessionID)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		json.Unmarshal(raw, &e)
		return nil, &APIError{Status: resp.StatusCode, Code: e.Code, Message: first(e.Message, strings.TrimSpace(string(raw)))}
	}
	return raw, nil
}

// GetJSON / PostJSON: signed calls decoding the {"success":true,"data":...} envelope.
func (c *Client) GetJSON(path string, out any) error { return c.reqJSON("GET", path, nil, out) }

func (c *Client) PostJSON(path string, in, out any) error {
	var body []byte
	if in != nil {
		body, _ = json.Marshal(in)
	}
	return c.reqJSON("POST", path, body, out)
}

func (c *Client) reqJSON(method, path string, body []byte, out any) error {
	raw, err := c.Call(method, path, body, PayloadJSON, "")
	if err != nil {
		return err
	}
	var env struct {
		Success bool            `json:"success"`
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("bad envelope: %s", trunc(raw, 200))
	}
	if !env.Success {
		return &APIError{Status: 200, Code: env.Code, Message: env.Message}
	}
	if out != nil && len(env.Data) > 0 {
		return json.Unmarshal(env.Data, out)
	}
	return nil
}

func (c *Client) bootstrapUserID() (string, error) {
	resp, err := http.Get(baseURL(c.Creds.Addr) + "/v1/api/auth/bootstrap")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var b struct {
		Data struct {
			UserID string `json:"user_id"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &b) != nil {
		return "", fmt.Errorf("bad bootstrap")
	}
	return b.Data.UserID, nil
}

// ---- pairing (unsigned endpoints) ----

func (c *Client) RequestPairing() (string, error) {
	resp, err := http.Post(baseURL(c.Creds.Addr)+"/v1/api/auth/pair/request", "application/json", nil)
	if err != nil {
		return "", err
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	var r struct {
		Success bool `json:"success"`
		Data    struct {
			ChallengeID string `json:"challenge_id"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &r) != nil || !r.Success || r.Data.ChallengeID == "" {
		return "", fmt.Errorf("pair/request failed: %s", trunc(raw, 200))
	}
	return r.Data.ChallengeID, nil
}

// ConfirmPairing exchanges the 6-digit code for a master secret, then immediately
// issues a dedicated client credential so the master secret never persists.
func (c *Client) ConfirmPairing(challengeID, code, label string) (*Creds, error) {
	body, _ := json.Marshal(map[string]string{"pairing_code": code, "challenge_id": challengeID, "auth_scope": "permanent"})
	resp, err := http.Post(baseURL(c.Creds.Addr)+"/v1/api/auth/pair/confirm", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	var pr struct {
		Data struct {
			LocalSecret string `json:"local_secret"`
			DeviceID    string `json:"device_id"`
			Name        string `json:"name"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &pr) != nil || pr.Data.LocalSecret == "" {
		return nil, fmt.Errorf("pair/confirm failed: %s", trunc(raw, 200))
	}
	uid, err := c.bootstrapUserID()
	if err != nil || uid == "" {
		return nil, fmt.Errorf("device reachable but not logged in")
	}
	master := &Creds{Addr: c.Creds.Addr, Secret: pr.Data.LocalSecret, UserID: uid, Kind: "master",
		DeviceID: pr.Data.DeviceID, Name: pr.Data.Name, PairedAt: time.Now().Unix()}
	// Issue a client credential bound to this app.
	mc := New(master)
	cb, _ := json.Marshal(map[string]string{"label": label})
	var cr struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	if err := mc.reqJSON("POST", "/v1/api/auth/credentials", cb, &cr); err == nil && cr.ID != "" {
		return &Creds{Addr: c.Creds.Addr, Secret: cr.Secret, SessionID: cr.ID, UserID: uid, Kind: "client",
			DeviceID: pr.Data.DeviceID, Name: pr.Data.Name, PairedAt: time.Now().Unix()}, nil
	}
	return master, nil // fallback: keep master (still works)
}

// ---- device/music API ----

type MusicStorage struct {
	IsLoaded  bool   `json:"is_loaded"`
	RootPath  string `json:"root_path"`
	CapacityT uint64 `json:"capacity_total_bytes"`
	CapacityU uint64 `json:"capacity_used_bytes"`
	Files     []struct {
		Name       string `json:"name"`
		Path       string `json:"path"`
		SizeBytes  int64  `json:"size_bytes"`
		DurationMs int64  `json:"duration_ms"`
	} `json:"music_files"`
}

func (c *Client) MusicStorage() (*MusicStorage, error) {
	var st MusicStorage
	if err := c.GetJSON(apiMusic+"/storage", &st); err != nil {
		return nil, err
	}
	return &st, nil
}

type PlaybackState struct {
	RequestID  string `json:"request_id"`
	Status     string `json:"status"`
	TrackID    string `json:"track_id"`
	TrackName  string `json:"track_name"`
	DurationMs int64  `json:"duration_ms"`
	PositionMs int64  `json:"position_ms"`
	IsPlaying  bool   `json:"is_playing"`
	Volume     int32  `json:"volume_level"`
	VolumeMax  int32  `json:"volume_max"`
	ErrorCode  string `json:"error_code"`
	ErrorMsg   string `json:"error_message"`
	Applied    bool   `json:"applied"`
	SessionVer int64  `json:"music_session_version"`
	UpdatedAt  int64  `json:"updated_at_ms"`
}

func (c *Client) Playback() (*PlaybackState, error) {
	var p PlaybackState
	if err := c.GetJSON(apiMusic+"/playback", &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Control sends a music control action. Queue/entry fields follow the on-device
// frontend contract (snake_case, PLAY + inline queue — see docs/m0-auth-report.md).
func (c *Client) Control(action string, extra map[string]any) error {
	payload := map[string]any{"action": action, "source": "LOCAL_TF", "request_id": fmt.Sprintf("cmp-%d", time.Now().UnixNano())}
	for k, v := range extra {
		payload[k] = v
	}
	return c.PostJSON(apiMusic+"/control", payload, nil)
}

// EnsureSource switches the device music source to LOCAL_TF if it is not already.
// A device left in BLUETOOTH silently ignores TF playback commands (M0 lesson #2).
func (c *Client) EnsureSource() error {
	var src struct {
		Source string `json:"source"`
	}
	if err := c.GetJSON(apiMusic+"/source", &src); err != nil {
		return err
	}
	if src.Source == "LOCAL_TF" {
		return nil
	}
	if err := c.PostJSON(apiMusic+"/source", map[string]any{"source": "LOCAL_TF"}, nil); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	return nil
}

// UploadFile uploads one audio file to the TF card (multipart field "files", not "files[]").
// Returns the on-card path of the uploaded file.
func (c *Client) UploadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	boundary := "cmp" + strconv.FormatInt(time.Now().UnixNano(), 10)
	writeField := func(name, value string) {
		buf.WriteString("--" + boundary + "\r\nContent-Disposition: form-data; name=\"" + name + "\"\r\n\r\n" + value + "\r\n")
	}
	writeField("conflict_policy", "overwrite")
	buf.WriteString("--" + boundary + "\r\nContent-Disposition: form-data; name=\"files\"; filename=\"" + filepath.Base(path) + "\"\r\nContent-Type: application/octet-stream\r\n\r\n")
	buf.Write(data)
	buf.WriteString("\r\n--" + boundary + "--\r\n")

	raw, err := c.Call("POST", apiMusic+"/upload", buf.Bytes(), PayloadMultipart, "multipart/form-data; boundary="+boundary)
	if err != nil {
		return "", err
	}
	var up struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Uploaded []struct {
				Name string `json:"name"`
				Path string `json:"path"`
			} `json:"uploaded"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &up) != nil || !up.Success || len(up.Data.Uploaded) == 0 {
		return "", &APIError{Status: 200, Code: up.Code, Message: first(up.Message, trunc(raw, 200))}
	}
	return up.Data.Uploaded[0].Path, nil
}

// Play uploads paths already on the TF card: builds an inline queue and plays it.
// ExpectedQueueRevision 0 creates a fresh queue (the only mode this app needs).
func (c *Client) PlayCardPaths(cardRoot string, cardPaths []string, title string) error {
	if len(cardPaths) == 0 {
		return fmt.Errorf("empty queue")
	}
	type entry struct {
		ID      string `json:"id"`
		TrackID string `json:"track_id"`
	}
	entries := make([]entry, 0, len(cardPaths))
	ids := make([]string, 0, len(cardPaths))
	for _, p := range cardPaths {
		id := randHex(16)
		entries = append(entries, entry{ID: id, TrackID: "local:" + p})
		ids = append(ids, id)
	}
	payload := map[string]any{
		"action": "PLAY", "source": "LOCAL_TF", "request_id": fmt.Sprintf("cmp-%d", time.Now().UnixNano()),
		"expected_queue_revision": 0, "queue_entry_id": ids[0],
		"queue": map[string]any{
			"storage_root": cardRoot, "title": first(title, "codie-music-play"), "auto_advance": true, "revision": 0,
			"entries": entries, "current_entry_id": ids[0],
		},
	}
	return c.PostJSON(apiMusic+"/control", payload, nil)
}

// MediaCapabilities probes the 1.6.0 media-session API (404 on older firmware).
func (c *Client) MediaCapabilities() bool {
	_, err := c.Call("GET", apiMedia+"/capabilities", nil, PayloadEmpty, "")
	return err == nil
}

// ---- helpers ----

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func trunc(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		return s[:n]
	}
	return s
}
