package docs

// DefaultSpec returns the full API specification for zoekt-server.
func DefaultSpec() Spec {
	return Spec{
		Title:   "zoekt-server API",
		Version: "1.0.0",
		Description: `Code search server combining [Zoekt](https://github.com/sourcegraph/zoekt)
with MCP (Model Context Protocol) for LLM tool use.

Includes the upstream Zoekt search/list API, file viewer, reindexing, and MCP endpoints.`,
		ServerURL:  "http://localhost:8000",
		ServerDesc: "Local development",
		Tags: []Tag{
			{Name: "Search", Description: "Code search and repository listing"},
			{Name: "Files", Description: "File content retrieval"},
			{Name: "Reindex", Description: "On-demand repository reindexing"},
			{Name: "MCP", Description: "Model Context Protocol endpoints"},
			{Name: "System", Description: "Health checks and metadata"},
		},
		Endpoints:  defaultEndpoints(),
		Components: defaultComponents(),
	}
}

func defaultEndpoints() []Endpoint {
	return []Endpoint{
		healthzEndpoint(),
		searchEndpoint(),
		listEndpoint(),
		fileEndpoint(),
		reindexPostEndpoint(),
		reindexGetEndpoint(),
		mcpEndpoint(),
	}
}

func healthzEndpoint() Endpoint {
	return Endpoint{
		Path:        "/healthz",
		Method:      "GET",
		Tag:         "System",
		Summary:     "Health check",
		OperationID: "healthCheck",
		Responses: map[string]Response{
			"200": {
				Description: "Server is healthy",
				ContentType: "application/json",
				Schema:      &Schema{Ref: `#/components/schemas/SearchResult`},
			},
			"500": {
				Description: "Server is unhealthy",
				ContentType: "text/plain",
				Schema: &Schema{
					Type:    "string",
					Example: "not ready: shard loading",
				},
			},
		},
	}
}

func searchEndpoint() Endpoint {
	return Endpoint{
		Path:        "/api/search",
		Method:      "POST",
		Tag:         "Search",
		Summary:     "Search code",
		OperationID: "search",
		Description: `Full-text code search across all indexed repositories.

Query syntax:
- Multiple terms are AND'd: ` + "`class needle`" + `
- ` + "`or`" + ` for alternatives: ` + "`thread or needle`" + `
- Quotes for phrases: ` + "`\"class Needle\"`" + `
- ` + "`-`" + ` to negate: ` + "`needle -hay`" + `
- ` + "`r:`" + ` or ` + "`repo:`" + ` to filter by repository
- ` + "`f:`" + ` or ` + "`file:`" + ` to filter by file path (regex)
- ` + "`lang:`" + ` to filter by language
- ` + "`sym:`" + ` to search symbol definitions
- ` + "`case:yes`" + ` for case-sensitive search`,
		RequestBody: &RequestBody{
			Required: true,
			Schema: Schema{
				Type:     "object",
				Required: []string{"Q"},
				Properties: map[string]Schema{
					"Q": {
						Type:    "string",
						Desc:    "Zoekt query string",
						Example: "func main lang:go",
					},
					"Opts": {
						Ref: `#/components/schemas/SearchOptions`,
					},
				},
			},
		},
		Responses: map[string]Response{
			"200": {
				Description: "Search results",
				Schema: &Schema{
					Type: "object",
					Properties: map[string]Schema{
						"Result": {Ref: `#/components/schemas/SearchResult`},
					},
				},
			},
			"400": {
				Description: "Invalid query or request",
				Schema:      &Schema{Ref: `#/components/schemas/Error`},
			},
		},
	}
}

func listEndpoint() Endpoint {
	return Endpoint{
		Path:        "/api/list",
		Method:      "POST",
		Tag:         "Search",
		Summary:     "List repositories",
		OperationID: "listRepos",
		Description: "List indexed repositories matching a query.",
		RequestBody: &RequestBody{
			Required: true,
			Schema: Schema{
				Type:     "object",
				Required: []string{"Q"},
				Properties: map[string]Schema{
					"Q": {
						Type:    "string",
						Desc:    `Query string (typically repo filters like ` + "`r:name`" + `)`,
						Example: "r:zoekt",
					},
					"Opts": {
						Type: "object",
						Properties: map[string]Schema{
							"Field": {
								Type:    "integer",
								Desc:    `"0 = full repo list, 2 = repos map"`,
								Default: 0,
							},
						},
					},
				},
			},
		},
		Responses: map[string]Response{
			"200": {
				Description: "Repository list",
				Schema: &Schema{
					Type: "object",
					Properties: map[string]Schema{
						"List": {Ref: `#/components/schemas/RepoList`},
					},
				},
			},
			"400": {
				Description: "Invalid query",
				Schema:      &Schema{Ref: `#/components/schemas/Error`},
			},
		},
	}
}

