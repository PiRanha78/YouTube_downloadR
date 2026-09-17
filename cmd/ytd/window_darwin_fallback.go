//go:build darwin

package main

import (
	"os"
	"os/exec"
)

func openMacFallbackWindow(url string) {
	candidates := []struct {
		app  string
		args []string
	}{
		{"Google Chrome", []string{"-na", "Google Chrome", "--args", "--app=" + url}},
		{"Microsoft Edge", []string{"-na", "Microsoft Edge", "--args", "--app=" + url}},
		{"Chromium", []string{"-na", "Chromium", "--args", "--app=" + url}},
		{"Brave Browser", []string{"-na", "Brave Browser", "--args", "--app=" + url}},
	}
	for _, c := range candidates {
		path := "/Applications/" + c.app + ".app"
		if st, err := os.Stat(path); err == nil && st.IsDir() {
			if err := exec.Command("open", c.args...).Start(); err == nil {
				return
			}
		}
	}
	openBrowser(url)
}

func waitForever() {
	select {}
}
