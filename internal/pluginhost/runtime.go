// Plugin runtime: one goja VM + event loop per plugin, with the mygo-music host API
// (fetch/storage/crypto/setting/signInPrompt/webSignIn) implemented in Go.
package pluginhost

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// Manifest mirrors plugin.json.
type Manifest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Main     string `json:"main"`
	Playback string `json:"playback"`
	Rights   struct {
		Network []string `json:"network"`
		Process []string `json:"process"`
	} `json:"permissions"`
	Settings []struct {
		Key   string `json:"key"`
		Label string `json:"label"`
		Help  string `json:"help"`
		Link  string `json:"link"`
	} `json:"settings"`
	Source string `json:"-"` // installed-from description
}

func (m Manifest) Network() []string { return m.Rights.Network }

// Prompt is the latest signInPrompt payload relayed to the UI.
type Prompt struct {
	QR       string `json:"qr"`
	Text     string `json:"text"`
	At       int64  `json:"at"`
	Finished bool   `json:"finished"`
}

// Host runs a single plugin.
type Host struct {
	ID       string
	Manifest Manifest

	loop      *eventloop.EventLoop
	exports   map[string]goja.Value
	exportsMu sync.Mutex

	storagePath string
	storageMu   sync.Mutex
	storage     map[string]any

	settings func(pluginID, key string) string // app-provided setting overrides

	promptMu sync.Mutex
	prompt   Prompt

	client *http.Client
	loaded bool
	Err    error
}

// prelude: polyfills the plugins rely on (URL, AbortController, fetch response factory).
const jsPrelude = `
function __URL(href){
  href = String(href);
  var m = /^[a-z][a-z0-9+.-]*:\/\/([^\/?#]*)([^?#]*)(\?[^#]*)?(#.*)?$/i.exec(href) || ["", "", href, "", ""];
  var proto = m[0] ? href.slice(0, href.indexOf("://") + 1) : "";
  this.href = href;
  this.protocol = proto;
  this.host = m[1] || "";
  this.hostname = this.host.split(":")[0] || "";
  this.pathname = m[2] || (m[0] ? "/" : href);
  this.search = m[3] || "";
  this.origin = this.protocol + "//" + this.host;
  var q = this.search.replace(/^\?/, "");
  this.searchParams = {
    _map: (function(){ var o={}; q.split("&").forEach(function(kv){ if(!kv) return; var p=kv.split("="); o[decodeURIComponent(p[0])]=decodeURIComponent(p[1]||"");}); return o;})(),
    get: function(k){ return k in this._map ? this._map[k] : null; }
  };
}
globalThis.URL = __URL;
globalThis.AbortController = function(){
  this.signal = { aborted: false, _l: [] };
  var s = this.signal;
  this.abort = function(){ if(s.aborted) return; s.aborted = true; s._l.forEach(function(f){ try{f();}catch(e){} }); };
  this.signal.addEventListener = function(_, f){ s._l.push(f); };
};
function __mkResponse(status, statusText, headersObj, setCookies, bodyText){
  var lower = {};
  Object.keys(headersObj||{}).forEach(function(k){ lower[String(k).toLowerCase()] = headersObj[k]; });
  var headers = {
    get: function(n){ var v = lower[String(n).toLowerCase()]; return v === undefined ? null : v; },
    getSetCookie: function(){ return setCookies || []; }
  };
  var text = function(){ return Promise.resolve(bodyText); };
  return {
    ok: status >= 200 && status < 300, status: status, statusText: statusText, headers: headers,
    text: text,
    json: function(){ return text().then(function(t){ return JSON.parse(t); }); }
  };
}
`

