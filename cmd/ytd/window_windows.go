//go:build windows

package main

import (
	"github.com/jchv/go-webview2"
)

var terminateUI func()

func runNativeUI(url string) {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  appTitle + " " + appVersion,
			Width:  820,
			Height: 760,
			Center: true,
		},
	})
	if w == nil {
		fatalPopup("Nem sikerült megnyitni a natív ablakot.\n\n" +
			"Telepítsd a Microsoft Edge WebView2 Runtime-ot:\n" +
			"https://developer.microsoft.com/microsoft-edge/webview2/")
		return
	}
	terminateUI = func() {
		w.Terminate()
	}
	w.Navigate(url)
	w.Run()
	w.Destroy()
	terminateUI = nil
}
