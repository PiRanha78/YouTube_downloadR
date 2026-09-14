package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	netURL "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	appTitle   = "YouTube DownloadR"
	appVersion = "1.1.0"
)

//go:embed static/*
var staticFS embed.FS

type server struct {
	base         string
	ytDlp        string
	ffmpeg       string
	mu           sync.Mutex
	busy         bool
	cancel       context.CancelFunc
	logCh        chan string
	clientsMu    sync.Mutex
	clients      map[chan string]struct{}
	httpServer   *http.Server
	shutdownOnce sync.Once
	hadClient    bool
}

type statusResp struct {
	Title      string `json:"title"`
	Version    string `json:"version"`
	YtDlpOK    bool   `json:"ytDlpOk"`
	FfmpegOK   bool   `json:"ffmpegOk"`
	YtDlpPath  string `json:"ytDlpPath"`
	FfmpegPath string `json:"ffmpegPath"`
	Downloads  string `json:"downloads"`
	Busy       bool   `json:"busy"`
}

type downloadReq struct {
	URL      string `json:"url"`
	Format   string `json:"format"`
	Bitrate  string `json:"bitrate"`
	DestRoot string `json:"destRoot"`
}

func main() {
	base := appBaseDir()

	if running, _ := trySingleInstance(); running {
		fatalPopup("A YouTube DownloadR már fut.\nNézd meg a tálcán / a megnyitott ablakok között.")
		return
	}

	ytDlp, ffmpeg, err := ensureBundledTools()
	if err != nil {
		// Külső app/ mappa, ha a beágyazott csomag hiányzik / hibás
		ytDlp = findYtDlp(base)
		ffmpeg = findFfmpeg(base)
		if ytDlp == "" || ffmpeg == "" {
			fatalPopup("Nem sikerült a beépített segédprogramok kibontása,\n" +
				"és az app\\ mappa sem található.\n\n" + err.Error())
			return
		}
	}
	s := &server{
		base:    base,
		ytDlp:   ytDlp,
		ffmpeg:  ffmpeg,
		logCh:   make(chan string, 256),
		clients: make(map[chan string]struct{}),
	}
	go s.broadcastLoop()

	mux := http.NewServeMux()
	sub, _ := fs.Sub(staticFS, "static")
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/download", s.handleDownload)
	mux.HandleFunc("/api/cancel", s.handleCancel)
	mux.HandleFunc("/api/update", s.handleUpdate)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/pick-folder", s.handlePickFolder)
	mux.HandleFunc("/api/open-folder", s.handleOpenFolder)
	mux.HandleFunc("/api/shutdown", s.handleShutdown)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fatalPopup("Nem indítható a helyi szerver:\n" + err.Error())
		return
	}
	url := fmt.Sprintf("http://%s", ln.Addr().String())
	if err := writeInstance(url); err != nil {
		// nem végzetes
		_ = err
	}
	defer clearInstance()

	s.httpServer = &http.Server{Handler: onlyLocal(mux)}
	s.startIdleWatcher()

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			fatalPopup(err.Error())
		}
	}()

	// Natív ablak (Windows WebView2) – nem böngészőben nyílik
	runNativeUI(url)

	// Ablak bezárva
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	clearInstance()
	_ = s.httpServer.Close()
}

func appBaseDir() string {
	exe, err := os.Executable()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	exe, _ = filepath.EvalSymlinks(exe)
	dir := filepath.Dir(exe)
	// .app csomag: ne a Contents/MacOS-be mentsünk
	if runtime.GOOS == "darwin" && strings.Contains(dir, ".app/Contents/MacOS") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, "Documents", "YouTube_DownloadR")
		}
	}
	return dir
}

