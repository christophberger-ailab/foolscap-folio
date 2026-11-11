package preview

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
)

const folioYML = `title: Test
formats:
  web: {type: html}
  handout: {type: docx}
audiences:
  std:
  pol:
topics:
  guide:
`

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "folio.yml", folioYML)
	write(t, root, "guide/index.qmd", "---\ntitle: Guide\n---\n# Guide\n\nHello preview.\n")
	write(t, root, "guide/second.qmd", "---\ntitle: Second\norder: 1\n---\n# Second\n")
	return root
}

func newServer(t *testing.T, root string, req config.Request) *Server {
	t.Helper()
	s := New(root, req, func(string, ...any) {})
	t.Cleanup(func() { s.Close() })
	return s
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

// preview.RENDER.1, LIVE.1, LIVE.4
func TestServesRenderedSiteWithLiveScript(t *testing.T) {
	s := newServer(t, fixture(t), config.Request{})
	if err := s.Build(); err != nil {
		t.Fatal(err)
	}
	rec := get(t, s, "/guide/index.html")
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Hello preview.") {
		t.Fatalf("code %d body %q", rec.Code, body)
	}
	if !strings.Contains(body, livePath) || strings.Index(body, livePath) > strings.LastIndex(body, "</body>") {
		t.Error("live script missing or not inside <body>")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control %q", rec.Header().Get("Cache-Control"))
	}
	css := get(t, s, "/_site/style.css")
	if css.Code != 200 || strings.Contains(css.Body.String(), livePath) {
		t.Errorf("stylesheet: code %d, or script injected", css.Code)
	}
	if home := get(t, s, "/"); home.Code != 200 || !strings.Contains(home.Body.String(), livePath) {
		t.Errorf("home: code %d", home.Code)
	}
}

// preview.RENDER.2, RENDER.3
func TestRebuildReplacesSiteOutsideProject(t *testing.T) {
	root := fixture(t)
	s := newServer(t, root, config.Request{})
	if err := s.Build(); err != nil {
		t.Fatal(err)
	}
	first := s.dir
	if strings.HasPrefix(first, root) {
		t.Errorf("site rendered into the project: %s", first)
	}
	if _, err := os.Stat(filepath.Join(root, "_output")); !os.IsNotExist(err) {
		t.Error("project output directory written")
	}
	write(t, root, "guide/index.qmd", "---\ntitle: Guide\n---\n# Guide\n\nChanged text.\n")
	if err := s.Build(); err != nil {
		t.Fatal(err)
	}
	if s.gen != 2 {
		t.Errorf("gen %d, want 2", s.gen)
	}
	if !strings.Contains(get(t, s, "/guide/index.html").Body.String(), "Changed text.") {
		t.Error("change not served")
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Error("previous render not removed")
	}
	s.Close()
	if _, err := os.Stat(s.dir); !os.IsNotExist(err) {
		t.Error("Close left the render behind")
	}
}

// preview.RENDER.4, RENDER.5
func TestFailedBuildKeepsLastGoodSite(t *testing.T) {
	root := fixture(t)
	s := newServer(t, root, config.Request{})
	if err := s.Build(); err != nil {
		t.Fatal(err)
	}
	write(t, root, "folio.yml", folioYML+"bogus: 1\n")
	if err := s.Build(); err == nil {
		t.Fatal("broken folio.yml accepted")
	}
	if s.err == nil {
		t.Error("error not recorded")
	}
	if rec := get(t, s, "/guide/index.html"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Hello preview.") {
		t.Errorf("last good site not served: %d", rec.Code)
	}
	write(t, root, "folio.yml", folioYML)
	if err := s.Build(); err != nil || s.err != nil {
		t.Errorf("good render did not clear the error: %v %v", err, s.err)
	}
}

// Before the first good render there is nothing to serve; the page says why
// and carries the live script, so it reloads once the project is fixed.
func TestNoSiteYetServesErrorWithLiveScript(t *testing.T) {
	root := fixture(t)
	write(t, root, "folio.yml", "bogus: 1\n")
	s := newServer(t, root, config.Request{})
	if err := s.Build(); err == nil {
		t.Fatal("broken folio.yml accepted")
	}
	rec := get(t, s, "/")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), livePath) ||
		!strings.Contains(rec.Body.String(), "bogus") {
		t.Errorf("code %d body %q", rec.Code, rec.Body.String())
	}
}

