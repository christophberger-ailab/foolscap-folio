package web

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// The rename field treats a section page the same whichever extension its
// index page carries: name/index.md, the extension new pages get, as well
// as the older name/index.qmd. The field stands for the directory, and the
// index file stays text after it. A page of its own gets its file name in
// the field.
func TestRenamePartsTreatMdAndQmdAlike(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	src, err := os.ReadFile("assets/static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	fn := regexp.MustCompile(`(?s)\nfunction renameParts\(.*?\n}\n`).Find(src)
	if fn == nil {
		t.Fatal("app.js has no function renameParts")
	}
	script := string(fn) + `
for (const p of ["guide/strings/index.md", "guide/strings/index.qmd", "guide/strings/4-builder.md", "guide/strings/4-builder.qmd"]) {
  const r = renameParts(p);
  console.log([p, r.head, r.name, r.tail].join("|"));
}`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	want := strings.Join([]string{
		"guide/strings/index.md|guide/|strings|/index.md",
		"guide/strings/index.qmd|guide/|strings|/index.qmd",
		"guide/strings/4-builder.md|guide/strings/|4-builder.md|",
		"guide/strings/4-builder.qmd|guide/strings/|4-builder.qmd|",
	}, "\n") + "\n"
	if string(out) != want {
		t.Errorf("renameParts:\n%s\nwant:\n%s", out, want)
	}
}
