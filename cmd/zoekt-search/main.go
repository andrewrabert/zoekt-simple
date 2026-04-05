// Command zoekt-search searches indexed repositories via the zoekt API.
//
// Deprecated: Use "zoekt-unified search" instead.
package main

import (
	"os"

	"github.com/sourcegraph/zoekt-simple/internal/cli"
)

func main() {
	cli.RunSearch(os.Args[1:])
}
