// Command zoekt-get-file retrieves a file from a zoekt server.
//
// Deprecated: Use "zoekt-unified get-file" instead.
package main

import (
	"os"

	"github.com/sourcegraph/zoekt-simple/internal/cli"
)

func main() {
	cli.RunGetFile(os.Args[1:])
}
