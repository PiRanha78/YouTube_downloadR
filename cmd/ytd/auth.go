package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type loginState struct {
	cmd     *exec.Cmd
	port    int
	browser string
}

var (
	loginMu sync.Mutex
	login   *loginState
)

func jsRuntimeArgs() []string {
	path, err := exec.LookPath("node")
	if err != nil || path == "" {
		for _, c := range []string{
			filepath.Join(os.Getenv("ProgramFiles"), "nodejs", "node.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "nodejs", "node.exe"),
		} {
			if fileExists(c) {
				path = c
				break
			}
		}
	}
	if path == "" {
		return nil
	}
	return []string{
		"--js-runtimes", "node:" + path,
		"--remote-components", "ejs:github",
	}
}

func authRoot() (string, error) {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Application Support", "YouTube_DownloadR", "auth"), nil
		}
		return filepath.Join(home, ".local", "share", "YouTube_DownloadR", "auth"), nil
	}
	return filepath.Join(local, "YouTube_DownloadR", "auth"), nil
}

func cookiesFilePath(browser string) string {
	root, err := authRoot()
	if err != nil {
		return ""
	}
	return filepath.Join(root, browser, "cookies.txt")
}

func saveAuthBrowser(browser string) {
	root, err := authRoot()
	if err != nil {
		return
	}
	_ = os.MkdirAll(root, 0o755)
	_ = os.WriteFile(filepath.Join(root, "browser.txt"), []byte(browser), 0o600)
}

func savedAuthBrowser() string {
	root, err := authRoot()
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(root, "browser.txt"))
	if err != nil {
		return ""
	}
	browser := strings.TrimSpace(string(raw))
	if browser != "chrome" && browser != "brave" {
		return ""
	}
	if !fileExists(cookiesFilePath(browser)) {
		return ""
	}
	return browser
}

func chooseLoginBrowser() string {
	if b := detectOpenBrowser(); b == "chrome" || b == "brave" {
		if findBrowserExe(b) != "" {
			return b
		}
	}
	if findBrowserExe("chrome") != "" {
		return "chrome"
	}
	if findBrowserExe("brave") != "" {
		return "brave"
	}
	return ""
}

func youtubeVideoURL(port int) string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", port))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var pages []struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	if json.NewDecoder(resp.Body).Decode(&pages) != nil {
		return ""
	}
	for _, p := range pages {
		if p.Type != "page" {
			continue
		}
		u := strings.TrimSpace(p.URL)
		low := strings.ToLower(u)
		if strings.Contains(low, "youtube.com/watch") || strings.Contains(low, "youtube.com/shorts/") || strings.Contains(low, "youtu.be/") {
			return u
		}
	}
	return ""
}

func findBrowserExe(browser string) string {
	var rels []string
	switch browser {
	case "chrome":
		rels = []string{
			filepath.Join("Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join("Google", "Chrome", "Application", "chrome"),
		}
	case "brave":
		rels = []string{
			filepath.Join("BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
			filepath.Join("BraveSoftware", "Brave-Browser", "Application", "brave"),
		}
	default:
		return ""
	}
	var roots []string
	if runtime.GOOS == "windows" {
		roots = []string{
			os.Getenv("PROGRAMFILES"),
			os.Getenv("PROGRAMFILES(X86)"),
			os.Getenv("LOCALAPPDATA"),
		}
	} else if runtime.GOOS == "darwin" {
		if browser == "chrome" {
			return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		}
		return "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"
	} else {
		name := "google-chrome"
		if browser == "brave" {
			name = "brave-browser"
		}
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		for _, rel := range rels {
			p := filepath.Join(root, rel)
			if fileExists(p) {
				return p
			}
		}
	}
	return ""
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	loginMu.Lock()
	if login != nil {
		loginMu.Unlock()
		writeJSON(w, map[string]any{"ok": true, "already": true})
		s.emit("A belépő ablak már nyitva van. Jelentkezz be, majd kattints a Belépés kész gombra.")
		return
	}
	loginMu.Unlock()

	browser := chooseLoginBrowser()
	if browser == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "Nincs telepített Chrome vagy Brave."})
		return
	}
	exe := findBrowserExe(browser)
	if exe == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "A böngésző nincs telepítve."})
		return
	}
	root, err := authRoot()
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	profile := filepath.Join(root, browser)
	if err := os.MkdirAll(profile, 0o755); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	cmd := exec.Command(exe,
		"--user-data-dir="+profile,
		"--remote-debugging-address=127.0.0.1",
		"--remote-debugging-port="+strconv.Itoa(port),
		"--remote-allow-origins=*",
		"--no-first-run",
		"--no-default-browser-check",
		"--new-window",
		"https://www.youtube.com",
	)
	if err := cmd.Start(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "A böngésző nem indult el: " + err.Error()})
		return
	}
	if err := waitDebugPort(port, 20*time.Second); err != nil {
		killLoginProcess(cmd)
		writeJSON(w, map[string]any{"ok": false, "error": "A belépő ablak nem nyílt meg: " + err.Error()})
		return
	}
	loginMu.Lock()
	login = &loginState{cmd: cmd, port: port, browser: browser}
	loginMu.Unlock()
	s.emit("Belépő ablak megnyílt (" + browser + "). Jelentkezz be a YouTube-ra, majd kattints a Belépés kész gombra.")
	writeJSON(w, map[string]any{"ok": true})
}

