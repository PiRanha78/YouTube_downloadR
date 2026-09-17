//go:build darwin && arm64

package main

import "embed"

//go:embed bundled/tools_darwin_arm64.zip
var bundledZip embed.FS

const bundledZipPath = "bundled/tools_darwin_arm64.zip"
