package web

import (
	"strings"
	"testing"
)

// UI.3-4-7: the preview renders a definition list as the site does: the terms in a
// <dt> each, every definition in a <dd> holding all of its paragraphs and
// the fenced divs indented with them; what is not indented ends the list.
func TestPreviewDefinitionList(t *testing.T) {
	script := `
const fs = require("fs");
const vm = require("vm");
const ctx = vm.createContext({console});
vm.runInContext(fs.readFileSync("assets/static/marked.umd.js", "utf8"), ctx);
vm.runInContext(fs.readFileSync("assets/static/preview.js", "utf8"), ctx);
ctx.src = process.argv[1];
console.log(vm.runInContext("registerDefinitionListExtension(); marked.parse(convertDivs(src))", ctx));
`
	css, err := assets.ReadFile("assets/static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".markdown-preview dt { font-weight: bold; }", ".markdown-preview dd { margin: 0 0 0.5em 1.6em; }"} {
		if !assetContains("app.css", string(css), want) {
			t.Errorf("app.css does not carry %q", want)
		}
	}
	tests := []struct{ name, src, want string }{
		{
			name: "paragraphs and the text after the list",
			src:  "id\n: A unique identifier\n\n  The id is auto-generated.\n\nThese values are used.\n",
			want: "<dl><dt>id</dt><dd><p>A unique identifier</p><p>The id is auto-generated.</p></dd></dl><p>These values are used.</p>",
		},
		{
			name: "a callout in the definition",
			src:  "id\n: An identifier\n\n  ::: {.callout-tip}\n  Use identifiers.\n  :::\n\nAfter.\n",
			want: `<dl><dt>id</dt><dd><p>An identifier</p><div class="folio callout-tip"><p>Use identifiers.</p></div></dd></dl><p>After.</p>`,
		},
		{
			name: "terms, definitions and entries",
			src:  "a\n*b*\n: c\nlazy\n: d\n\ne\n\n: f\n\nout\n",
			want: "<dl><dt>a</dt><dt><em>b</em></dt><dd><p>c\nlazy</p></dd><dd><p>d</p></dd><dt>e</dt><dd><p>f</p></dd></dl><p>out</p>",
		},
		{
			name: "no definition: a plain paragraph",
			src:  "a\nb\n\n:c\n",
			want: "<p>a\nb</p><p>:c</p>",
		},
		{
			name: "a code block is no term",
			src:  "```\nx\n: y\n```\n",
			want: "<pre><code>x\n: y\n</code></pre>",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runNode(t, script, tc.src)
			got = strings.ReplaceAll(strings.TrimSpace(got), ">\n<", "><")
			if got != tc.want {
				t.Errorf("got\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}
