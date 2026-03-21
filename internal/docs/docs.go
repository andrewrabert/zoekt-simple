package docs

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openapiSpec []byte

const docsPage = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>zoekt-server API</title>
  <link rel="stylesheet" href="/static/stoplight-elements.min.css">
</head>
<body>
  <elements-api
    apiDescriptionUrl="/docs/openapi.yaml"
    router="hash"
    layout="sidebar"
    tryItCredentialsPolicy="same-origin"
  />
  <script src="/static/stoplight-elements.min.js"></script>
</body>
</html>`

func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /docs/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(docsPage))
	})
	mux.HandleFunc("GET /docs/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Write(openapiSpec)
	})
}
