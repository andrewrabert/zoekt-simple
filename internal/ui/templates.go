// Forked from github.com/sourcegraph/zoekt/web/templates.go
// Changes: package name, vendored assets, dark mode, CSS classes for themed colors.

package ui

import (
	"html/template"
	"log"

	"github.com/sourcegraph/zoekt/web"
)

var Top = template.New("top").Funcs(web.Funcmap)

var TemplateText = map[string]string{
	"head": `
<head>
<meta charset="utf-8">
<meta http-equiv="X-UA-Compatible" content="IE=edge">
<meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="stylesheet" href="/static/bootstrap.min.css">
<style>
  #navsearchbox { width: 350px !important; }
  #maxhits { width: 100px !important; }
  #context { width: 70px !important; }
  .label-dup { border: 1px solid #aaa !important; color: black; }
  .noselect { color: #999; user-select: none; }
  a.label-dup:hover { color: black; background: #ddd; }
  .result { display: block; content: " "; visibility: hidden; }
  .container-results { overflow: auto; max-height: calc(100% - 72px); }
  .inline-pre { border: unset; background-color: unset; margin: unset; padding: unset; overflow: unset; }
  :target { background-color: #ccf; }
  table tbody tr td { border: none !important; padding: 2px !important; }
  .match-bg { background-color: rgba(238, 238, 255, 0.6); }
  .file-bg { overflow: auto; background: #eef; }

  @media (prefers-color-scheme: dark) {
    body { background: #1b1b1b; color: #ccc; }
    a { color: #7ab0df; }
    a:hover, a:focus { color: #a3c9ea; }
    .navbar-default { background: #252525; border-color: #333; }
    .navbar-default .navbar-brand, .navbar-default .navbar-text { color: #ccc; }
    .navbar-default .navbar-brand:hover { color: #fff; }
    .form-control { background: #2d2d2d; color: #ccc; border-color: #444; }
    .form-control:focus { border-color: #666; box-shadow: 0 0 4px rgba(100,100,100,0.4); }
    input[type="number"] { color-scheme: dark; }
    label { color: #ccc; }
    pre, code { background: #252525; color: #ccc; border-color: #333; }
    .btn-primary { background: #3a3a3a; border-color: #555; color: #ccc; }
    .btn-primary:hover { background: #444; border-color: #666; color: #fff; }
    .label-default { background: #555; }
    .label-primary { background: #2a4a6b; color: #ccc; border: none; }
    .label-dup { color: #ccc !important; border-color: #555 !important; }
    .jumbotron { background: #252525; color: #ccc; }
    .input-group-addon { background: #333; color: #ccc; border-color: #444; }
    .input-group-btn .btn { background: #333; color: #ccc; border-color: #444; }
    .input-group-btn .btn:hover { background: #3a3a3a; color: #fff; }
    select, textarea { background: #2d2d2d; color: #ccc; border-color: #444; }
    .noselect { color: #666; }
    u { text-decoration: none; }
    .inline-pre { color: #ccc; }
    .inline-pre b { color: #e8c96a; }
    h1, h2, h3, h4, h5, h6, p, small, dt, dd, th, td, tr, b, strong { color: #ccc; }
    :target { background: #333; }
    .match-bg { background: #252525; }
    .file-bg { background: #222; }
    .table-hover > tbody > tr:hover { background: #2d2d2d; }
    table, tr, td, th, thead, tbody,
    .table > thead > tr > th,
    .table > tbody > tr > td,
    .table > tbody > tr > th,
    table tbody tr td { border-color: #333 !important; }
    .dl-horizontal dt a { color: #7ab0df; }
    hr { border-color: #333; }
  }
</style>
</head>
  `,

	"jsdep": `
<script src="/static/jquery.min.js"></script>
<script src="/static/bootstrap.min.js"></script>
`,

	"searchbox": `
<form action="search">
  <div class="form-group form-group-lg">
    <div class="input-group input-group-lg">
      <input class="form-control" placeholder="Search for some code..." autofocus
              {{if .Query}}
              value={{.Query}}
              {{end}}
              id="searchbox" type="text" name="q">
      <div class="input-group-btn">
        <button class="btn btn-primary">Search</button>
      </div>
    </div>
  </div>
</form>
`,

	"navbar": `
<nav class="navbar navbar-default">
  <div class="container-fluid">
    <div class="navbar-header">
      <a class="navbar-brand" href="/">Zoekt</a>
      <button type="button" class="navbar-toggle collapsed" data-toggle="collapse" data-target="#navbar-collapse" aria-expanded="false">
        <span class="sr-only">Toggle navigation</span>
        <span class="icon-bar"></span>
        <span class="icon-bar"></span>
        <span class="icon-bar"></span>
      </button>
    </div>
    <div class="navbar-collapse collapse" id="navbar-collapse" aria-expanded="false" style="height: 1px;">
      <form class="navbar-form navbar-left" action="search">
        <div class="form-group">
          <input class="form-control"
                placeholder="Search for some code..." role="search"
                id="navsearchbox" type="text" name="q" autofocus
                {{if .Query}}
                value={{.Query}}
                {{end}}>
          <div class="input-group">
            <div class="input-group-addon">Max Results</div>
            <input class="form-control" type="number" id="maxhits" name="num" value="{{.Num}}">
          </div>
          <div class="input-group">
            <div class="input-group-addon">Context Lines</div>
            <input class="form-control" id="context" name="ctx" type="number" value="{{.Ctx}}">
          </div>
          <button class="btn btn-primary">Search</button>
          {{if .Debug}}<input id="debug" name="debug" type="hidden" value="{{.Debug}}">{{end}}
        </div>
      </form>
    </div>
  </div>
</nav>
<script>
document.onkeydown=function(e){
  var e = e || window.event;
  if (e.key == "/") {
    var navbox = document.getElementById("navsearchbox");
    if (document.activeElement !== navbox) {
      navbox.focus();
      return false;
    }
  }
};
</script>
`,

	"search": `
<html>
{{template "head"}}
<title>Zoekt, en gij zult spinazie eten</title>
<body>
  <div class="jumbotron">
    <div class="container">
      {{template "searchbox" .Last}}
    </div>
  </div>

  <div class="container">
    <div class="row">
      <div class="col-md-8">
        <h3>Search examples:</h3>
        <dl class="dl-horizontal">
          <dt><a href="search?q=needle">needle</a></dt><dd>search for "needle"</dd>
          <dt><a href="search?q=thread+or+needle">thread or needle</a></dt><dd>search for either "thread" or "needle"</dd>
          <dt><a href="search?q=class+needle">class needle</a></dt><dd>search for files containing both "class" and "needle"</dd>
          <dt><a href="search?q=class+Needle">class Needle</a></dt><dd>search for files containing both "class" (case insensitive) and "Needle" (case sensitive)</dd>
          <dt><a href="search?q=class+Needle+case:yes">class Needle case:yes</a></dt><dd>search for files containing "class" and "Needle", both case sensitively</dd>
          <dt><a href="search?q=%22class Needle%22">"class Needle"</a></dt><dd>search for files with the phrase "class Needle"</dd>
          <dt><a href="search?q=needle+-hay">needle -hay</a></dt><dd>search for files with the word "needle" but not the word "hay"</dd>
          <dt><a href="search?q=path+file:java">path file:java</a></dt><dd>search for the word "path" in files whose name contains "java"</dd>
          <dt><a href="search?q=needle+lang%3Apython&num=50">needle lang:python</a></dt><dd>search for "needle" in Python source code</dd>
          <dt><a href="search?q=f:%5C.c%24">f:\.c$</a></dt><dd>search for files whose name ends with ".c"</dd>
          <dt><a href="search?q=path+-file:java">path -file:java</a></dt><dd>search for the word "path" excluding files whose name contains "java"</dd>
          <dt><a href="search?q=foo.*bar">foo.*bar</a></dt><dd>search for the regular expression "foo.*bar"</dd>
          <dt><a href="search?q=-%28Path File%29 Stream">-(Path File) Stream</a></dt><dd>search "Stream", but exclude files containing both "Path" and "File"</dd>
          <dt><a href="search?q=-Path%5c+file+Stream">-Path\ file Stream</a></dt><dd>search "Stream", but exclude files containing "Path File"</dd>
          <dt><a href="search?q=sym:data">sym:data</a></dt><dd>search for symbol definitions containing "data"</dd>
          <dt><a href="search?q=phone+r:droid">phone r:droid</a></dt><dd>search for "phone" in repositories whose name contains "droid"</dd>
          <dt><a href="search?q=phone+archived:no">phone archived:no</a></dt><dd>search for "phone" in repositories that are not archived</dd>
          <dt><a href="search?q=phone+fork:no">phone fork:no</a></dt><dd>search for "phone" in repositories that are not forks</dd>
          <dt><a href="search?q=phone+public:no">phone public:no</a></dt><dd>search for "phone" in repositories that are not public</dd>
          <dt><a href="search?q=phone+b:master">phone b:master</a></dt><dd>for Git repos, find "phone" in files in branches whose name contains "master".</dd>
          <dt><a href="search?q=phone+b:HEAD">phone b:HEAD</a></dt><dd>for Git repos, find "phone" in the default ('HEAD') branch.</dd>
        </dl>
      </div>
      <div class="col-md-4">
        <h3>To list repositories, try:</h3>
        <dl class="dl-horizontal">
          <dt><a href="search?q=r:droid">r:droid</a></dt><dd>list repositories whose name contains "droid".</dd>
          <dt><a href="search?q=r:go+-r:google">r:go -r:google</a></dt><dd>list repositories whose name contains "go" but not "google".</dd>
        </dl>
      </div>
    </div>
  </div>
  <nav class="navbar navbar-default navbar-bottom">
    <div class="container">
      {{template "footerBoilerplate"}}
      <p class="navbar-text navbar-right">
        Used {{HumanUnit .Stats.IndexBytes}} mem for
        {{.Stats.Documents}} documents ({{HumanUnit .Stats.ContentBytes}})
        from {{.Stats.Repos}} repositories.
      </p>
    </div>
  </nav>
</body>
</html>
`,

	"footerBoilerplate": `<a class="navbar-text" href="about">About</a>
<a class="navbar-text" href="/docs/">API Docs</a>`,

	"results": `
<html>
{{template "head"}}
<title>Results for {{.QueryStr}}</title>
<script>
  function zoektAddQ(atom) {
      window.location.href = "/search?q=" + encodeURIComponent("{{.QueryStr}}" + " " + atom) +
	  "&" + "num=" + {{.Last.Num}};
  }
</script>
<body id="results">
  {{template "navbar" .Last}}
  <div class="container-fluid container-results">
    <h5>
      {{if .Stats.Crashes}}<br><b>{{.Stats.Crashes}} shards crashed</b><br>{{end}}
      {{ $fileCount := len .FileMatches }}
      Found {{.Stats.MatchCount}} results in {{.Stats.FileCount}} files{{if or (lt $fileCount .Stats.FileCount) (or (gt .Stats.ShardsSkipped 0) (gt .Stats.FilesSkipped 0)) }},
        showing top {{ $fileCount }} files (<a rel="nofollow"
           href="search?q={{.Last.Query}}&num={{More .Last.Num}}">show more</a>).
      {{else}}.{{end}}
    </h5>
    {{range .FileMatches}}
    <table class="table table-hover table-condensed">
      <thead>
        <tr>
          <th colspan="2">
            {{if .URL}}<a name="{{.ResultID}}" class="result"></a><a href="{{.URL}}" >{{else}}<a name="{{.ResultID}}">{{end}}
            <small>
              {{.Repo}}:{{.FileName}} {{if .ScoreDebug}}<i>({{.ScoreDebug}})</i>{{end}}</a>:
              <span style="font-weight: normal">[ {{if .Branches}}{{range .Branches}}<span class="label label-default">{{.}}</span>,{{end}}{{end}} ]</span>
              {{if .Language}}<button
                   title="restrict search to files written in {{.Language}}"
                   onclick="zoektAddQ('lang:&quot;{{.Language}}&quot;')" class="label label-primary">language {{.Language}}</button></span>{{end}}
              {{if .DuplicateID}}<a class="label label-dup" href="#{{.DuplicateID}}">Duplicate result</a>{{end}}
            </small>
          </th>
        </tr>
      </thead>
      {{if not .DuplicateID}}
      <tbody>
        {{range .Matches}}
        {{if gt .LineNum 0}}
        <tr>
          <td class="match-bg" style="width: 1%; white-space: nowrap;">
<pre class="inline-pre"><p style="margin: 0px;">{{$beforeLines := AddLineNumbers .Before .LineNum true}}{{range $line := $beforeLines}}<span class="noselect"><u>{{$line.LineNum}}</u>:</span>
{{end}}<span class="noselect">{{if .URL}}<a href="{{.URL}}">{{end}}<u>{{.LineNum}}</u>{{if .URL}}</a>{{end}}:</span>
{{$afterLines := AddLineNumbers .After .LineNum false}}{{range $line := $afterLines}}<span class="noselect"><u>{{$line.LineNum}}</u>:</span>
{{end}}</p></pre>
          </td>
          <td class="match-bg">
<pre class="inline-pre"><p style="margin: 0px;">{{range $line := $beforeLines}} {{$line.Content}}
{{end}}</p> {{range .Fragments}}{{LimitPre 100 .Pre}}<b>{{.Match}}</b>{{LimitPost 100 (TrimTrailingNewline .Post)}}{{end}}<p style="margin: 0px;">{{range $line := $afterLines}} {{$line.Content}}
{{end}}</p>{{if .ScoreDebug}}<i>({{.ScoreDebug}})</i>{{end}}</pre>
          </td>
        </tr>
        {{end}}
      </tbody>
      {{end}}
      {{end}}
    </table>
    {{end}}

  <nav class="navbar navbar-default navbar-bottom">
    <div class="container">
      {{template "footerBoilerplate"}}
      <p class="navbar-text navbar-right">
      Took {{.Stats.Duration}}{{if .Stats.Wait}} (queued: {{.Stats.Wait}}){{end}} for
      {{HumanUnit .Stats.IndexBytesLoaded}}B index data,
      {{.Stats.NgramMatches}} ngram matches,
      {{.Stats.FilesConsidered}} docs considered,
      {{.Stats.FilesLoaded}} docs ({{HumanUnit .Stats.ContentBytesLoaded}}B) loaded,
      {{.Stats.ShardsScanned}} shards scanned,
      {{.Stats.ShardsSkippedFilter}} shards filtered
      {{- if or .Stats.FilesSkipped .Stats.ShardsSkipped -}}
        , {{.Stats.FilesSkipped}} docs skipped, {{.Stats.ShardsSkipped}} shards skipped
      {{- end -}}
	  .
      </p>
    </div>
  </nav>
  </div>
  {{ template "jsdep"}}
</body>
</html>
`,

	"repolist": `
<html>
{{template "head"}}
<body id="results">
  <div class="container">
    {{template "navbar" .Last}}
    <div><b>
    Found {{.Stats.Repos}} repositories ({{.Stats.Documents}} files, {{HumanUnit .Stats.ContentBytes}}B content)
    </b></div>
    <table class="table table-hover table-condensed">
      <thead>
	<tr>
	  {{- define "q"}}q={{.Last.Query}}{{if (gt .Last.Num 0)}}&num={{.Last.Num}}{{end}}{{end}}
	  <th>Name <a href="/search?{{template "q" .}}&order=name">▼</a><a href="/search?{{template "q" .}}&order=revname">▲</a></th>
	  <th>Last updated <a href="/search?{{template "q" .}}&order=revtime">▼</a><a href="/search?{{template "q" .}}&order=time">▲</a></th>
	  <th>Branches</th>
	  <th>Size <a href="/search?{{template "q" .}}&order=revsize">▼</a><a href="/search?{{template "q" .}}&order=size">▲</a></th>
	  <th>RAM <a href="/search?{{template "q" .}}&order=revram">▼</a><a href="/search?{{template "q" .}}&order=ram">▲</a></th>
	</tr>
      </thead>
      <tbody>
	{{range .Repos -}}
	<tr>
	  <td>{{if .URL}}<a href="{{.URL}}">{{end}}{{.Name}}{{if .URL}}</a>{{end}}</td>
	  <td><small>{{.IndexTime.Format "Jan 02, 2006 15:04"}}</small></td>
	  <td style="vertical-align: middle;">
	    {{- range .Branches -}}
	    {{if .URL}}<tt><a class="label label-default small" href="{{.URL}}">{{end}}{{.Name}}{{if .URL}}</a> </tt>{{end}}&nbsp;
	    {{- end -}}
	  </td>
	  <td><small>{{HumanUnit .Files}} files ({{HumanUnit .Size}}B)</small></td>
	  <td><small>{{HumanUnit .MemorySize}}B</td>
	</tr>
	{{end}}
      </tbody>
    </table>
  </div>

  <nav class="navbar navbar-default navbar-bottom">
    <div class="container">
      {{template "footerBoilerplate"}}
      <p class="navbar-text navbar-right">
      </p>
    </div>
  </nav>

  {{ template "jsdep"}}
</body>
</html>
`,

	"print": `
<html>
  {{template "head"}}
  <title>{{.Repo}}:{{.Name}}</title>
<body id="results">
  {{template "navbar" .Last}}
  <div class="container-fluid container-results" >
     <div><b>{{.Name}}</b></div>
     <div class="table table-hover table-condensed file-bg">
       {{ range $index, $ln := .Lines}}
	 <pre id="l{{Inc $index}}" class="inline-pre"><span class="noselect"><a href="#l{{Inc $index}}">{{Inc $index}}</a>: </span>{{$ln}}</pre>
       {{end}}
     </div>
  <nav class="navbar navbar-default navbar-bottom">
    <div class="container">
      {{template "footerBoilerplate"}}
      <p class="navbar-text navbar-right">
      </p>
    </div>
  </nav>
  </div>
 {{ template "jsdep"}}
</body>
</html>
`,

	"about": `
<html>
  {{template "head"}}
  <title>About <em>zoekt</em></title>
<body>
  <div class="jumbotron">
    <div class="container">
      {{template "searchbox" .Last}}
    </div>
  </div>

  <div class="container">
    <p>
      This is <a href="http://github.com/sourcegraph/zoekt"><em>zoekt</em> (IPA: /zukt/)</a>,
      an open-source full text search engine. It's pronounced roughly as you would
      pronounce "zooked" in English.
    </p>
    <p>
    {{if .Version}}<em>Zoekt</em> version {{.Version}}, uptime{{else}}Uptime{{end}} {{.Uptime}}
    </p>

    <p>
    Used {{HumanUnit .Stats.IndexBytes}} memory for
    {{.Stats.Documents}} documents ({{HumanUnit .Stats.ContentBytes}})
    from {{.Stats.Repos}} repositories.
    </p>
  </div>

  <nav class="navbar navbar-default navbar-bottom">
    <div class="container">
      {{template "footerBoilerplate"}}
      <p class="navbar-text navbar-right">
      </p>
    </div>
  </nav>
`,

	"robots": `
user-agent: *
disallow: /search
`,
}

func init() {
	for k, v := range TemplateText {
		_, err := Top.New(k).Parse(v)
		if err != nil {
			log.Panicf("parse(%s): %v:", k, err)
		}
	}
}