// NewHost builds (but does not start) a plugin host.
func NewHost(id string, manifest Manifest, pluginDataDir string, settings func(pluginID, key string) string) *Host {
	h := &Host{
		ID:          id,
		Manifest:    manifest,
		storagePath: filepath.Join(pluginDataDir, "storage.json"),
		storage:     map[string]any{},
		settings:    settings,
		client:      &http.Client{Timeout: 60 * time.Second},
	}
	if data, err := os.ReadFile(h.storagePath); err == nil {
		json.Unmarshal(data, &h.storage)
	}
	return h
}

// Start boots the event loop and evaluates the plugin module.
func (h *Host) Start(mainJS string) error {
	wrapped, _, err := transformESM(mainJS)
	if err != nil {
		h.Err = err
		return err
	}
	ready := make(chan error, 1)
	h.loop = eventloop.NewEventLoop()
	// NOTE: Run(fn) only starts the loop after fn returns — blocking inside fn deadlocks.
	// Start() runs the loop in the background; setup happens through RunOnLoop.
	h.loop.Start()
	h.loop.RunOnLoop(func(vm *goja.Runtime) {
		h.setupVM(vm)
		exportsVal, err := vm.RunScript(h.ID+".js", jsPrelude+"\n"+wrapped)
		if err != nil {
			h.Err = fmt.Errorf("plugin load: %w", err)
			ready <- h.Err
			return
		}
		exports, ok := exportsVal.(*goja.Object)
		if !ok {
			h.Err = fmt.Errorf("plugin module did not return an object")
			ready <- h.Err
			return
		}
		m := map[string]goja.Value{}
		for _, key := range exports.Keys() {
			m[key] = exports.Get(key)
		}
		h.exportsMu.Lock()
		h.exports = m
		h.exportsMu.Unlock()
		h.loaded = true
		ready <- nil
	})
	select {
	case err := <-ready:
		return err
	case <-time.After(15 * time.Second):
		h.Err = fmt.Errorf("plugin start timeout")
		return h.Err
	}
}

func (h *Host) Loaded() bool { return h.loaded }

func (h *Host) Stop() {
	if h.loop != nil {
		h.loop.Stop()
	}
}

// Exports lists the export names available on this plugin.
func (h *Host) Exports() []string {
	h.exportsMu.Lock()
	defer h.exportsMu.Unlock()
	out := make([]string, 0, len(h.exports))
	for k := range h.exports {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---- VM setup: globals and the host object ----

func (h *Host) setupVM(vm *goja.Runtime) {
	vm.Set("fetch", func(call goja.FunctionCall) goja.Value { return h.jsFetch(vm, call) })
	vm.Set("btoa", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(base64.StdEncoding.EncodeToString([]byte(call.Argument(0).String())))
	})
	vm.Set("atob", func(call goja.FunctionCall) goja.Value {
		s, err := base64.StdEncoding.DecodeString(call.Argument(0).String())
		if err != nil {
			panic(vm.NewTypeError("atob: invalid input"))
		}
		return vm.ToValue(string(s))
	})

	hostObj := vm.NewObject()
	_ = hostObj.Set("storage", h.storageAPI(vm))
	cryptoObj := vm.NewObject()
	_ = cryptoObj.Set("md5", func(call goja.FunctionCall) goja.Value {
		sum := md5.Sum([]byte(call.Argument(0).String()))
		return vm.ToValue(hex.EncodeToString(sum[:]))
	})
	_ = cryptoObj.Set("aesEcbEncrypt", func(call goja.FunctionCall) goja.Value {
		out, err := aesECBEncryptPKCS7([]byte(call.Argument(0).String()), []byte(call.Argument(1).String()))
		if err != nil {
			panic(vm.NewTypeError("aesEcbEncrypt: " + err.Error()))
		}
		return vm.ToValue(hex.EncodeToString(out))
	})
	_ = hostObj.Set("crypto", cryptoObj)
	_ = hostObj.Set("setting", func(call goja.FunctionCall) goja.Value {
		if v := h.settings(h.ID, call.Argument(0).String()); v != "" {
			return vm.ToValue(v)
		}
		return goja.Undefined()
	})
	_ = hostObj.Set("signInPrompt", func(call goja.FunctionCall) goja.Value {
		opts := call.Argument(0).ToObject(vm)
		h.promptMu.Lock()
		h.prompt = Prompt{
			QR:   opts.Get("qr").String(),
			Text: opts.Get("text").String(),
			At:   time.Now().UnixMilli(),
		}
		h.promptMu.Unlock()
		return goja.Undefined()
	})
	_ = hostObj.Set("webSignIn", func(call goja.FunctionCall) goja.Value {
		panic(vm.NewTypeError("webSignIn is not supported by codie-music-player yet — use QR login"))
	})
	vm.Set("host", hostObj)
}

func (h *Host) storageAPI(vm *goja.Runtime) *goja.Object {
	obj := vm.NewObject()
	save := func() {
		h.storageMu.Lock()
		defer h.storageMu.Unlock()
		if data, err := json.MarshalIndent(h.storage, "", " "); err == nil {
			os.MkdirAll(filepath.Dir(h.storagePath), 0o700)
			os.WriteFile(h.storagePath, data, 0o600)
		}
	}
	_ = obj.Set("get", func(call goja.FunctionCall) goja.Value {
		h.storageMu.Lock()
		v, ok := h.storage[call.Argument(0).String()]
		h.storageMu.Unlock()
		if !ok {
			return goja.Undefined()
		}
		return vm.ToValue(v)
	})
	_ = obj.Set("set", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		var val any
		if !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			val = call.Argument(1).Export()
		}
		h.storageMu.Lock()
		h.storage[key] = val
		h.storageMu.Unlock()
		save()
		return goja.Undefined()
	})
	_ = obj.Set("remove", func(call goja.FunctionCall) goja.Value {
		h.storageMu.Lock()
		delete(h.storage, call.Argument(0).String())
		h.storageMu.Unlock()
		save()
		return goja.Undefined()
	})
	return obj
}

