package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func configPath() string {
	dir, err := runtimeToolsDir()
	if err != nil {
		return ""
	}
	// runtime/.../bundle -> parent YouTube_DownloadR
	root := filepath.Dir(filepath.Dir(dir))
	return filepath.Join(root, "config.json")
}

type appConfig struct {
	DestRoot string `json:"destRoot"`
}

func loadConfig() appConfig {
	p := configPath()
	if p == "" {
		return appConfig{}
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return appConfig{}
	}
	var c appConfig
	_ = json.Unmarshal(raw, &c)
	return c
}

func saveConfig(c appConfig) {
	p := configPath()
	if p == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(p, raw, 0o644)
}

func pickFolder(initial string) (string, error) {
	if initial == "" {
		initial = defaultDownloadsDir(appBaseDir())
	}
	switch runtime.GOOS {
	case "windows":
		return pickFolderWindows(initial)
	case "darwin":
		return pickFolderMac(initial)
	default:
		return pickFolderLinux(initial)
	}
}

func pickFolderWindows(initial string) (string, error) {
	// 1) Shell.Application BrowseForFolder (cscript) – megbízható GUI app mellett is
	if path, err := pickFolderWindowsVBS(initial); err == nil && path != "" {
		return path, nil
	}
	// 2) PowerShell FolderBrowserDialog – LÁTHATÓ folyamat (HideWindow nélkül!)
	return pickFolderWindowsPS(initial)
}

func pickFolderWindowsVBS(initial string) (string, error) {
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("ytd_pick_%d.vbs", time.Now().UnixNano()))
	// &H10 = BIF_NEWDIALOGSTYLE (új mappa gomb)
	root := `0`
	if initial != "" {
		// Útvonal gyökérként – VBS string escape
		esc := strings.ReplaceAll(initial, `\`, `\\`)
		esc = strings.ReplaceAll(esc, `"`, `""`)
		root = `"` + esc + `"`
	}
	script := fmt.Sprintf(`
Set shell = CreateObject("Shell.Application")
On Error Resume Next
Set folder = shell.BrowseForFolder(0, "Mentasi mappa kivalasztasa", &H10&, %s)
If folder Is Nothing Then
  WScript.Quit 1
End If
WScript.Echo folder.Self.Path
`, root)
	if err := os.WriteFile(tmp, []byte(script), 0o644); err != nil {
		return "", err
	}
	defer os.Remove(tmp)

	cmd := exec.Command("cscript", "//Nologo", tmp)
	// Ne rejtsük el: a dialógusnak meg kell jelennie
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("cancelled")
	}
	return path, nil
}

func pickFolderWindowsPS(initial string) (string, error) {
	init := strings.ReplaceAll(initial, `'`, `''`)
	script := `
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.Application]::EnableVisualStyles()
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = 'Mentási mappa kiválasztása'
$d.ShowNewFolderButton = $true
$init = '` + init + `'
if ($init -and (Test-Path -LiteralPath $init)) { $d.SelectedPath = $init }
$r = $d.ShowDialog()
if ($r -eq [System.Windows.Forms.DialogResult]::OK) {
  Write-Output $d.SelectedPath
  exit 0
}
exit 1
`
	// powershell.exe explicit + STA; HideWindow NÉLKÜL (különben nem jön fel a dialógus)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-STA", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("cancelled")
	}
	return path, nil
}

func pickFolderMac(initial string) (string, error) {
	script := `set theFolder to choose folder with prompt "Mentési mappa kiválasztása"`
	if initial != "" {
		script = `set theFolder to choose folder with prompt "Mentési mappa kiválasztása" default location POSIX file "` +
			strings.ReplaceAll(initial, `"`, `\"`) + `"`
	}
	script += "\nPOSIX path of theFolder"
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(strings.TrimSpace(string(out)), "/"), nil
}

func pickFolderLinux(initial string) (string, error) {
	args := []string{"--directory", "--title=Mentési mappa"}
	if initial != "" {
		args = append(args, "--filename="+initial+"/")
	}
	cmd := exec.Command("zenity", append([]string{"--file-selection"}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("kdialog", "--getexistingdirectory", initial)
		out, err = cmd.Output()
		if err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(string(out)), nil
}

func openFolder(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("üres útvonal")
	}
	path = filepath.Clean(path)
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		return fmt.Errorf("a mappa nem található: %s", path)
	}

	switch runtime.GOOS {
	case "windows":
		return openFolderWindows(path)
	case "darwin":
		cmd := exec.Command("open", path)
		return cmd.Start()
	default:
		cmd := exec.Command("xdg-open", path)
		return cmd.Start()
	}
}

func openFolderWindows(path string) error {
	// 1) explorer.exe – HideWindow NÉLKÜL (különben nem jön fel)
	cmd := exec.Command("explorer.exe", path)
	if err := cmd.Start(); err == nil {
		// explorer gyakran „hibával” lép ki, de az ablak megnyílik
		go func() { _ = cmd.Wait() }()
		return nil
	}

	// 2) tartalék: cmd start
	cmd = exec.Command("cmd.exe", "/c", "start", "", path)
	if err := cmd.Start(); err == nil {
		go func() { _ = cmd.Wait() }()
		return nil
	}

	// 3) tartalék: PowerShell Invoke-Item
	ps := strings.ReplaceAll(path, `'`, `''`)
	cmd = exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-Command",
		"Invoke-Item -LiteralPath '"+ps+"'",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mappa megnyitás sikertelen: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func defaultDownloadsDir(base string) string {
	cfg := loadConfig()
	if cfg.DestRoot != "" {
		return cfg.DestRoot
	}
	return filepath.Join(base, "downloads")
}
