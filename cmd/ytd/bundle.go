package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const bundledStamp = "bundle-1.3.1"

func runtimeToolsDir() (string, error) {
	var root string
	switch runtime.GOOS {
	case "windows":
		root = os.Getenv("LOCALAPPDATA")
		if root == "" {
			root = os.TempDir()
		}
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, "Library", "Application Support")
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(root, "YouTube_DownloadR", "runtime", bundledStamp), nil
}

// ensureBundledTools kicsomagolja a beágyazott yt-dlp + ffmpeg csomagot, ha még nincs meg.
func ensureBundledTools() (ytDlp, ffmpeg string, err error) {
	dir, err := runtimeToolsDir()
	if err != nil {
		return "", "", err
	}
	ytName := "yt-dlp"
	ffName := "ffmpeg"
	if runtime.GOOS == "windows" {
		ytName += ".exe"
		ffName += ".exe"
	}
	ytPath := filepath.Join(dir, ytName)
	ffPath := filepath.Join(dir, ffName)
	stampPath := filepath.Join(dir, ".ok")

	if fileExists(ytPath) && fileExists(ffPath) && fileExists(stampPath) {
		return ytPath, ffPath, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("runtime mappa: %w", err)
	}
	if err := extractBundledZip(dir); err != nil {
		return "", "", fmt.Errorf("csomag kibontása: %w", err)
	}
	// macOS: végrehajtható bit biztosítása
	_ = os.Chmod(ytPath, 0o755)
	_ = os.Chmod(ffPath, 0o755)
	if probe := filepath.Join(dir, "ffprobe"); fileExists(probe) {
		_ = os.Chmod(probe, 0o755)
	}
	if probe := filepath.Join(dir, "ffprobe.exe"); fileExists(probe) {
		_ = os.Chmod(probe, 0o755)
	}
	if !fileExists(ytPath) || !fileExists(ffPath) {
		return "", "", fmt.Errorf("a csomag nem tartalmazza a %s / %s fájlokat", ytName, ffName)
	}
	_ = os.WriteFile(stampPath, []byte(bundledStamp), 0o644)
	return ytPath, ffPath, nil
}

func extractBundledZip(destDir string) error {
	raw, err := bundledZip.ReadFile(bundledZipPath)
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(f.Name)
		if name == "." || name == ".." {
			continue
		}
		target := filepath.Join(destDir, name)
		if err := writeZipFile(f, target); err != nil {
			return err
		}
	}
	return nil
}

func writeZipFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	tmp := target + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, rc)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	_ = os.Remove(target)
	return os.Rename(tmp, target)
}
