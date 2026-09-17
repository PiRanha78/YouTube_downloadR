//go:build darwin && cgo

package main

import (
	webview "github.com/webview/webview_go"
)

var terminateUI func()

func runNativeUI(url string) {
	w := webview.New(false)
	if w == nil {
		openMacFallbackWindow(url)
		waitForever()
		return
	}
	terminateUI = func() {
		w.Terminate()
	}
	w.SetTitle(appTitle + " " + appVersion)
	w.SetSize(820, 760, webview.HintNone)
	w.Navigate(url)
	w.Run()
	w.Destroy()
	terminateUI = nil
}
