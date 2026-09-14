//go:build !windows && !darwin

package main

import "embed"

// Linuxhez jelenleg nincs beágyazott csomag – külső binárisok kellenek.
//
//go:embed bundled/.keep
var bundledZip embed.FS

const bundledZipPath = "bundled/.keep"
