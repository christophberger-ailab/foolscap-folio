package site

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// longPage builds the project with a page whose matches are far below the
// top, and returns the output directory.
func longPage(t *testing.T) string {
	t.Helper()
	root, out := project(t)
	filler := strings.Repeat("Filler text of no interest.\n\n", 80)
	write(t, root, "x/long.qmd", "---\ntitle: Long\n---\n\n## Start\n\n"+filler+
		"## Animals\n\nA zebra here. ZEBRA again, and a zébra.\n\nLast *zebra*.\n\n<script>var zebra;</script>\n")
	if err := Build(opts(root, out)); err != nil {
		t.Fatal(err)
	}
	return out
}

// marks reports the page's marks, the current one, and whether the first or
// current mark is in view, as the browse scripts log them.
const marks = `
  const marks = () => page.evaluate(() => {
    const m = [...document.querySelectorAll('article mark.page-mark')];
    const c = m.findIndex(x => x.classList.contains('current'));
    const r = (m[c] || m[0] || document.body).getBoundingClientRect();
    return m.length + ' ' + m.map(x => x.textContent).join(',') + ' current:' + c +
      ' inview:' + (m.length > 0 && scrollY > 0 && r.top >= 0 && r.bottom <= innerHeight);
  });`

// SITE.6-10: a page opened with the words a search hit matched marks them
// all and scrolls to the first.
func TestPageMarksMatchedWords(t *testing.T) {
	out := longPage(t)
	got := browse(t, "file://"+filepath.Join(out, "x", "long.html")+"?mark=zebra", marks+`
  console.log(await marks());`)
	if want := "4 zebra,ZEBRA,zébra,zebra current:0 inview:true\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// SITE.6-10: a hit opens its page with the words it matched marked.
func TestSearchHitOpensMarked(t *testing.T) {
	out := longPage(t)
	srv := httptest.NewServer(http.FileServer(http.Dir(out)))
	defer srv.Close()
	got := browse(t, srv.URL+"/x/page.html", marks+`
  await page.fill('.search input', 'zebra');
  const hit = page.locator('.search-hit').first();
  await hit.waitFor();
  console.log(new URL(await hit.getAttribute('href'), page.url()).search);
  await hit.click();
  await page.waitForURL(/long\.html/);
  console.log(await marks());`)
	if want := "?mark=zebra\n4 zebra,ZEBRA,zébra,zebra current:0 inview:true\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// SITE.6-11: the field above the contents list finds its text in the page
// alone; Enter and Shift+Enter step through the matches, Escape clears it.
func TestFindInPage(t *testing.T) {
	out := longPage(t)
	page := read(t, out, "x/long.html")
	has(t, "long", page, `<div class="find" role="search">`, `aria-label="Diese Seite durchsuchen"`)
	if i, j := strings.Index(page, `class="find"`), strings.Index(page, `class="toc-title"`); i < 0 || i > j {
		t.Error("the find field is not above the contents list")
	}
	got := browse(t, "file://"+filepath.Join(out, "x", "long.html")+"?mark=filler", marks+`
  const count = () => page.textContent('.find-count');
  await page.fill('.find input', 'Zebra');
  console.log(await marks(), await count());
  await page.press('.find input', 'Enter');
  console.log(await marks(), await count());
  await page.press('.find input', 'Shift+Enter');
  await page.press('.find input', 'Shift+Enter');
  console.log(await marks(), await count());
  await page.fill('.find input', 'z');
  console.log(await marks(), await count());
  await page.fill('.find input', 'nothing like it');
  console.log(await marks(), await count());
  await page.fill('.find input', 'zebra');
  await page.press('.find input', 'Escape');
  console.log(await marks(), await count(), JSON.stringify(await page.inputValue('.find input')));`)
	want := "4 zebra,ZEBRA,zébra,zebra current:0 inview:true 1/4\n" +
		"4 zebra,ZEBRA,zébra,zebra current:1 inview:true 2/4\n" +
		"4 zebra,ZEBRA,zébra,zebra current:3 inview:true 4/4\n" +
		"0  current:-1 inview:false \n" +
		"0  current:-1 inview:false 0/0\n" +
		"0  current:-1 inview:false  \"\"\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	hasNot(t, "x/page", read(t, out, "x/page.html"), `class="find"`)
}
