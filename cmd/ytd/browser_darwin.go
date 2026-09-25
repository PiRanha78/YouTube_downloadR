//go:build darwin

package main

import (
	"os/exec"
	"strings"
)

func detectOpenBrowser() string {
	out, err := exec.Command("ps", "-axc", "-o", "comm=").Output()
	if err != nil {
		return ""
	}
	var chrome, brave bool
	for _, line := range strings.Split(string(out), "\n") {
		switch strings.TrimSpace(line) {
		case "Google Chrome", "Google Chrome Helper", "Chromium":
			chrome = true
		case "Brave Browser", "Brave Browser Helper":
			brave = true
		}
	}
	// Futó böngésző: Chrome előnyben, ha mindkettő megy.
	if chrome {
		if findBrowserExe("chrome") != "" {
			return "chrome"
		}
	}
	if brave {
		if findBrowserExe("brave") != "" {
			return "brave"
		}
	}
	return ""
}
