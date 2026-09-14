//go:build darwin

package main

import "embed"

//go:embed bundled/tools_darwin.zip
var bundledZip embed.FS

const bundledZipPath = "bundled/tools_darwin.zip"
