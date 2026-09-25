//go:build !windows && !darwin

package main

func detectOpenBrowser() string { return "" }