// preview.COMMAND.3-4, COMMAND.3-5
func TestSelectSite(t *testing.T) {
	root := fixture(t)
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	sel, others, err := selectSite(cfg, config.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if sel.Format.Name != "web" || sel.Audience.Name != "std" || len(others) != 1 || others[0] != "--format web --audience pol" {
		t.Errorf("sel %s others %q", sel, others)
	}
	sel, others, err = selectSite(cfg, config.Request{Audiences: []string{"pol"}})
	if err != nil || sel.Audience.Name != "pol" || len(others) != 0 {
		t.Errorf("sel %s others %q err %v", sel, others, err)
	}
	if _, _, err := selectSite(cfg, config.Request{Formats: []string{"handout"}}); err == nil ||
		!strings.Contains(err.Error(), "website") {
		t.Errorf("document format accepted: %v", err)
	}
}

// preview.WATCH.2, WATCH.3
func TestSnapshotSeesChangesButNotHiddenOrOutput(t *testing.T) {
	root := fixture(t)
	skip := map[string]bool{filepath.Join(root, "_output"): true}
	before := snapshot(root, skip)
	if snapshot(root, skip) != before {
		t.Fatal("snapshot not stable")
	}
	write(t, root, ".git/HEAD", "x")
	write(t, root, "_output/web/index.html", "x")
	write(t, root, "_build/guide.md", "x")
	write(t, root, "guide/.index.qmd.swp", "x")
	if snapshot(root, skip) != before {
		t.Error("hidden or output files changed the snapshot")
	}
	write(t, root, "guide/third.qmd", "# Third\n")
	added := snapshot(root, skip)
	if added == before {
		t.Error("new page not seen")
	}
	write(t, root, "guide/third.qmd", "# Third, longer\n")
	if snapshot(root, skip) == added {
		t.Error("changed page not seen")
	}
}

// preview.WATCH.1, LIVE.2
func TestWatchRebuildsAndNotifiesBrowser(t *testing.T) {
	root := fixture(t)
	s := newServer(t, root, config.Request{})
	if err := s.Build(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s)
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Watch(ctx, 10*time.Millisecond)

	resp, err := http.Get(srv.URL + livePath)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type %q", ct)
	}
	events := make(chan string, 8)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if d, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				events <- d
			}
		}
		close(events)
	}()
	next := func() string {
		select {
		case e := <-events:
			return e
		case <-time.After(5 * time.Second):
			t.Fatal("no event")
			return ""
		}
	}
	if e := next(); e != "1" {
		t.Fatalf("first event %q, want the current render 1", e)
	}
	time.Sleep(30 * time.Millisecond)
	write(t, root, "guide/second.qmd", "---\ntitle: Second\norder: 1\n---\n# Second, edited\n")
	if e := next(); e != "2" {
		t.Fatalf("event %q, want render 2", e)
	}
	page, err := http.Get(srv.URL + "/guide/second.html")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(page.Body)
	page.Body.Close()
	if !strings.Contains(string(b), "Second, edited") || !strings.Contains(string(b), "data-gen=\"2\"") {
		t.Error("edited page not served with its render number")
	}
}

// preview.LIVE.5
func TestRefusesPathsOutsideSite(t *testing.T) {
	s := newServer(t, fixture(t), config.Request{})
	if err := s.Build(); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.URL.Path = "/../../etc/passwd"
	s.ServeHTTP(rec, req)
	if rec.Code == 200 {
		t.Errorf("served %q", req.URL.Path)
	}
}

// The project's folio.yml names where the preview listens, under preview:;
// --addr wins over it and is used as given, while the address from the
// project is a starting point the port search counts up from.
func TestTheListenAddressComesFromTheFlagOrTheProject(t *testing.T) {
	withYML := func(yml string) string {
		root := t.TempDir()
		write(t, root, "folio.yml", yml)
		return root
	}
	configured := withYML("preview: {baseurl: 0.0.0.0, port: 8300}\nweb: {port: 8200}\n")
	cases := []struct {
		name, flag, root string
		want             string
		search, warn     bool
	}{
		{"a project without a preview section", "", withYML("title: T\n"), "localhost:8198", true, false},
		{"the project", "", configured, "0.0.0.0:8300", true, false},
		{"the flag over the project", ":9000", configured, ":9000", false, false},
		{"a broken folio.yml", "", withYML("preview: {port: 70000}\n"), "localhost:8198", true, true},
	}
	for _, c := range cases {
		addr, search, err := listenTarget(c.flag, c.root)
		if addr != c.want || search != c.search || (err != nil) != c.warn {
			t.Errorf("%s: listenTarget = %q, %v, %v; want %q, %v, warning %v", c.name, addr, search, err, c.want, c.search, c.warn)
		}
	}
}