// ---- fetch ----

func (h *Host) jsFetch(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	// JS-side deferred: {p, res, rej} — keeps everything as plain goja.Value.
	deferred, _ := vm.RunString("(function(){ var r, j; var p = new Promise(function(res, rej){ r = res; j = rej; }); return { p: p, res: r, rej: j }; })()")
	d := deferred.(*goja.Object)
	promise := d.Get("p")
	resolve, _ := goja.AssertFunction(d.Get("res"))
	reject, _ := goja.AssertFunction(d.Get("rej"))

	rawURL := call.Argument(0).String()
	opts := map[string]any{}
	if o := call.Argument(1); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
		if m, ok := o.Export().(map[string]any); ok {
			opts = m
		}
	}
	method := strings.ToUpper(optStr(opts, "method", "GET"))
	headers := map[string]string{}
	if hm, ok := opts["headers"].(map[string]any); ok {
		for k, v := range hm {
			headers[k] = toStr(v)
		}
	}
	var body []byte
	if b, ok := opts["body"]; ok && b != nil {
		if s, isStr := b.(string); isStr {
			body = []byte(s)
		} else if j, err := json.Marshal(b); err == nil {
			body = j
		}
	}
	signal := map[string]any{}
	if s, ok := opts["signal"].(map[string]any); ok {
		signal = s
	}
	aborted := func() bool {
		ab, _ := signal["aborted"].(bool)
		return ab
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		h.loopSoon(func() { reject(goja.Undefined(), vm.NewTypeError("fetch: invalid URL "+rawURL)) })
		return promise
	}
	if !h.hostAllowed(parsed.Hostname()) {
		h.loopSoon(func() { reject(goja.Undefined(), vm.NewTypeError("fetch blocked: "+parsed.Hostname()+" is not in the plugin network permissions")) })
		return promise
	}

	go func() {
		if aborted() {
			h.loopSoon(func() { reject(goja.Undefined(), vm.NewGoError(fmt.Errorf("aborted"))) })
			return
		}
		var reqBody io.Reader
		if len(body) > 0 {
			reqBody = bytes.NewReader(body)
		}
		req, err := http.NewRequest(method, rawURL, reqBody)
		if err != nil {
			h.loopSoon(func() { reject(goja.Undefined(), vm.NewGoError(err)) })
			return
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if len(body) > 0 && req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		resp, err := h.client.Do(req)
		if err != nil {
			if aborted() {
				h.loopSoon(func() { reject(goja.Undefined(), vm.NewGoError(fmt.Errorf("aborted"))) })
			} else {
				h.loopSoon(func() { reject(goja.Undefined(), vm.NewGoError(err)) })
			}
			return
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if aborted() {
			h.loopSoon(func() { reject(goja.Undefined(), vm.NewGoError(fmt.Errorf("aborted"))) })
			return
		}
		headerObj := map[string]any{}
		for k := range resp.Header {
			headerObj[k] = resp.Header.Get(k)
		}
		setCookies := append([]string{}, resp.Header.Values("Set-Cookie")...)
		status, statusText, bodyText := resp.StatusCode, resp.Status, string(raw)
		h.loopSoon(func() {
			mk, err2 := vm.RunString("(__mkResponse)")
			mkFn, okFn := goja.AssertFunction(mk)
			if err2 != nil || !okFn {
				reject(goja.Undefined(), vm.NewGoError(fmt.Errorf("__mkResponse missing")))
				return
			}
			res, err3 := mkFn(goja.Undefined(),
				vm.ToValue(status), vm.ToValue(statusText),
				vm.ToValue(headerObj), vm.ToValue(setCookies), vm.ToValue(bodyText))
			if err3 != nil {
				reject(goja.Undefined(), vm.NewGoError(err3))
				return
			}
			resolve(goja.Undefined(), res)
		})
	}()
	return promise
}

// loopSoon schedules fn on the plugin's event loop.
func (h *Host) loopSoon(fn func()) {
	if h.loop == nil {
		return
	}
	h.loop.RunOnLoop(func(vm *goja.Runtime) { fn() })
}

// hostAllowed matches an exact host or a "*.domain" permission suffix.
func (h *Host) hostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, pat := range h.Manifest.Rights.Network {
		pat = strings.ToLower(strings.TrimSpace(pat))
		if pat == "" {
			continue
		}
		if strings.HasPrefix(pat, "*.") {
			if host == pat[2:] || strings.HasSuffix(host, pat[1:]) { // *.x.com matches a.x.com and x.com
				return true
			}
		} else if host == pat {
			return true
		}
	}
	return false
}

