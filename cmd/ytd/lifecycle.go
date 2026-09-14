package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type instanceInfo struct {
	PID int    `json:"pid"`
	URL string `json:"url"`
}

func instanceFilePath() string {
	dir, err := runtimeToolsDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "YouTube_DownloadR_instance.json")
	}
	root := filepath.Dir(filepath.Dir(dir))
	return filepath.Join(root, "instance.json")
}

func trySingleInstance() (alreadyRunning bool, existingURL string) {
	path := instanceFilePath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, ""
	}
	var info instanceInfo
	if json.Unmarshal(raw, &info) != nil || info.URL == "" {
		_ = os.Remove(path)
		return false, ""
	}
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get(strings.TrimRight(info.URL, "/") + "/api/status")
	if err != nil {
		_ = os.Remove(path)
		return false, ""
	}
	_ = resp.Body.Close()
	return true, info.URL
}

func writeInstance(url string) error {
	path := instanceFilePath()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	info := instanceInfo{PID: os.Getpid(), URL: url}
	raw, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func clearInstance() {
	_ = os.Remove(instanceFilePath())
}

func (s *server) startIdleWatcher() {
	go func() {
		for {
			time.Sleep(2 * time.Second)
			s.clientsMu.Lock()
			n := len(s.clients)
			s.clientsMu.Unlock()

			s.mu.Lock()
			busy := s.busy
			had := s.hadClient
			s.mu.Unlock()

			if had && n == 0 && !busy {
				// Böngésző bezárva, nincs futó feladat → biztonságos leállás
				time.Sleep(1500 * time.Millisecond)
				s.clientsMu.Lock()
				n2 := len(s.clients)
				s.clientsMu.Unlock()
				s.mu.Lock()
				busy2 := s.busy
				s.mu.Unlock()
				if n2 == 0 && !busy2 {
					s.requestShutdown("böngésző bezárva")
					return
				}
			}
		}
	}()
}

func (s *server) markClient() {
	s.mu.Lock()
	s.hadClient = true
	s.mu.Unlock()
}

func (s *server) requestShutdown(reason string) {
	s.shutdownOnce.Do(func() {
		s.emit("Leállítás: " + reason)
		s.mu.Lock()
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
		clearInstance()
		go func() {
			time.Sleep(150 * time.Millisecond)
			if terminateUI != nil {
				terminateUI()
				return
			}
			if s.httpServer != nil {
				_ = s.httpServer.Close()
			}
			os.Exit(0)
		}()
	})
}

func (s *server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if !isLocalRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var body struct {
		Force bool `json:"force"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	s.mu.Lock()
	busy := s.busy
	s.mu.Unlock()
	if busy && !body.Force {
		writeJSON(w, map[string]any{
			"ok":      false,
			"busy":    true,
			"message": "Letöltés folyamatban. Erősítsd meg a kilépést.",
		})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
	s.requestShutdown("felhasználói kilépés")
}

func isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func onlyLocal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
