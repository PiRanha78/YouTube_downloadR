//go:build darwin && !cgo

package main

import "os"

var terminateUI func()

// CGO nélküli build (pl. cross-compile): app-ablak / böngésző.
func runNativeUI(url string) {
	terminateUI = func() {
		os.Exit(0)
	}
	openMacFallbackWindow(url)
	waitForever()
}
