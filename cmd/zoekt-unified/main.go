// Command zoekt-unified provides a single binary with subcommands for all
// zoekt-simple CLI tools.
//
// Usage:
//
//	zoekt-unified serve        # was zoekt-server
//	zoekt-unified search       # was zoekt-search
//	zoekt-unified get-file     # was zoekt-get-file
//	zoekt-unified git-clone    # was zoekt-simple-git-clone
package main

import (
	"os"

	"github.com/sourcegraph/zoekt-simple/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
