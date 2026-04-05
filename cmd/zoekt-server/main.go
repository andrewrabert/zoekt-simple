// Command zoekt-server starts the zoekt-simple web server and indexer.
//
// Deprecated: Use "zoekt-unified serve" instead.
package main

import (
	"flag"
	"os"

	"github.com/sourcegraph/zoekt-simple/internal/cli"
)

func envDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func main() {
	configFile := flag.String("config", envDefault("ZOEKT_CONFIG", ""), "path to YAML config file (env: ZOEKT_CONFIG)")
	listen := flag.String("listen", envDefault("ZOEKT_LISTEN", ""), "override listen address (env: ZOEKT_LISTEN)")
	flag.Parse()

	cli.RunServe(*configFile, *listen)
}