func (s *server) handleLoginFinish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	loginMu.Lock()
	state := login
	loginMu.Unlock()
	if state == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "Nincs nyitott belépő ablak."})
		return
	}
	cookies, err := fetchBrowserCookies(state.port)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "A sütiket nem sikerült kiolvasni. Az ablak még nyitva van."})
		s.emit("Süti olvasási hiba: " + err.Error())
		return
	}
	kept := filterLoginCookies(cookies)
	if !hasYouTubeLogin(kept) {
		writeJSON(w, map[string]any{"ok": false, "error": "Nem látok YouTube bejelentkezést. Jelentkezz be az ablakban, majd próbáld újra."})
		return
	}
	path := cookiesFilePath(state.browser)
	if err := writeNetscapeCookies(path, kept); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	saveAuthBrowser(state.browser)
	videoURL := youtubeVideoURL(state.port)
	killLoginProcess(state.cmd)
	loginMu.Lock()
	login = nil
	loginMu.Unlock()
	if videoURL != "" {
		s.emit("Belépés mentve, a videó címe beírva.")
	} else {
		s.emit("YouTube belépés mentve (" + state.browser + ").")
	}
	writeJSON(w, map[string]any{"ok": true, "videoUrl": videoURL})
}

func waitDebugPort(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	client := &http.Client{Timeout: 800 * time.Millisecond}
	var last error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			last = fmt.Errorf("státusz %d", resp.StatusCode)
		} else {
			last = err
		}
		time.Sleep(300 * time.Millisecond)
	}
	if last == nil {
		last = fmt.Errorf("időtúllépés")
	}
	return last
}

func killLoginProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if runtime.GOOS == "windows" {
		killer := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
		hideWindow(killer)
		_ = killer.Run()
		return
	}
	_ = cmd.Process.Kill()
}

type cdpCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
	Session  bool    `json:"session"`
}

