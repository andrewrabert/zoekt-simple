package cli

// This file registers all upstream zoekt commands as subcommands of the
// unified CLI. Each subcommand is a thin wrapper that resets flag.CommandLine,
// sets os.Args, and calls the upstream Main() function.
//
// The upstream packages live in internal/upstream/<pkgname>/ and are compiled
// through a build overlay (overlay.json) that maps them into the zoekt
// module's namespace so they can access zoekt's internal packages.

import (
	"flag"
	"os"

	"github.com/spf13/cobra"

	zoekt "github.com/sourcegraph/zoekt/unified/zoekt"
	zoekt_archive_index "github.com/sourcegraph/zoekt/unified/zoekt_archive_index"
	zoekt_dynamic_indexserver "github.com/sourcegraph/zoekt/unified/zoekt_dynamic_indexserver"
	zoekt_git_clone "github.com/sourcegraph/zoekt/unified/zoekt_git_clone"
	zoekt_git_index "github.com/sourcegraph/zoekt/unified/zoekt_git_index"
	zoekt_index "github.com/sourcegraph/zoekt/unified/zoekt_index"
	zoekt_indexserver "github.com/sourcegraph/zoekt/unified/zoekt_indexserver"
	zoekt_merge_index "github.com/sourcegraph/zoekt/unified/zoekt_merge_index"
	zoekt_mirror_bitbucket_server "github.com/sourcegraph/zoekt/unified/zoekt_mirror_bitbucket_server"
	zoekt_mirror_gerrit "github.com/sourcegraph/zoekt/unified/zoekt_mirror_gerrit"
	zoekt_mirror_gitea "github.com/sourcegraph/zoekt/unified/zoekt_mirror_gitea"
	zoekt_mirror_github "github.com/sourcegraph/zoekt/unified/zoekt_mirror_github"
	zoekt_mirror_gitiles "github.com/sourcegraph/zoekt/unified/zoekt_mirror_gitiles"
	zoekt_mirror_gitlab "github.com/sourcegraph/zoekt/unified/zoekt_mirror_gitlab"
	zoekt_repo_index "github.com/sourcegraph/zoekt/unified/zoekt_repo_index"
	zoekt_sourcegraph_indexserver "github.com/sourcegraph/zoekt/unified/zoekt_sourcegraph_indexserver"
	zoekt_test "github.com/sourcegraph/zoekt/unified/zoekt_test"
	zoekt_webserver "github.com/sourcegraph/zoekt/unified/zoekt_webserver"
)

// upstreamCmd creates a Cobra command that delegates to an upstream zoekt
// command's Main() function. It resets the global flag state and sets os.Args
// so the upstream code's flag.Parse() sees the right arguments.
func upstreamCmd(use, short string, mainFn func()) *cobra.Command {
	return &cobra.Command{
		Use:                use,
		Short:              short,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		Run: func(cmd *cobra.Command, args []string) {
			// Reset the global flag set so the upstream code's flag.Parse()
			// starts fresh.
			flag.CommandLine = flag.NewFlagSet(use, flag.ExitOnError)

			// Set os.Args so the upstream code sees: [command, args...]
			os.Args = append([]string{use}, args...)

			mainFn()
		},
	}
}

// upstreamCommands returns all upstream zoekt commands as Cobra subcommands.
func upstreamCommands() []*cobra.Command {
	return []*cobra.Command{
		upstreamCmd("zoekt-search", "Search an index (upstream zoekt CLI)", zoekt.Main),
		upstreamCmd("zoekt-index", "Index a directory of files", zoekt_index.Main),
		upstreamCmd("zoekt-git-index", "Index a git repository", zoekt_git_index.Main),
		upstreamCmd("zoekt-git-clone", "Clone a git repository (upstream)", zoekt_git_clone.Main),
		upstreamCmd("zoekt-archive-index", "Index a git archive", zoekt_archive_index.Main),
		upstreamCmd("zoekt-webserver", "Start the zoekt web server", zoekt_webserver.Main),
		upstreamCmd("zoekt-indexserver", "Periodically reindex repositories (pull-based)", zoekt_indexserver.Main),
		upstreamCmd("zoekt-dynamic-indexserver", "Dynamic push-based index server", zoekt_dynamic_indexserver.Main),
		upstreamCmd("zoekt-merge-index", "Merge or explode index shards", zoekt_merge_index.Main),
		upstreamCmd("zoekt-mirror-github", "Mirror GitHub repos", zoekt_mirror_github.Main),
		upstreamCmd("zoekt-mirror-gitlab", "Mirror GitLab repos", zoekt_mirror_gitlab.Main),
		upstreamCmd("zoekt-mirror-gitea", "Mirror Gitea repos", zoekt_mirror_gitea.Main),
		upstreamCmd("zoekt-mirror-gerrit", "Mirror Gerrit repos", zoekt_mirror_gerrit.Main),
		upstreamCmd("zoekt-mirror-gitiles", "Mirror Gitiles/cgit repos", zoekt_mirror_gitiles.Main),
		upstreamCmd("zoekt-mirror-bitbucket-server", "Mirror Bitbucket Server repos", zoekt_mirror_bitbucket_server.Main),
		upstreamCmd("zoekt-repo-index", "Index an Android repo manifest", zoekt_repo_index.Main),
		upstreamCmd("zoekt-sourcegraph-indexserver", "Sourcegraph indexserver", zoekt_sourcegraph_indexserver.Main),
		upstreamCmd("zoekt-test", "Compare zoekt results with raw search", zoekt_test.Main),
	}
}
