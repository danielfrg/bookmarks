package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var frontend embed.FS

func Assets() (fs.FS, error) { return fs.Sub(frontend, "dist") }