func fetchBrowserCookies(port int) ([]cdpCookie, error) {
	wsURL, err := pageDebugSocket(port)
	if err != nil {
		return nil, err
	}
	conn, err := wsDial(wsURL)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.call(1, "Network.enable", nil); err != nil {
		return nil, err
	}
	raw, err := conn.call(2, "Network.getCookies", map[string]any{
		"urls": []string{
			"https://www.youtube.com/",
			"https://youtube.com/",
			"https://accounts.google.com/",
			"https://www.google.com/",
		},
	})
	if err != nil {
		raw, err = conn.call(3, "Network.getAllCookies", nil)
	}
	if err != nil {
		return nil, err
	}
	var result struct {
		Cookies []cdpCookie `json:"cookies"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result.Cookies, nil
}

func pageDebugSocket(port int) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", port))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var pages []struct {
		Type                 string `json:"type"`
		URL                  string `json:"url"`
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pages); err != nil {
		return "", err
	}
	var fallback string
	for _, p := range pages {
		if p.Type != "page" || p.WebSocketDebuggerURL == "" {
			continue
		}
		if strings.Contains(p.URL, "youtube.com") || strings.Contains(p.URL, "google.com") {
			return p.WebSocketDebuggerURL, nil
		}
		if fallback == "" {
			fallback = p.WebSocketDebuggerURL
		}
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("nincs nyitott böngészőlap")
}

func filterLoginCookies(in []cdpCookie) []cdpCookie {
	var out []cdpCookie
	for _, c := range in {
		if cookieDomainWanted(c.Domain) {
			out = append(out, c)
		}
	}
	return out
}

func cookieDomainWanted(domain string) bool {
	d := strings.TrimPrefix(strings.ToLower(domain), ".")
	switch {
	case d == "youtube.com" || strings.HasSuffix(d, ".youtube.com"):
		return true
	case d == "google.com" || strings.HasSuffix(d, ".google.com"):
		return true
	case d == "youtu.be" || strings.HasSuffix(d, ".youtu.be"):
		return true
	default:
		return false
	}
}

func hasYouTubeLogin(cookies []cdpCookie) bool {
	for _, c := range cookies {
		switch c.Name {
		case "LOGIN_INFO", "SID", "__Secure-1PSID", "__Secure-3PSID":
			return true
		}
	}
	return false
}

func writeNetscapeCookies(path string, cookies []cdpCookie) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Netscape HTTP Cookie File\n")
	for _, c := range cookies {
		domain := c.Domain
		if domain == "" || strings.ContainsAny(c.Name, "\t\r\n") || strings.ContainsAny(c.Value, "\t\r\n") {
			continue
		}
		sub := "FALSE"
		if strings.HasPrefix(domain, ".") {
			sub = "TRUE"
		}
		secure := "FALSE"
		if c.Secure {
			secure = "TRUE"
		}
		exp := "0"
		if !c.Session && c.Expires > 0 {
			exp = strconv.FormatInt(int64(c.Expires), 10)
		}
		lineDomain := domain
		if c.HTTPOnly {
			lineDomain = "#HttpOnly_" + domain
		}
		pathPart := c.Path
		if pathPart == "" {
			pathPart = "/"
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", lineDomain, sub, pathPart, secure, exp, c.Name, c.Value)
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

type wsConn struct {
	c *bufio.ReadWriter
	r net.Conn
}

func wsDial(rawURL string) (*wsConn, error) {
	if !strings.HasPrefix(rawURL, "ws://") {
		return nil, fmt.Errorf("nem támogatott cím")
	}
	rest := strings.TrimPrefix(rawURL, "ws://")
	host, path, ok := strings.Cut(rest, "/")
	if !ok {
		path = ""
	}
	path = "/" + path
	conn, err := net.DialTimeout("tcp", host, 5*time.Second)
	if err != nil {
		return nil, err
	}
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\nOrigin: http://127.0.0.1\r\n\r\n", path, host, key)
	if _, err := io.WriteString(conn, req); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, err
	}
	if !strings.Contains(status, "101") {
		conn.Close()
		return nil, fmt.Errorf("websocket elutasítva")
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, err
		}
		if line == "\r\n" {
			break
		}
	}
	accept := websocketAccept(key)
	_ = accept
	return &wsConn{c: bufio.NewReadWriter(br, bufio.NewWriter(conn)), r: conn}, nil
}

func websocketAccept(key string) string {
	h := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h[:])
}

func (w *wsConn) Close() error { return w.r.Close() }

func (w *wsConn) call(id int, method string, params any) (json.RawMessage, error) {
	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	payload, _ := json.Marshal(msg)
	if err := w.writeText(payload); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("időtúllépés")
		}
		_ = w.r.SetReadDeadline(deadline)
		msg, op, err := w.readMessage()
		if err != nil {
			return nil, err
		}
		if op == 0x8 {
			return nil, fmt.Errorf("a kapcsolat bezárult")
		}
		if op == 0x9 {
			_ = w.writeControl(0xA, msg)
			continue
		}
		if op != 0x1 {
			continue
		}
		var envelope struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(msg, &envelope) != nil || envelope.ID != id {
			continue
		}
		if envelope.Error != nil {
			return nil, fmt.Errorf("%s", envelope.Error.Message)
		}
		return envelope.Result, nil
	}
}

func (w *wsConn) writeText(payload []byte) error {
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}
	header := []byte{0x81}
	n := len(payload)
	switch {
	case n < 126:
		header = append(header, byte(0x80|n))
	case n <= 65535:
		header = append(header, 0x80|126, byte(n>>8), byte(n))
	default:
		ext := make([]byte, 8)
		binary.BigEndian.PutUint64(ext, uint64(n))
		header = append(header, 0x80|127)
		header = append(header, ext...)
	}
	if _, err := w.c.Write(header); err != nil {
		return err
	}
	if _, err := w.c.Write(mask); err != nil {
		return err
	}
	masked := make([]byte, n)
	for i := range payload {
		masked[i] = payload[i] ^ mask[i%4]
	}
	if _, err := w.c.Write(masked); err != nil {
		return err
	}
	return w.c.Flush()
}

func (w *wsConn) writeControl(op byte, payload []byte) error {
	if len(payload) > 125 {
		payload = payload[:125]
	}
	mask := make([]byte, 4)
	_, _ = rand.Read(mask)
	buf := []byte{0x80 | op, byte(0x80 | len(payload))}
	buf = append(buf, mask...)
	for i := range payload {
		buf = append(buf, payload[i]^mask[i%4])
	}
	if _, err := w.c.Write(buf); err != nil {
		return err
	}
	return w.c.Flush()
}

func (w *wsConn) readMessage() ([]byte, byte, error) {
	var buf []byte
	var op byte
	for {
		fin, frameOp, payload, err := w.readFrame()
		if err != nil {
			return nil, 0, err
		}
		if frameOp == 0x8 || frameOp == 0x9 || frameOp == 0xA {
			return payload, frameOp, nil
		}
		if frameOp != 0 {
			op = frameOp
		}
		buf = append(buf, payload...)
		if fin {
			return buf, op, nil
		}
	}
}

func (w *wsConn) readFrame() (bool, byte, []byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(w.c, hdr[:]); err != nil {
		return false, 0, nil, err
	}
	fin := hdr[0]&0x80 != 0
	op := hdr[0] & 0x0f
	masked := hdr[1]&0x80 != 0
	n := int(hdr[1] & 0x7f)
	switch n {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(w.c, ext[:]); err != nil {
			return false, 0, nil, err
		}
		n = int(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(w.c, ext[:]); err != nil {
			return false, 0, nil, err
		}
		n = int(binary.BigEndian.Uint64(ext[:]))
	}
	if n < 0 || n > 8*1024*1024 {
		return false, 0, nil, fmt.Errorf("túl nagy üzenet")
	}
	var mask []byte
	if masked {
		mask = make([]byte, 4)
		if _, err := io.ReadFull(w.c, mask); err != nil {
			return false, 0, nil, err
		}
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(w.c, payload); err != nil {
		return false, 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return fin, op, payload, nil
}
