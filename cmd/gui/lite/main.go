//go:build windows

package main

import (
	"embed"

	"github.com/autocambar/autocambar/cmd/gui/internal/backend"
)

//go:embed all:frontend/dist-lite
var assets embed.FS

//go:embed assets/icon.png
var appIcon []byte

func main() {
	backend.RunGUI(true, assets, appIcon)
}
