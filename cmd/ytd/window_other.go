//go:build !windows && !darwin

package main

import (
	"os"
	"os/signal"
)

var terminateUI func()

func runNativeUI(url string) {
	openBrowser(url)
	terminateUI = func() {
		os.Exit(0)
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
}
