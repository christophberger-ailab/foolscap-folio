package site

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// browse runs script in node with Playwright's Chromium against the page at
// url, printing what the script logs; it skips without node or Playwright.
func browse(t *testing.T, url, script string) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	if exec.Command(node, "-e", "require('playwright')").Run() != nil {
		t.Skip("playwright not installed")
	}
	js := `const { chromium } = require('playwright');
(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  await page.goto(process.argv[1]);
  try {` + script + `} finally { await browser.close(); }
})().catch(e => { console.error(e); process.exit(1); });`
	out, err := exec.Command(node, "-e", js, "--", url).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	return string(out)
}

// SITE.9: a click on an image of the page shows it at full size over the
// page; a click or Escape closes it; an image inside a link is followed.
func TestImageFullSize(t *testing.T) {
	root, out := project(t)
	write(t, root, "x/page.qmd", "---\ntitle: X Page\n---\n\n![Pic](/assets/pic.png)\n\n[![Linked](/assets/pic.png)](/x/index.qmd)\n")
	if err := Build(opts(root, out)); err != nil {
		t.Fatal(err)
	}
	got := browse(t, "file://"+filepath.Join(out, "x", "page.html"), `
  const shown = () => page.evaluate(() => {
    const d = document.querySelector('dialog.lightbox');
    return d && d.open ? d.querySelector('img').getAttribute('src') : '';
  });
  console.log('before:' + await shown());
  await page.click('article img[alt="Pic"]');
  console.log('click:' + await shown());
  await page.keyboard.press('Escape');
  console.log('escape:' + await shown());
  await page.click('article img[alt="Pic"]');
  await page.mouse.click(5, 5);
  console.log('closed:' + await shown());
  await page.click('article img[alt="Linked"]');
  await page.waitForURL(/index\.html$/);
  console.log('link:' + page.url().split('/').pop());`)
	want := "before:\nclick:../assets/pic.png\nescape:\nclosed:\nlink:index.html\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	css, err := os.ReadFile(filepath.Join(out, "_site", "style.css"))
	if err != nil || !strings.Contains(string(css), ".lightbox") {
		t.Error("style.css does not style the lightbox")
	}
}

// RENDER.DRAFT.7: in a draft site, a bookmarked page shows 🔖 in the top
// right corner of the page, not in the text below its title.
func TestDraftBookmarkInTheCorner(t *testing.T) {
	root, out := project(t)
	write(t, root, "x/page.qmd", "---\ntitle: X Page\nbookmarked: true\n---\n\n# X Page\n\nSome text.\n")
	o := opts(root, out)
	o.Draft = true
	if err := Build(o); err != nil {
		t.Fatal(err)
	}
	got := browse(t, "file://"+filepath.Join(out, "x", "page.html"), `
  const r = await page.evaluate(() => {
    const b = document.querySelector('.folio-bookmark').getBoundingClientRect();
    const p = document.querySelector('article.page').getBoundingClientRect();
    const h = document.querySelector('article.page h1').getBoundingClientRect();
    return {right: Math.abs(p.right - b.right) < 2, top: Math.abs(p.top - b.top) < 2, aside: b.left >= h.left + 50};
  });
  console.log(JSON.stringify(r));`)
	if want := `{"right":true,"top":true,"aside":true}` + "\n"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
