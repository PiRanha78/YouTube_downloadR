package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	netURL "net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type formatProbe struct {
	URL      string
	Dest     string
	Heights  []int
	FilePath string
	Thumb    string
}

type probeHTTPResp struct {
	OK        bool   `json:"ok"`
	NeedLogin bool   `json:"needLogin,omitempty"`
	Playlist  bool   `json:"playlist,omitempty"`
	Message   string `json:"message"`
	Heights   []int  `json:"heights,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
}

func (s *server) handleProbe(w http.ResponseWriter, r *http.Request) {
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
	req.Resolution = strings.TrimSpace(req.Resolution)
	if !validMP4Height(req.Resolution) {
		req.Resolution = "1080"
	}
	dest := strings.TrimSpace(req.DestRoot)
	if dest == "" {
		dest = defaultDownloadsDir(s.base)
	}
	if req.URL == "" {
		writeJSON(w, probeHTTPResp{OK: true, Message: ""})
		return
	}
	if isPlaylistURL(req.URL) {
		writeJSON(w, probeHTTPResp{
			OK:       true,
			Playlist: true,
			Message:  "Lejátszási lista: a felbontás videónként a letöltéskor dől el.",
		})
		return
	}
	browser := savedAuthBrowser()
	cookies := cookiesFilePath(browser)
	if browser == "" || !fileExists(cookies) {
		writeJSON(w, probeHTTPResp{
			OK:        true,
			NeedLogin: true,
			Message:   "A felbontás ellenőrzéséhez előbb jelentkezz be.",
		})
		return
	}
	s.mu.Lock()
	busy := s.busy
	s.mu.Unlock()
	if busy {
		writeJSON(w, probeHTTPResp{OK: false, Message: ""})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	probe, err := s.videoFormats(ctx, req.URL, dest, cookies)
	if err != nil {
		writeJSON(w, probeHTTPResp{OK: false, Message: probeFailMessage(err.Error())})
		return
	}
	avail := maxHeight(probe.Heights)
	writeJSON(w, probeHTTPResp{
		OK:        true,
		Message:   availableHeightMessage(avail),
		Heights:   probe.Heights,
		Thumbnail: probe.Thumb,
	})
}

func (s *server) videoFormats(ctx context.Context, url, dest, cookies string) (formatProbe, error) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if s.cachedProbe != nil && s.cachedProbe.URL == url && s.cachedProbe.Dest == dest && len(s.cachedProbe.Heights) > 0 {
		return *s.cachedProbe, nil
	}
	probe, err := s.fetchVideoFormats(ctx, url, dest, cookies)
	if err != nil {
		return formatProbe{}, err
	}
	s.cachedProbe = &probe
	return probe, nil
}

func (s *server) fetchVideoFormats(ctx context.Context, url, dest, cookies string) (formatProbe, error) {
	outTpl := filepath.Join(dest, "%(title)s.%(ext)s")
	args := []string{
		"--skip-download",
		"--no-playlist",
		"--no-warnings",
		"--force-ipv4",
		"--ffmpeg-location", s.ffmpeg,
		"--cookies", cookies,
		"-o", outTpl,
		"--merge-output-format", "mp4",
		"--print", "FILE %(filename)s",
		"-J",
	}
	args = append(args, jsRuntimeArgs(s.ytDlp)...)
	args = append(args, "--extractor-args", "youtube:player_client=default,web_safari", url)
	cmd := exec.CommandContext(ctx, s.ytDlp, args...)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return formatProbe{}, fmt.Errorf("%s", msg)
	}
	filePath, heights, thumb, err := parseProbeOutput(out, dest)
	if err != nil {
		return formatProbe{}, err
	}
	return formatProbe{URL: url, Dest: dest, Heights: heights, FilePath: filePath, Thumb: thumb}, nil
}

func parseProbeOutput(out []byte, dest string) (string, []int, string, error) {
	text := string(out)
	filePath := ""
	jsonAt := strings.Index(text, "{")
	head := text
	body := text
	if jsonAt >= 0 {
		head = text[:jsonAt]
		body = text[jsonAt:]
	}
	for _, line := range strings.Split(head, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "FILE ") {
			filePath = strings.TrimSpace(strings.TrimPrefix(line, "FILE "))
		}
	}
	if filePath != "" && !filepath.IsAbs(filePath) {
		filePath = filepath.Join(dest, filePath)
	}
	var info struct {
		Thumbnail string `json:"thumbnail"`
		Formats   []struct {
			Height     int    `json:"height"`
			VCodec     string `json:"vcodec"`
			FormatNote string `json:"format_note"`
		} `json:"formats"`
	}
	if err := json.Unmarshal([]byte(body), &info); err != nil {
		return "", nil, "", fmt.Errorf("a felbontáslista nem olvasható")
	}
	seen := map[int]struct{}{}
	var heights []int
	for _, f := range info.Formats {
		if f.Height <= 0 || f.VCodec == "" || f.VCodec == "none" {
			continue
		}
		if strings.Contains(strings.ToLower(f.FormatNote), "storyboard") {
			continue
		}
		if _, ok := seen[f.Height]; ok {
			continue
		}
		seen[f.Height] = struct{}{}
		heights = append(heights, f.Height)
	}
	sort.Ints(heights)
	if len(heights) == 0 {
		return "", nil, "", fmt.Errorf("nincs videósáv")
	}
	return filePath, heights, youtubeThumb(info.Thumbnail), nil
}

func (s *server) handleThumb(w http.ResponseWriter, r *http.Request) {
	thumb := youtubeThumb(r.URL.Query().Get("u"))
	if thumb == "" {
		http.NotFound(w, r)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, thumb, nil)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		http.NotFound(w, r)
		return
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	_, _ = io.Copy(w, resp.Body)
}

func youtubeThumb(raw string) string {
	u, err := netURL.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host == "i.ytimg.com" || host == "i9.ytimg.com" || strings.HasSuffix(host, ".ytimg.com") {
		return u.String()
	}
	return ""
}

func bestHeightAtMost(heights []int, requested string) int {
	capH, _ := strconv.Atoi(requested)
	if capH <= 0 {
		capH = 1080
	}
	best := 0
	for _, h := range heights {
		if h <= capH && h > best {
			best = h
		}
	}
	if best > 0 {
		return best
	}
	for _, h := range heights {
		if best == 0 || h < best {
			best = h
		}
	}
	return best
}

func maxHeight(heights []int) int {
	best := 0
	for _, h := range heights {
		if h > best {
			best = h
		}
	}
	return best
}

func availableHeightMessage(available int) string {
	if available <= 0 {
		return "A felbontást nem sikerült ellenőrizni."
	}
	return fmt.Sprintf("Elérhető: %dp.", available)
}

func probeFailMessage(raw string) string {
	low := strings.ToLower(raw)
	if strings.Contains(low, "sign in") || strings.Contains(low, "age") || strings.Contains(low, "cookie") || strings.Contains(low, "login") {
		return "A belépés lejárt vagy a videó nem érhető el. Jelentkezz be újra."
	}
	return "A felbontást nem sikerült ellenőrizni."
}

func (s *server) fileVideoHeight(path string) int {
	if path == "" || !fileExists(path) {
		return 0
	}
	probe := s.ffprobe
	if probe == "" {
		name := "ffprobe"
		if runtime.GOOS == "windows" {
			name = "ffprobe.exe"
		}
		probe = filepath.Join(filepath.Dir(s.ffmpeg), name)
	}
	if !fileExists(probe) {
		return 0
	}
	cmd := exec.Command(probe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=height", "-of", "csv=p=0", path)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	line := strings.TrimSpace(string(bytes.Split(out, []byte("\n"))[0]))
	h, _ := strconv.Atoi(line)
	return h
}
