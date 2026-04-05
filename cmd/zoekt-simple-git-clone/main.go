// Command zoekt-simple-git-clone clones a git repository as a bare repo.
//
// Deprecated: Use "zoekt-unified git-clone" instead.
package main

import (
	"os"

	"github.com/sourcegraph/zoekt-simple/internal/cli"
)

func main() {
	cli.RunGitClone(os.Args[1:])
}
