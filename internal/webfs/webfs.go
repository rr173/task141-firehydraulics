// Package webfs embeds the static frontend so both the main binary and the
// selfcheck smoke test serve the same page without duplicating the //go:embed
// directive (the directive cannot escape its package directory, so the web
// source must live under this package).
package webfs

import (
	"embed"
	"io/fs"
)

//go:embed web
var web embed.FS

// FS returns the embedded web/ filesystem rooted at the directory itself (so
// "index.html", "app.js" and "style.css" are top-level entries). Callers wrap
// it with http.FS to get an http.FileSystem.
func FS() fs.FS {
	sub, err := fs.Sub(web, "web")
	if err != nil {
		// fs.Sub only errors if the name is malformed; "web" is a valid dir.
		panic(err)
	}
	return sub
}
