package gateway

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var embeddedStaticFS embed.FS

func embeddedFileServer() http.Handler {
	sub, err := fs.Sub(embeddedStaticFS, "static")
	if err != nil {
		// Fallback to empty FS; caller will handle 404s
		return http.FileServer(http.FS(embeddedStaticFS))
	}
	return http.FileServer(http.FS(sub))
}