func fileEndpoint() Endpoint {
	return Endpoint{
		Path:        "/api/file",
		Method:      "GET",
		Tag:         "Files",
		Summary:     "Get file contents",
		OperationID: "getFile",
		Description: `Retrieve file contents from a bare git repository at HEAD.
Uses literal (not regex) repo and path parameters.`,
		Parameters: []Parameter{
			{
				Name:        "repo",
				In:          "query",
				Required:    true,
				Description: "Full repository name",
				Example:     "github.com/sourcegraph/zoekt",
				Schema:      Schema{Type: "string"},
			},
			{
				Name:        "path",
				In:          "query",
				Required:    true,
				Description: "File path within the repository",
				Example:     "README.md",
				Schema:      Schema{Type: "string"},
			},
		},
		Responses: map[string]Response{
			"200": {
				Description: "File contents as plain text",
				ContentType: "text/plain",
				Schema:      &Schema{Type: "string"},
			},
			"400": {
				Description: "Missing parameters",
			},
			"404": {
				Description: "File or repository not found",
			},
		},
	}
}

func reindexPostEndpoint() Endpoint {
	return Endpoint{
		Path:        "/api/reindex",
		Method:      "POST",
		Tag:         "Reindex",
		Summary:     "Trigger reindex",
		OperationID: "reindex",
		Description: `Queue a repository for immediate reindexing. The request is added to
the high-priority queue and processed ahead of periodic indexing.
The bare repo must already exist on disk (cloned by the mirror process).`,
		RequestBody: &RequestBody{
			Required: true,
			Schema: Schema{
				Type:     "object",
				Required: []string{"repo"},
				Properties: map[string]Schema{
					"repo": {
						Type:    "string",
						Desc:    `"Repository path in hostname/org/name format"`,
						Example: "github.com/myorg/myrepo",
					},
				},
			},
		},
		Responses: map[string]Response{
			"202": {
				Description: "Reindex task accepted",
				Schema: &Schema{
					Type: "object",
					Properties: map[string]Schema{
						"id": {
							Type: "string",
							Desc: "Task ID (UUID)",
						},
						"status": {
							Type: "string",
							Enum: []string{"pending"},
						},
					},
				},
			},
			"400": {
				Description: "Invalid request",
				Schema:      &Schema{Ref: `#/components/schemas/Error`},
			},
			"503": {
				Description: "Index queue full",
				Schema:      &Schema{Ref: `#/components/schemas/Error`},
			},
		},
	}
}

func reindexGetEndpoint() Endpoint {
	return Endpoint{
		Path:        "/api/reindex/{taskID}",
		Method:      "GET",
		Tag:         "Reindex",
		Summary:     "Get reindex task status",
		OperationID: "getReindexStatus",
		Parameters: []Parameter{
			{
				Name:        "taskID",
				In:          "path",
				Required:    true,
				Description: "Task ID returned by POST /api/reindex",
				Schema:      Schema{Type: "string"},
			},
		},
		Responses: map[string]Response{
			"200": {
				Description: "Task status",
				Schema:      &Schema{Ref: `#/components/schemas/ReindexTask`},
			},
			"404": {
				Description: "Task not found",
				Schema:      &Schema{Ref: `#/components/schemas/Error`},
			},
		},
	}
}

func mcpEndpoint() Endpoint {
	return Endpoint{
		Path:        "/mcp",
		Method:      "POST",
		Tag:         "MCP",
		Summary:     "MCP endpoint",
		OperationID: "mcp",
		Description: `Model Context Protocol (streamable HTTP, stateless).
Used by LLM clients for tool use. Exposes ` + "`search`" + ` and ` + "`get_file`" + ` tools.

See [MCP spec](https://modelcontextprotocol.io/) for the protocol format.`,
		Responses: map[string]Response{
			"200": {
				Description: "MCP response",
			},
		},
	}
}

func defaultComponents() []ComponentSchema {
	return []ComponentSchema{
		{Name: "Error", Schema: errorSchema()},
		{Name: "SearchOptions", Schema: searchOptionsSchema()},
		{Name: "SearchResult", Schema: searchResultSchema()},
		{Name: "Stats", Schema: statsSchema()},
		{Name: "FileMatch", Schema: fileMatchSchema()},
		{Name: "LineMatch", Schema: lineMatchSchema()},
		{Name: "LineFragment", Schema: lineFragmentSchema()},
		{Name: "SymbolInfo", Schema: symbolInfoSchema()},
		{Name: "RepoList", Schema: repoListSchema()},
		{Name: "RepoListEntry", Schema: repoListEntrySchema()},
		{Name: "Repository", Schema: repositorySchema()},
		{Name: "RepoStats", Schema: repoStatsSchema()},
		{Name: "ReindexTask", Schema: reindexTaskSchema()},
	}
}

func errorSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Schema{
			"error": {Type: "string"},
		},
	}
}

func searchOptionsSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Schema{
			"ChunkMatches": {
				Type:    "boolean",
				Desc:    "Use chunk-based matching instead of line-based",
				Default: false,
			},
			"MaxDocDisplayCount": {
				Type:    "integer",
				Desc:    "Max file matches to return",
				Default: 0,
			},
			"MaxWallTime": {
				Type:    "string",
				Desc:    "Max search duration (Go duration string)",
				Example: "10s",
			},
			"NumContextLines": {
				Type:    "integer",
				Desc:    "Context lines around matches",
				Default: 0,
			},
			"ShardMaxMatchCount": {
				Type: "integer",
				Desc: "Max matches per shard",
			},
			"TotalMaxMatchCount": {
				Type: "integer",
				Desc: "Max total matches across all shards",
			},
		},
	}
}

func searchResultSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Schema{
			"Files": {
				Type:  "array",
				Items: &Schema{Ref: `#/components/schemas/FileMatch`},
			},
			"Stats": {
				Ref: `#/components/schemas/Stats`,
			},
		},
	}
}

func statsSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Schema{
			"ContentBytesLoaded": {Type: "integer"},
			"Duration":           {Type: "string"},
			"FileCount":          {Type: "integer"},
			"FilesConsidered":    {Type: "integer"},
			"FilesLoaded":        {Type: "integer"},
			"IndexBytesLoaded":   {Type: "integer"},
			"MatchCount":         {Type: "integer"},
			"ShardsScanned":      {Type: "integer"},
		},
	}
}

func fileMatchSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Schema{
			"Branches": {
				Type:  "array",
				Items: &Schema{Type: "string"},
			},
			"FileName":   {Type: "string"},
			"Language":    {Type: "string"},
			"LineMatches": {Type: "array", Items: &Schema{Ref: `#/components/schemas/LineMatch`}},
			"Repository":  {Type: "string"},
			"Score":       {Type: "number"},
		},
	}
}

func lineMatchSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Schema{
			"Line":          {Type: "string", Desc: "Base64-encoded line content"},
			"LineFragments": {Type: "array", Items: &Schema{Ref: `#/components/schemas/LineFragment`}},
			"LineNumber":    {Type: "integer"},
		},
	}
}

func lineFragmentSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Schema{
			"LineOffset":  {Type: "integer"},
			"MatchLength": {Type: "integer"},
			"SymbolInfo":  {Ref: `#/components/schemas/SymbolInfo`},
		},
	}
}

func symbolInfoSchema() Schema {
	return Schema{
		Type:     "object",
		Nullable: true,
		Properties: map[string]Schema{
			"Kind":       {Type: "string"},
			"Parent":     {Type: "string"},
			"ParentKind": {Type: "string"},
			"Sym":        {Type: "string"},
		},
	}
}

func repoListSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Schema{
			"Repos": {
				Type:  "array",
				Items: &Schema{Ref: `#/components/schemas/RepoListEntry`},
			},
			"Stats": {Ref: `#/components/schemas/RepoStats`},
		},
	}
}

func repoListEntrySchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Schema{
			"IndexMetadata": {
				Type: "object",
				Properties: map[string]Schema{
					"IndexFormatVersion": {Type: "integer"},
					"IndexTime":          {Type: "string", Format: "date-time"},
				},
			},
			"Repository": {Ref: `#/components/schemas/Repository`},
			"Stats":      {Ref: `#/components/schemas/RepoStats`},
		},
	}
}

func repositorySchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Schema{
			"Branches": {
				Type: "array",
				Items: &Schema{
					Type: "object",
					Properties: map[string]Schema{
						"Name":    {Type: "string"},
						"Version": {Type: "string"},
					},
				},
			},
			"HasSymbols": {Type: "boolean"},
			"ID":         {Type: "integer"},
			"Name":       {Type: "string"},
			"URL":        {Type: "string"},
		},
	}
}

func repoStatsSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Schema{
			"ContentBytes": {Type: "integer"},
			"Documents":    {Type: "integer"},
			"IndexBytes":   {Type: "integer"},
			"Repos":        {Type: "integer"},
			"Shards":       {Type: "integer"},
		},
	}
}

func reindexTaskSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Schema{
			"error": {
				Type:     "string",
				Nullable: true,
			},
			"id":   {Type: "string"},
			"repo": {Type: "string"},
			"status": {
				Type: "string",
				Enum: []string{"pending", "running", "completed", "failed"},
			},
		},
	}
}
