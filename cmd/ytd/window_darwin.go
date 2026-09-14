//go:build darwin

package main

import (
	webview "github.com/webview/webview_go"
)

var terminateUI func()

func runNativeUI(url string) {
	w := webview.New(false)
	if w == nil {
		// Tartalék, ha a WebKit binding nem jön létre
		openBrowser(url)
		select {}
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
