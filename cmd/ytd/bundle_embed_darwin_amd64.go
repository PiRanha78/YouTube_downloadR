//go:build darwin && amd64

package main

import "embed"

//go:embed bundled/tools_darwin_amd64.zip
var bundledZip embed.FS

const bundledZipPath = "bundled/tools_darwin_amd64.zip"
