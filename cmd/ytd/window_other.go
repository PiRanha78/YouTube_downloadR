//go:build !windows

package main

import (
	"os"
	"os/signal"
)

var terminateUI func()

// Fejlesztéshez (macOS/Linux): böngésző, amíg natív WebView2 csak Windows-on van.
func runNativeUI(url string) {
	openBrowser(url)
	terminateUI = func() {
		os.Exit(0)
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
}