// ---- calling plugin exports ----

// Call invokes an exported plugin function (unwrapping its promise) with a timeout.
func (h *Host) Call(ctx context.Context, name string, timeout time.Duration, args ...any) (any, error) {
	h.exportsMu.Lock()
	fnVal, ok := h.exports[name]
	h.exportsMu.Unlock()
	if !ok || goja.IsUndefined(fnVal) {
		return nil, fmt.Errorf("plugin %q has no export %q", h.ID, name)
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	type callResult struct {
		val any
		err error
	}
	ch := make(chan callResult, 1)
	loop := h.loop
	loop.RunOnLoop(func(vm *goja.Runtime) {
		defer func() {
			if r := recover(); r != nil {
				ch <- callResult{err: fmt.Errorf("panic in plugin %q: %v", h.ID, r)}
			}
		}()
		fn, ok := goja.AssertFunction(fnVal)
		if !ok {
			ch <- callResult{err: fmt.Errorf("export %q is not callable", name)}
			return
		}
		argv := make([]goja.Value, 0, len(args))
		for _, a := range args {
			argv = append(argv, vm.ToValue(a))
		}
		res, err := fn(goja.Undefined(), argv...)
		if err != nil {
			ch <- callResult{err: exceptionToErr(vm, err)}
			return
		}
		// Bridge the promise (or plain value) to Go channels via native settle callbacks.
		var once sync.Once
		onOK := func(call goja.FunctionCall) goja.Value {
			once.Do(func() { ch <- callResult{val: exportValue(call.Argument(0))} })
			return goja.Undefined()
		}
		onErr := func(call goja.FunctionCall) goja.Value {
			once.Do(func() { ch <- callResult{err: valueToErr(call.Argument(0))} })
			return goja.Undefined()
		}
		settleVal, _ := vm.RunString("(function(r, ok, bad){ Promise.resolve(r).then(function(v){ ok(v); }).catch(function(e){ bad(e); }); })")
		settle, okSettle := goja.AssertFunction(settleVal)
		if !okSettle {
			once.Do(func() { ch <- callResult{err: fmt.Errorf("settle helper missing")} })
			return
		}
		if _, err = settle(goja.Undefined(), res, vm.ToValue(onOK), vm.ToValue(onErr)); err != nil {
			once.Do(func() { ch <- callResult{err: exceptionToErr(vm, err)} })
		}
	})
	select {
	case r := <-ch:
		return r.val, r.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("plugin %q call %q timed out after %s", h.ID, name, timeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Prompt returns the latest sign-in prompt (QR) state.
func (h *Host) Prompt() Prompt {
	h.promptMu.Lock()
	defer h.promptMu.Unlock()
	return h.prompt
}

func (h *Host) ClearPrompt() {
	h.promptMu.Lock()
	h.prompt = Prompt{}
	h.promptMu.Unlock()
}

// PluginError carries a plugin-protocol error code (signIn/unavailable/setup/…).
type PluginError struct {
	Code    string
	Message string
}

func (e *PluginError) Error() string { return e.Code + ": " + e.Message }

// exceptionToErr extracts message + `.code` from a goja exception value.
func exceptionToErr(vm *goja.Runtime, err error) error {
	if exc, ok := err.(*goja.Exception); ok {
		return valueToErr(exc.Value())
	}
	return err
}

// valueToErr converts a thrown JS value (Error with optional .code) into an error.
func valueToErr(v goja.Value) error {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return fmt.Errorf("unknown plugin error")
	}
	if obj, ok := v.(*goja.Object); ok {
		code := ""
		if c := obj.Get("code"); c != nil && !goja.IsUndefined(c) && !goja.IsNull(c) {
			code = c.String()
		}
		msg := obj.Get("message")
		if msg != nil && !goja.IsUndefined(msg) && !goja.IsNull(msg) && msg.String() != "" {
			if code != "" {
				return &PluginError{Code: code, Message: msg.String()}
			}
			return fmt.Errorf("%s", msg.String())
		}
		if code != "" {
			return &PluginError{Code: code, Message: obj.String()}
		}
		return fmt.Errorf("%s", obj.String())
	}
	return fmt.Errorf("%s", v.String())
}

// exportValue converts a goja value into plain Go structures (JSON-friendly).
func exportValue(v goja.Value) any {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	return v.Export()
}

// ---- crypto helper ----

func aesECBEncryptPKCS7(key, plain []byte) ([]byte, error) {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, fmt.Errorf("key must be 16/24/32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append(plain, bytes.Repeat([]byte{byte(pad)}, pad)...)
	out := make([]byte, len(padded))
	for off := 0; off < len(padded); off += aes.BlockSize {
		block.Encrypt(out[off:off+aes.BlockSize], padded[off:off+aes.BlockSize])
	}
	return out, nil
}

// ---- small helpers ----

func optStr(m map[string]any, key, def string) string {
	if v, ok := m[key]; ok && v != nil {
		if s := toStr(v); s != "" {
			return s
		}
	}
	return def
}

func toStr(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case fmt.Stringer:
		return s.String()
	case nil:
		return ""
	default:
		b, _ := json.Marshal(v)
		return strings.Trim(string(b), `"`)
	}
}
