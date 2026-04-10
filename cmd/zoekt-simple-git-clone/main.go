// Command zoekt-simple-git-clone clones a git repository as a bare repo.
// It extends zoekt-git-clone with a --dest-name flag that controls the
// filesystem directory name independently from -name (which sets zoekt.name).
package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/zoekt/gitindex"
)

func main() {
	dest := flag.String("dest", "", "destination directory")
	nameFlag := flag.String("name", "", "name of repository (sets zoekt.name)")
	destNameFlag := flag.String("dest-name", "", "directory name for the bare repo under -dest (overrides -name for filesystem path)")
	webURL := flag.String("web-url", "", "web-browsable URL for the repository (sets zoekt.web-url)")
	flag.Parse()

	if *dest == "" {
		log.Fatal("must set --dest")
	}
	if len(flag.Args()) == 0 {
		log.Fatal("must provide URL")
	}
	u, err := url.Parse(flag.Arg(0))
	if err != nil {
		log.Fatalf("url.Parse: %v", err)
	}

	name := *nameFlag
	if name == "" {
		name = filepath.Join(u.Host, u.Path)
		name = strings.TrimSuffix(name, ".git")
	}

	dirName := name
	if *destNameFlag != "" {
		dirName = *destNameFlag
	}

	destDir := filepath.Dir(filepath.Join(*dest, dirName))
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		log.Fatal(err)
	}

	config := map[string]string{
		"zoekt.name": name,
	}
	webURLSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "web-url" {
			webURLSet = true
		}
	})
	if webURLSet {
		// Explicit value (including empty to unset an existing web-url).
		config["zoekt.web-url"] = *webURL
	} else {
		config["zoekt.web-url"] = u.String()
	}

	destRepo, err := gitindex.CloneRepo(destDir, filepath.Base(dirName), u.String(), config)
	if err != nil {
		log.Fatalf("CloneRepo: %v", err)
	}
	if destRepo != "" {
		fmt.Println(destRepo)
	}
}
