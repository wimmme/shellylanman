// Package web embeds the built frontend.
//
// The Docker build copies web/dist here before compiling. In a plain checkout
// dist holds only .keep, and the server answers with a short "frontend not
// built" page instead of the UI; the API works either way.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Files is the frontend's file system, rooted at dist.
func Files() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // the embed pattern guarantees dist exists
	}
	return sub
}
