//go:build windows

package main

import "embed"

//go:embed bundled/tools_windows.zip
var bundledZip embed.FS

const bundledZipPath = "bundled/tools_windows.zip"
