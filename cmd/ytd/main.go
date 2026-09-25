package main

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io/fs"
	"math"
	"net"
	"net/http"
	netURL "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	id3v2 "github.com/bogem/id3v2/v2"
	xdraw "golang.org/x/image/draw"
)

const (
	appTitle   = "YouTube DownloadR"
	appVersion = "1.3.4"
)

//go:embed static/*
var staticFS embed.FS

type server struct {
	base         string
	ytDlp        string
	ffmpeg       string
	ffprobe      string
	probeMu      sync.Mutex
	cachedProbe  *formatProbe
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
	AuthOK     bool   `json:"authOk"`
	AuthBrowser string `json:"authBrowser,omitempty"`
}

type downloadReq struct {
	URL            string `json:"url"`
	Format         string `json:"format"`
	Bitrate        string `json:"bitrate"`
	Resolution     string `json:"resolution"`
	CookiesBrowser string `json:"cookiesBrowser"`
	DestRoot       string `json:"destRoot"`
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
		ffprobe: ffprobeBeside(ffmpeg),
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
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/login/finish", s.handleLoginFinish)
	mux.HandleFunc("/api/probe", s.handleProbe)
	mux.HandleFunc("/api/thumb", s.handleThumb)

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
	authBrowser := savedAuthBrowser()
	resp := statusResp{
		Title:       appTitle,
		Version:     appVersion,
		YtDlpOK:     s.ytDlp != "",
		FfmpegOK:    s.ffmpeg != "",
		YtDlpPath:   orDefault(s.ytDlp, filepath.Join(s.base, "app", "yt-dlp.exe")),
		FfmpegPath:  orDefault(s.ffmpeg, filepath.Join(s.base, "app", "ffmpeg-8.0.1-essentials_build", "bin", "ffmpeg.exe")),
		Downloads:   defaultDownloadsDir(s.base),
		Busy:        busy,
		AuthOK:      authBrowser != "",
		AuthBrowser: authBrowser,
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
	req.Resolution = strings.TrimSpace(req.Resolution)
	req.CookiesBrowser = strings.ToLower(strings.TrimSpace(req.CookiesBrowser))
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
	if req.Format == "mp4" && !validMP4Height(req.Resolution) {
		req.Resolution = "1080"
	}
	if !validCookiesBrowser(req.CookiesBrowser) || req.CookiesBrowser == "" {
		req.CookiesBrowser = savedAuthBrowser()
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
		s.doDownload(ctx, req.URL, destRoot, req.Format, req.Bitrate, req.Resolution, req.CookiesBrowser)
	}()
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *server) doDownload(ctx context.Context, url, destRoot, format, bitrate, resolution, cookiesBrowser string) {
	s.emit(fmt.Sprintf("——— %s ———", time.Now().Format("2006-01-02 15:04:05")))
	s.emit("URL: " + url)
	extra := ""
	if format == "mp3" {
		extra = " @ " + bitrate + " kbps"
	} else if format == "mp4" {
		extra = " @ " + mp4HeightLabel(resolution)
	}
	s.emit("Formátum: " + strings.ToUpper(format) + extra)
	cookiesPath := cookiesFilePath(cookiesBrowser)
	if cookiesBrowser == "" || !fileExists(cookiesPath) {
		s.emit("Nincs mentett YouTube belépés. Kattints a Bejelentkezés gombra, jelentkezz be, majd a Belépés kész gombra.")
		s.emit("EVENT:FAIL")
		return
	}
	s.emit("YouTube fiók: mentett sütik (" + cookiesBrowser + ")")
	if !hasJSRuntime(s.ytDlp) {
		s.emit("FIGYELEM: nincs JavaScript runtime (Deno/Node) — az age-restricted / aláírásos YouTube videók bukhatnak.")
	} else {
		jsArgs := jsRuntimeArgs(s.ytDlp)
		if len(jsArgs) >= 2 {
			s.emit("JS runtime: " + jsArgs[1])
		}
	}

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

	overwrite := false
	if format == "mp4" && !playlist {
		probe, err := s.videoFormats(ctx, url, outDir, cookiesPath)
		if err == nil {
			available := bestHeightAtMost(probe.Heights, resolution)
			have := s.fileVideoHeight(withResolution(probe.FilePath, available))
			if have > 0 && available > 0 && have >= available {
				s.emit(fmt.Sprintf("Már megvan (%dp).", have))
				s.emit("EVENT:OK:" + outDir)
				return
			}
			if have > 0 && available > have {
				s.emit(fmt.Sprintf("A mentett %dp helyére %dp kerül.", have, available))
				overwrite = true
			}
		}
	}
	args := s.buildArgs(url, outDir, format, bitrate, resolution, cookiesPath, playlist, overwrite)
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

func validCookiesBrowser(name string) bool {
	switch name {
	case "", "chrome", "brave":
		return true
	default:
		return false
	}
}

func validMP4Height(h string) bool {
	switch h {
	case "720", "1080", "1440", "2160":
		return true
	default:
		return false
	}
}

func mp4HeightLabel(h string) string {
	switch h {
	case "720":
		return "720p"
	case "1440":
		return "2K (1440p)"
	case "2160":
		return "4K (2160p)"
	default:
		return "1080p"
	}
}

func withResolution(path string, height int) string {
	if path == "" || height <= 0 {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	return fmt.Sprintf("%s [%dp]%s", base, height, ext)
}

func ffprobeBeside(ffmpeg string) string {
	name := "ffprobe"
	if runtime.GOOS == "windows" {
		name = "ffprobe.exe"
	}
	return filepath.Join(filepath.Dir(ffmpeg), name)
}

func (s *server) buildArgs(url, outDir, format, bitrate, resolution, cookiesPath string, playlist, overwrite bool) []string {
	var outTpl string
	if playlist {
		// Playlist: playlist mappa / videónként saját alkönyvtár
		outTpl = filepath.Join(
			outDir,
			"%(playlist)s",
			"%(title)s [%(id)s]",
			"%(title)s.%(ext)s",
		)
	} else if format == "mp4" {
		outTpl = filepath.Join(outDir, "%(title)s [%(height)sp].%(ext)s")
	} else {
		outTpl = filepath.Join(outDir, "%(title)s.%(ext)s")
	}
	if playlist && format == "mp4" {
		outTpl = filepath.Join(
			outDir,
			"%(playlist)s",
			"%(title)s [%(id)s]",
			"%(title)s [%(height)sp].%(ext)s",
		)
	}
	common := []string{
		"--ffmpeg-location", s.ffmpeg,
		"--force-ipv4",
		"--newline",
		"-o", outTpl,
	}
	if playlist {
		common = append([]string{"--yes-playlist"}, common...)
	} else {
		common = append([]string{"--no-playlist"}, common...)
	}
	if cookiesPath != "" {
		common = append(common, "--cookies", cookiesPath)
	}
	if overwrite {
		common = append(common, "--force-overwrites")
	}
	common = append(common, jsRuntimeArgs(s.ytDlp)...)
	// Korhatáros videón a https-sávok gyakran hiányoznak, a web_safari HLS viszont megadja az 1080-at.
	common = append(common, "--extractor-args", "youtube:player_client=default,web_safari")
	switch format {
	case "mp4":
		h := resolution
		if !validMP4Height(h) {
			h = "1080"
		}
		selector := fmt.Sprintf("bestvideo*[height<=%s]+bestaudio/best[height<=%s]/best", h, h)
		return append(common,
			"-f", selector,
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
			"--embed-metadata",
			"--embed-thumbnail",
			"--convert-thumbnails", "jpg",
			"--parse-metadata", "%(artist,creator,uploader)s:%(meta_artist)s",
			"--parse-metadata", "%(album,playlist_title,channel,uploader)s:%(meta_album)s",
			"--parse-metadata", "%(title)s:%(meta_title)s",
			"--parse-metadata", "%(upload_date>%Y)s:%(meta_date)s",
			"--parse-metadata", ":(?P<meta_description>)",
			"--parse-metadata", ":(?P<meta_comment>)",
			"--postprocessor-args", fmt.Sprintf("ExtractAudio+ffmpeg:-c:a libmp3lame -b:a %sk -threads 4", br),
			"--postprocessor-args", "Metadata+ffmpeg:-id3v2_version 3 -write_id3v1 0",
			"--postprocessor-args", "EmbedThumbnail+ffmpeg:-id3v2_version 3",
			"--write-info-json",
			"--print", "after_move:MP3\t%(filepath)s",
			url,
		)
	}
}

func (s *server) runCmd(ctx context.Context, name string, args ...string) int {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
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
	sc.Split(splitCRLF)
	var lastProgress string
	var covers []string
	for sc.Scan() {
		line := strings.TrimSpace(decodeOutput(sc.Bytes()))
		if line == "" {
			continue
		}
		if i := strings.Index(line, "MP3\t"); i >= 0 {
			// A fájl yt-dlp kilépéséig zárolt maradhat; a cserét utána végezzük.
			path := strings.Trim(strings.TrimSpace(line[i+len("MP3\t"):]), `"`)
			if path != "" && path != "NA" {
				covers = append(covers, path)
			}
			continue
		}
		if path := embedThumbPath(line); path != "" {
			covers = append(covers, path)
			continue
		}
		short, progress := shortenYtLine(line)
		if progress {
			if short == "" || short == lastProgress {
				continue
			}
			lastProgress = short
		}
		if short == "" {
			continue
		}
		s.emit(short)
	}
	waitErr := cmd.Wait()
	seen := map[string]bool{}
	for _, path := range covers {
		if seen[path] {
			continue
		}
		seen[path] = true
		s.squareEmbeddedCover(path)
	}
	if waitErr != nil {
		if ee, ok := waitErr.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return 1
	}
	return 0
}

func decodeOutput(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	sb.Grow(len(b))
	for _, c := range b {
		if c < 0x80 {
			sb.WriteByte(c)
			continue
		}
		sb.WriteRune(cp1250[c-0x80])
	}
	return sb.String()
}

// Windows-1250, a magyar rendszerkódlap. Az á 0xE1, ezért a naplóban nem �.
var cp1250 = [128]rune{
	0x20AC, 0x0081, 0x201A, 0x0083, 0x201E, 0x2026, 0x2020, 0x2021,
	0x0089, 0x2030, 0x0160, 0x2039, 0x015A, 0x0164, 0x017D, 0x0179,
	0x0090, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014,
	0x0099, 0x2122, 0x0161, 0x203A, 0x015B, 0x0165, 0x017E, 0x017A,
	0x00A0, 0x02C7, 0x02D8, 0x0141, 0x00A4, 0x0104, 0x00A6, 0x00A7,
	0x00A8, 0x00A9, 0x015E, 0x00AB, 0x00AC, 0x00AD, 0x00AE, 0x017B,
	0x00B0, 0x00B1, 0x02DB, 0x0142, 0x00B4, 0x00B5, 0x00B6, 0x00B7,
	0x00B8, 0x0105, 0x015F, 0x00BB, 0x013D, 0x02DD, 0x013E, 0x017C,
	0x0154, 0x00C1, 0x00C2, 0x0102, 0x00C4, 0x0139, 0x0106, 0x00C7,
	0x010C, 0x00C9, 0x0118, 0x00CB, 0x011A, 0x00CD, 0x00CE, 0x010E,
	0x0110, 0x0143, 0x0147, 0x00D3, 0x00D4, 0x0150, 0x00D6, 0x00D7,
	0x0158, 0x016E, 0x00DA, 0x0170, 0x00DC, 0x00DD, 0x0162, 0x00DF,
	0x0155, 0x00E1, 0x00E2, 0x0103, 0x00E4, 0x013A, 0x0107, 0x00E7,
	0x010D, 0x00E9, 0x0119, 0x00EB, 0x011B, 0x00ED, 0x00EE, 0x010F,
	0x0111, 0x0144, 0x0148, 0x00F3, 0x00F4, 0x0151, 0x00F6, 0x00F7,
	0x0159, 0x016F, 0x00FA, 0x0171, 0x00FC, 0x00FD, 0x0163, 0x02D9,
}

func splitCRLF(data []byte, atEOF bool) (int, []byte, error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

var (
	dlPercentRe = regexp.MustCompile(`^\[download\]\s+([\d.]+)%`)
	dlSpeedRe   = regexp.MustCompile(`\bat\s+(\S+)`)
	dlETARe     = regexp.MustCompile(`\bETA\s+(\S+)`)
)

func shortenYtLine(line string) (string, bool) {
	if m := dlPercentRe.FindStringSubmatch(line); m != nil {
		pct := 0.0
		fmt.Sscanf(m[1], "%f", &pct)
		bar := progressBarLine(pct)
		if eta := dlETARe.FindStringSubmatch(line); eta != nil && realETA(eta[1]) {
			bar += " ETA " + eta[1]
		}
		speed := ""
		if sp := dlSpeedRe.FindStringSubmatch(line); sp != nil {
			speed = sp[1]
		}
		return bar + "\t" + speed + "\t" + m[1], true
	}
	if strings.Contains(line, "PO Token") || strings.Contains(line, "po_token") {
		return "Egyes sávok kimaradnak.", false
	}
	if strings.Contains(line, "Total fragments") || strings.Contains(line, "Downloading m3u8") {
		return "", false
	}
	if rest, ok := strings.CutPrefix(line, "[download] Destination: "); ok {
		return "Fájl: " + filepath.Base(strings.Trim(rest, `"`)), false
	}
	if strings.HasPrefix(line, "[download] Sleeping") {
		return "Várakozás…", false
	}
	if strings.HasPrefix(line, "[FixupM3u8]") {
		return "Összefűzés…", false
	}
	return line, false
}

func squareSide(height int) int {
	switch {
	case height <= 0:
		return 0
	case height >= 1080:
		return 1000
	case height >= 720:
		return 500
	default:
		return height
	}
}

func (s *server) squareEmbeddedCover(mp3Path string) {
	infoPath := strings.TrimSuffix(mp3Path, filepath.Ext(mp3Path)) + ".info.json"
	if !fileExists(infoPath) {
		alt := mp3Path + ".info.json"
		if fileExists(alt) {
			infoPath = alt
		}
	}
	height := 0
	if fileExists(infoPath) {
		height = videoHeightFromInfo(infoPath)
	}
	side := squareSide(height)
	if side == 0 {
		s.emit("Borító: a videó magassága nem olvasható.")
		return
	}
	if err := replaceMP3Cover(s.ffmpeg, mp3Path, side); err != nil {
		msg := strings.TrimSpace(err.Error())
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = strings.TrimSpace(msg[:i])
		}
		if msg == "" {
			msg = "ismeretlen hiba"
		}
		s.emit("Borító csere sikertelen: " + msg)
		return
	}
	if fileExists(infoPath) {
		_ = os.Remove(infoPath)
	}
	s.emit(fmt.Sprintf("Borító: %d×%d.", side, side))
}

func embedThumbPath(line string) string {
	const key = "Adding thumbnail to "
	i := strings.Index(line, key)
	if i < 0 {
		return ""
	}
	path := strings.Trim(strings.TrimSpace(line[i+len(key):]), `"`)
	if path == "" || path == "NA" || !strings.EqualFold(filepath.Ext(path), ".mp3") {
		return ""
	}
	return path
}

func videoHeightFromInfo(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var info struct {
		Height  float64 `json:"height"`
		Formats []struct {
			Height int    `json:"height"`
			VCodec string `json:"vcodec"`
		} `json:"formats"`
	}
	if json.Unmarshal(raw, &info) != nil {
		return 0
	}
	best := int(info.Height)
	for _, f := range info.Formats {
		if f.Height > best && f.VCodec != "" && f.VCodec != "none" && !strings.Contains(strings.ToLower(f.VCodec), "images") {
			best = f.Height
		}
	}
	return best
}

func cropSquareJPEG(src []byte, side int) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 {
		return nil, fmt.Errorf("üres borítókép")
	}
	n := w
	if h < n {
		n = h
	}
	srcRect := image.Rect(b.Min.X+(w-n)/2, b.Min.Y+(h-n)/2, 0, 0)
	srcRect.Max = image.Point{srcRect.Min.X + n, srcRect.Min.Y + n}
	cropped := image.NewRGBA(image.Rect(0, 0, n, n))
	xdraw.Draw(cropped, cropped.Bounds(), img, srcRect.Min, xdraw.Src)
	dst := image.NewRGBA(image.Rect(0, 0, side, side))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), cropped, cropped.Bounds(), xdraw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func replaceMP3Cover(ffmpeg, mp3Path string, side int) error {
	jpg := mp3Path + ".cover.jpg"
	sq := mp3Path + ".cover.sq.jpg"
	tmp := mp3Path + ".cover.mp3"
	_ = os.Remove(jpg)
	_ = os.Remove(sq)
	_ = os.Remove(tmp)
	defer os.Remove(jpg)
	defer os.Remove(sq)
	extract := exec.Command(ffmpeg,
		"-y", "-hide_banner", "-loglevel", "error",
		"-i", mp3Path,
		"-map", "0:v:0",
		"-frames:v", "1",
		"-q:v", "2",
		jpg,
	)
	hideWindow(extract)
	if out, err := extract.CombinedOutput(); err != nil {
		if len(out) > 0 {
			return fmt.Errorf("%s", strings.TrimSpace(string(out)))
		}
		return err
	}
	raw, err := os.ReadFile(jpg)
	if err != nil {
		return err
	}
	squared, err := cropSquareJPEG(raw, side)
	if err != nil {
		return err
	}
	if err := os.WriteFile(sq, squared, 0o644); err != nil {
		return err
	}
	// Csak a hang megy át. A régi képsáv és a régi borítókeret nem.
	cmd := exec.Command(ffmpeg,
		"-y", "-hide_banner", "-loglevel", "error",
		"-i", mp3Path,
		"-map", "0:a:0",
		"-c:a", "copy",
		"-map_metadata", "0",
		"-id3v2_version", "3",
		"-write_id3v1", "0",
		tmp,
	)
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(tmp)
		if len(out) > 0 {
			return fmt.Errorf("%s", strings.TrimSpace(string(out)))
		}
		return err
	}
	if err := writeSquareAPIC(tmp, squared); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	bak := mp3Path + ".bak"
	var renameErr error
	for attempt := 0; attempt < 8; attempt++ {
		_ = os.Remove(bak)
		renameErr = os.Rename(mp3Path, bak)
		if renameErr == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if renameErr != nil {
		_ = os.Remove(tmp)
		return renameErr
	}
	if err := os.Rename(tmp, mp3Path); err != nil {
		_ = os.Rename(bak, mp3Path)
		_ = os.Remove(tmp)
		return err
	}
	return os.Remove(bak)
}

func writeSquareAPIC(path string, jpeg []byte) error {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return err
	}
	defer tag.Close()
	tag.SetVersion(3)
	tag.DeleteFrames(tag.CommonID("Attached picture"))
	tag.AddAttachedPicture(id3v2.PictureFrame{
		Encoding:    id3v2.EncodingISO,
		MimeType:    "image/jpeg",
		PictureType: id3v2.PTFrontCover,
		Description: "Cover",
		Picture:     jpeg,
	})
	return tag.Save()
}

func realETA(eta string) bool {
	if eta == "" || eta == "Unknown" || eta == "NA" {
		return false
	}
	// A 00:00 nem hátralévő idő, hanem üres minta két sáv vagy töredék között.
	return eta != "00:00" && eta != "00:00:00"
}

func progressBarLine(pct float64) string {
	const width = 28
	filled := int(math.Round(pct / 100 * float64(width)))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
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
