package resources

import (
	"embed"
	"io/fs"
)

//go:embed static/*
var staticEmbed embed.FS

// StaticFS returns an fs.FS for serving embedded static files (the
// contents of the static/ directory). Wrap with http.FS when passing
// to http.FileServer.
func StaticFS() fs.FS {
	sub, err := fs.Sub(staticEmbed, "static")
	if err != nil {
		panic("resources: missing static directory: " + err.Error())
	}
	return sub
}
