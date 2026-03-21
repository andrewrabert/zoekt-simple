package static

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed bootstrap.min.css bootstrap.min.js jquery.min.js stoplight-elements.min.css stoplight-elements.min.js
var assets embed.FS

func RegisterHandlers(mux *http.ServeMux) {
	sub, _ := fs.Sub(assets, ".")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
}