func findYtDlp(base string) string {
	cands := []string{
		filepath.Join(base, "app", "yt-dlp.exe"),
		filepath.Join(base, "yt-dlp.exe"),
		filepath.Join(base, "app", "yt-dlp"),
		filepath.Join(base, "yt-dlp"),
	}
	for _, p := range cands {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func findFfmpeg(base string) string {
	cands := []string{
		filepath.Join(base, "app", "ffmpeg-8.0.1-essentials_build", "bin", "ffmpeg.exe"),
		filepath.Join(base, "app", "ffmpeg", "bin", "ffmpeg.exe"),
		filepath.Join(base, "app", "ffmpeg.exe"),
		filepath.Join(base, "ffmpeg.exe"),
		filepath.Join(base, "app", "ffmpeg-8.0.1-essentials_build", "bin", "ffmpeg"),
		filepath.Join(base, "app", "ffmpeg", "bin", "ffmpeg"),
	}
	for _, p := range cands {
		if fileExists(p) {
			return p
		}
	}
	matches, _ := filepath.Glob(filepath.Join(base, "app", "ffmpeg-*", "bin", "ffmpeg.exe"))
	if len(matches) > 0 {
		return matches[0]
	}
	matches, _ = filepath.Glob(filepath.Join(base, "app", "ffmpeg-*", "bin", "ffmpeg"))
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	busy := s.busy
	s.mu.Unlock()
	// Csak akkor keressünk külső app/ mappát, ha a beágyazott runtime nincs meg
	if !fileExists(s.ytDlp) {
		s.ytDlp = findYtDlp(s.base)
	}
	if !fileExists(s.ffmpeg) {
		s.ffmpeg = findFfmpeg(s.base)
	}
	resp := statusResp{
		Title:      appTitle,
		Version:    appVersion,
		YtDlpOK:    s.ytDlp != "",
		FfmpegOK:   s.ffmpeg != "",
		YtDlpPath:  orDefault(s.ytDlp, filepath.Join(s.base, "app", "yt-dlp.exe")),
		FfmpegPath: orDefault(s.ffmpeg, filepath.Join(s.base, "app", "ffmpeg-8.0.1-essentials_build", "bin", "ffmpeg.exe")),
		Downloads:  defaultDownloadsDir(s.base),
		Busy:       busy,
	}
	writeJSON(w, resp)
}

func (s *server) handlePickFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Initial string `json:"initial"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	initial := strings.TrimSpace(body.Initial)
	if initial == "" {
		initial = defaultDownloadsDir(s.base)
	}
	path, err := pickFolder(initial)
	if err != nil || path == "" {
		msg := "cancelled"
		if err != nil {
			msg = err.Error()
		}
		writeJSON(w, map[string]any{"ok": false, "path": "", "error": msg})
		return
	}
	saveConfig(appConfig{DestRoot: path})
	writeJSON(w, map[string]any{"ok": true, "path": path})
}

func (s *server) handleOpenFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	path := strings.TrimSpace(body.Path)
	if path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	if err := openFolder(path); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		s.emit("Megszakítás kérve…")
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		http.Error(w, "busy", http.StatusConflict)
		return
	}
	s.busy = true
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.busy = false
			s.mu.Unlock()
		}()
		if s.ytDlp == "" {
			s.emit("HIBA: yt-dlp nem található")
			return
		}
		s.emit("yt-dlp frissítés indul…")
		code := s.runCmd(context.Background(), s.ytDlp, "-U")
		if code == 0 {
			s.emit("yt-dlp frissítés kész.")
		} else {
			s.emit(fmt.Sprintf("yt-dlp frissítés hibakód: %d", code))
		}
	}()
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	var req downloadReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	req.Format = strings.ToLower(strings.TrimSpace(req.Format))
	req.Bitrate = strings.TrimSpace(req.Bitrate)
	if req.URL == "" {
		http.Error(w, "missing url", http.StatusBadRequest)
		return
	}
	if req.Format != "mp3" && req.Format != "mp4" && req.Format != "wav" {
		http.Error(w, "bad format", http.StatusBadRequest)
		return
	}
	if req.Format == "mp3" && req.Bitrate != "128" && req.Bitrate != "256" && req.Bitrate != "320" {
		req.Bitrate = "320"
	}
	if s.ytDlp == "" || s.ffmpeg == "" {
		http.Error(w, "missing tools", http.StatusBadRequest)
		return
	}

	destRoot := strings.TrimSpace(req.DestRoot)
	if destRoot == "" {
		destRoot = defaultDownloadsDir(s.base)
	}
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	saveConfig(appConfig{DestRoot: destRoot})

	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		http.Error(w, "busy", http.StatusConflict)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.busy = true
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		defer func() {
			cancel()
			s.mu.Lock()
			s.busy = false
			s.cancel = nil
			s.mu.Unlock()
		}()
		s.doDownload(ctx, req.URL, destRoot, req.Format, req.Bitrate)
	}()
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *server) doDownload(ctx context.Context, url, destRoot, format, bitrate string) {
	s.emit(fmt.Sprintf("——— %s ———", time.Now().Format("2006-01-02 15:04:05")))
	s.emit("URL: " + url)
	extra := ""
	if format == "mp3" {
		extra = " @ " + bitrate + " kbps"
	}
	s.emit("Formátum: " + strings.ToUpper(format) + extra)

	playlist := isPlaylistURL(url)
	outDir := destRoot
	if playlist {
		s.emit("Playlist észlelve (list=) – videónként külön alkönyvtár.")
	} else {
		s.emit("Egyetlen videó – mentés közvetlenül a kiválasztott mappába.")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		s.emit("Mappa hiba: " + err.Error())
		s.emit("EVENT:FAIL")
		return
	}
	s.emit("Mappa: " + outDir)

	args := s.buildArgs(url, outDir, format, bitrate, playlist)
	s.emit("Parancs: " + s.ytDlp + " " + strings.Join(args, " "))
	code := s.runCmd(ctx, s.ytDlp, args...)
	if code == 0 {
		s.emit("Sikeres letöltés → " + outDir)
		s.emit("EVENT:OK:" + outDir)
	} else if ctx.Err() != nil {
		s.emit("Megszakítva.")
		s.emit("EVENT:CANCEL")
	} else {
		s.emit(fmt.Sprintf("Sikertelen, hibakód: %d", code))
		s.emit("EVENT:FAIL")
	}
}

func isPlaylistURL(raw string) bool {
	u, err := netURL.Parse(raw)
	if err != nil {
		return strings.Contains(strings.ToLower(raw), "list=")
	}
	return strings.TrimSpace(u.Query().Get("list")) != ""
}

func (s *server) buildArgs(url, outDir, format, bitrate string, playlist bool) []string {
	var outTpl string
	if playlist {
		// Playlist: playlist mappa / videónként saját alkönyvtár
		outTpl = filepath.Join(
			outDir,
			"%(playlist)s",
			"%(title)s [%(id)s]",
			"%(title)s.%(ext)s",
		)
	} else {
		outTpl = filepath.Join(outDir, "%(title)s.%(ext)s")
	}
	common := []string{
		"--ffmpeg-location", s.ffmpeg,
		"--force-ipv4",
		"-o", outTpl,
	}
	if playlist {
		common = append([]string{"--yes-playlist"}, common...)
	} else {
		common = append([]string{"--no-playlist"}, common...)
	}
	switch format {
	case "mp4":
		return append(common,
			"-f", "bestvideo*+bestaudio/best",
			"--merge-output-format", "mp4",
			url,
		)
	case "wav":
		return append(common,
			"-ciw",
			"-f", "bestaudio/best",
			"-x",
			"--audio-format", "wav",
			"--concurrent-fragments", "10",
			"--http-chunk-size", "10M",
			"--postprocessor-args", "ffmpeg:-c:a pcm_s16le -threads 4",
			url,
		)
	default:
		br := bitrate
		return append(common,
			"-ciw",
			"-f", "bestaudio/best",
			"-x",
			"--audio-format", "mp3",
			"--audio-quality", br+"K",
			"--concurrent-fragments", "10",
			"--http-chunk-size", "10M",
			"--postprocessor-args", fmt.Sprintf("ffmpeg:-c:a libmp3lame -b:a %sk -threads 4", br),
			url,
		)
	}
}

func (s *server) runCmd(ctx context.Context, name string, args ...string) int {
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.emit("Indítási hiba: " + err.Error())
		return 1
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		s.emit("Indítási hiba: " + err.Error())
		return 1
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		s.emit(sc.Text())
	}
	if err := cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return 1
	}
	return 0
}

func (s *server) handleLogs(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 64)
	s.clientsMu.Lock()
	s.clients[ch] = struct{}{}
	s.clientsMu.Unlock()
	s.markClient()
	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, ch)
		s.clientsMu.Unlock()
		close(ch)
	}()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", jsonString(msg))
			flusher.Flush()
		}
	}
}

func (s *server) emit(msg string) {
	select {
	case s.logCh <- msg:
	default:
	}
}

func (s *server) broadcastLoop() {
	for msg := range s.logCh {
		s.clientsMu.Lock()
		for ch := range s.clients {
			select {
			case ch <- msg:
			default:
			}
		}
		s.clientsMu.Unlock()
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func fatalPopup(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	if runtime.GOOS == "windows" {
		ps := fmt.Sprintf(`Add-Type -AssemblyName PresentationFramework;[System.Windows.MessageBox]::Show("%s","%s")`,
			escapePS(msg), appTitle)
		c := exec.Command("powershell", "-NoProfile", "-Command", ps)
		hideWindow(c)
		_ = c.Run()
	}
}

func escapePS(s string) string {
	s = strings.ReplaceAll(s, `'`, `''`)
	s = strings.ReplaceAll(s, `"`, `'`)
	s = strings.ReplaceAll(s, "\n", "`n")
	return s
}