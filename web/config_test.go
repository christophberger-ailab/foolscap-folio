package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func configTestServer(t *testing.T) (*server, string) {
	t.Helper()
	root := fixture(t)
	prefs := filepath.Join(t.TempDir(), configFileName)
	// The tests set up the editing tasks they need, so they start from a
	// config without the shipped ones.
	blank := defaultConfig()
	blank.Copyedit.Prompts = nil
	if err := saveConfigFile(prefs, blank); err != nil {
		t.Fatal(err)
	}
	srv, err := newServer(prefs)
	if err != nil {
		t.Fatal(err)
	}
	rec := post(t, srv, "/open", url.Values{"path": {root}})
	if rec.Code != http.StatusOK {
		t.Fatalf("open: status %d: %s", rec.Code, rec.Body)
	}
	return srv, root
}

func TestAppLinksConfigAndPreviewStylesheet(t *testing.T) {
	srv, _ := configTestServer(t)
	body := get(t, srv, "/").Body.String()
	if !strings.Contains(body, `<a class="button topbar-config" href="/config">Config</a>`) {
		t.Errorf("page missing top-bar Config button:\n%s", body)
	}
	app := strings.Index(body, `href="/static/app.css"`)
	custom := strings.Index(body, `href="/config/preview.css?file=`)
	switch {
	case app < 0:
		t.Fatalf("page does not link app.css:\n%s", body)
	case custom < 0:
		t.Fatalf("page does not link custom preview stylesheet:\n%s", body)
	case custom < app:
		t.Fatalf("custom preview stylesheet is linked before app.css:\n%s", body)
	}
}

// The Config overview leads to the configuration files by one entry: the
// page that holds all of them as tabs (#91).
func TestConfigPageListsTheFilesOnce(t *testing.T) {
	srv, _ := configTestServer(t)
	rec := get(t, srv, "/config")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `href="/config/files"`) || !strings.Contains(body, "Configuration files") {
		t.Errorf("config page does not lead to the configuration files:\n%s", body)
	}
	for _, gone := range []string{"/config/preview-css", "/config/project-file", "/config/brand-file", "/config/settings-file"} {
		if strings.Contains(body, gone) {
			t.Errorf("config page still links %s:\n%s", gone, body)
		}
	}
}

// Every file has a tab on the one page, and the page marks the tab shown.
func TestConfigFilesPageHasATabPerFile(t *testing.T) {
	srv, _ := configTestServer(t)
	if rec := get(t, srv, "/config/files"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "folio.yml") {
		t.Fatalf("/config/files: status %d:\n%s", rec.Code, rec.Body)
	}
	for _, tab := range []string{"css", "project", "brand", "settings"} {
		body := get(t, srv, "/config/files/"+tab).Body.String()
		for _, other := range []string{"css", "project", "brand", "settings"} {
			if !strings.Contains(body, `href="/config/files/`+other+`"`) {
				t.Errorf("tab %s does not link tab %s:\n%s", tab, other, body)
			}
		}
		if !strings.Contains(body, `href="/config/files/`+tab+`" aria-current="page"`) {
			t.Errorf("tab %s is not marked as the one shown:\n%s", tab, body)
		}
	}
	if rec := get(t, srv, "/config/files/nosuch"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown tab: status %d, want 404", rec.Code)
	}
}

// cssDir is where the fixture project keeps its preview stylesheets.
func cssDir(root string) string { return filepath.Join(root, filepath.FromSlash(projectCSSDir)) }

func writeCSS(t *testing.T, root, name, css string) {
	t.Helper()
	if err := os.MkdirAll(cssDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cssDir(root), name), []byte(css), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewCSSEditorServesCodeMirrorCSSMode(t *testing.T) {
	srv, _ := configTestServer(t)
	rec := get(t, srv, "/config/files/css")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<textarea name="text" class="config-file-content" data-mode="css"`,
		`src="/static/codemirror/codemirror.js"`,
		`src="/static/codemirror/css.js"`,
		`src="/static/config.js"`,
		"assets/styles/folio",
		".markdown-preview h1 { color: rebeccapurple; }",
		`href="/config"`,
		`href="/"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("editor page missing %q:\n%s", want, body)
		}
	}
	before := func(first, second string) {
		t.Helper()
		i, j := strings.Index(body, first), strings.Index(body, second)
		switch {
		case i < 0:
			t.Fatalf("editor page does not load %s", first)
		case j < 0:
			t.Fatalf("editor page does not load %s", second)
		case i > j:
			t.Fatalf("%s is loaded after %s:\n%s", first, second, body)
		}
	}
	before("/static/codemirror/codemirror.js", "/static/codemirror/css.js")
	before("/static/codemirror/css.js", "/static/config.js")
	if rec := get(t, srv, "/static/codemirror/css.js"); rec.Code != http.StatusOK {
		t.Errorf("GET css mode: status %d, want 200", rec.Code)
	}
}

// Saving a stylesheet writes it into the project, where the preview reads
// it from (#89).
func TestPreviewCSSSaveAndServe(t *testing.T) {
	srv, root := configTestServer(t)
	css := ".markdown-preview h1 { color: red; }\n"
	rec := post(t, srv, "/config/files/css", url.Values{"file": {defaultCSSName}, "text": {css}})
	if rec.Code != http.StatusOK {
		t.Fatalf("save: status %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "Saved.") {
		t.Errorf("save response missing status:\n%s", rec.Body)
	}
	if got, err := os.ReadFile(filepath.Join(cssDir(root), defaultCSSName)); err != nil || string(got) != css {
		t.Fatalf("stored CSS = %q, %v; want %q", got, err, css)
	}

	rec = get(t, srv, "/config/preview.css?file="+defaultCSSName)
	if rec.Code != http.StatusOK {
		t.Fatalf("stylesheet: status %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/css" {
		t.Errorf("Content-Type %q, want text/css", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control %q, want no-store", cc)
	}
	if rec.Body.String() != css {
		t.Errorf("stylesheet body = %q, want %q", rec.Body.String(), css)
	}
}

func TestPreviewCSSMalformedFormKeepsSavedCSS(t *testing.T) {
	srv, _ := configTestServer(t)
	css := ".markdown-preview p { color: blue; }\n"
	if rec := post(t, srv, "/config/files/css", url.Values{"file": {defaultCSSName}, "text": {css}}); rec.Code != http.StatusOK {
		t.Fatalf("initial save: status %d: %s", rec.Code, rec.Body)
	}

	req := httptest.NewRequest("POST", "/config/files/css", strings.NewReader("file="+defaultCSSName+"&text=a;b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "invalid semicolon") {
		t.Errorf("malformed response missing parse error:\n%s", body)
	}
	if strings.Contains(body, "Saved.") {
		t.Errorf("malformed response claims success:\n%s", body)
	}

	rec = get(t, srv, "/config/preview.css?file="+defaultCSSName)
	if rec.Body.String() != css {
		t.Errorf("stylesheet body = %q, want %q", rec.Body.String(), css)
	}
}

func TestPreviewCSSEmptyFieldClearsStylesheet(t *testing.T) {
	srv, _ := configTestServer(t)
	css := ".markdown-preview p { color: blue; }\n"
	if rec := post(t, srv, "/config/files/css", url.Values{"file": {defaultCSSName}, "text": {css}}); rec.Code != http.StatusOK {
		t.Fatalf("initial save: status %d: %s", rec.Code, rec.Body)
	}

	rec := post(t, srv, "/config/files/css", url.Values{"file": {defaultCSSName}, "text": {""}})
	if rec.Code != http.StatusOK {
		t.Fatalf("clear: status %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "Saved.") {
		t.Errorf("clear response missing status:\n%s", rec.Body)
	}
	rec = get(t, srv, "/config/preview.css?file="+defaultCSSName)
	if rec.Body.Len() != 0 {
		t.Errorf("stylesheet body = %q, want empty", rec.Body.String())
	}
}

// A project without stylesheets of its own is previewed in the built-in
// one, and nothing is written for it: neither into the project nor beside
// the user's settings (#89).
func TestDefaultCSSIsBakedIn(t *testing.T) {
	srv, root := configTestServer(t)
	rec := get(t, srv, "/config/preview.css")
	if rec.Code != http.StatusOK {
		t.Fatalf("stylesheet: status %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/css" {
		t.Errorf("Content-Type %q, want text/css", ct)
	}
	if rec.Body.String() != defaultCustomCSS {
		t.Errorf("stylesheet body = %q, want the baked-in default", rec.Body.String())
	}
	if _, err := os.Stat(cssDir(root)); err == nil {
		t.Error("the project got a stylesheet directory it did not ask for")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(srv.configFile), "custom-css")); err == nil {
		t.Error("stylesheets were written beside the user's settings")
	}
	if rec := get(t, srv, "/"); rec.Code != http.StatusOK {
		t.Errorf("app page after baked-in default: status %d", rec.Code)
	}
}

func TestPreviewCSSRouteWithoutConfigDir(t *testing.T) {
	srv, _ := testServer(t)
	rec := get(t, srv, "/config/preview.css")
	if rec.Code != http.StatusOK {
		t.Fatalf("stylesheet: status %d: %s", rec.Code, rec.Body)
	}
	if rec.Body.String() != defaultCustomCSS {
		t.Errorf("stylesheet body = %q, want the baked-in default", rec.Body.String())
	}
}

// The preview's dropdown offers the project's stylesheets, .css and .scss,
// once there is more than one to choose from (#89).
func TestPreviewOffersTheProjectsStylesheets(t *testing.T) {
	srv, root := configTestServer(t)
	if body := get(t, srv, "/content?path=index.qmd").Body.String(); strings.Contains(body, `id="preview-css-select"`) {
		t.Errorf("content pane offers a stylesheet dropdown without stylesheets:\n%s", body)
	}
	writeCSS(t, root, "all.scss", "/* all */\n")
	writeCSS(t, root, "pol.css", "/* pol */\n")
	writeCSS(t, root, "notes.txt", "not a stylesheet")

	body := get(t, srv, "/content?path=index.qmd").Body.String()
	if !strings.Contains(body, `id="preview-css-select"`) {
		t.Fatalf("content pane missing stylesheet dropdown once two files exist:\n%s", body)
	}
	if preview, sel := strings.Index(body, `id="preview"`), strings.Index(body, `id="preview-css-select"`); !(preview >= 0 && preview < sel) {
		t.Errorf("the stylesheet dropdown is not below the preview:\n%s", body)
	}
	for _, want := range []string{`value="all.scss"`, `value="pol.css"`} {
		if !strings.Contains(body, want) {
			t.Errorf("stylesheet dropdown missing option %q:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{`value="notes.txt"`, `value="custom.css"`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("stylesheet dropdown offers %q:\n%s", unwanted, body)
		}
	}
	if got := get(t, srv, "/config/preview.css?file=pol.css").Body.String(); got != "/* pol */\n" {
		t.Errorf("pol.css served as %q", got)
	}
}

// The CSS tab edits every stylesheet of the project and adds new ones
// there (#91).
func TestAddAlternateCSSFile(t *testing.T) {
	srv, root := configTestServer(t)
	writeCSS(t, root, "pol.css", "/* pol */\n")

	if body := get(t, srv, "/config/files/css").Body.String(); strings.Contains(body, `id="css-file-select"`) {
		t.Errorf("single-file editor page unexpectedly offers a file switcher:\n%s", body)
	}

	rec := post(t, srv, "/config/files/css/new", url.Values{"name": {"dark"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("new stylesheet: status %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "Created.") {
		t.Errorf("new stylesheet response missing status:\n%s", rec.Body)
	}
	if _, err := os.Stat(filepath.Join(cssDir(root), "dark.css")); err != nil {
		t.Fatalf("dark.css was not created in the project: %v", err)
	}
	for _, name := range []string{"../evil", "pol"} {
		body := post(t, srv, "/config/files/css/new", url.Values{"name": {name}}).Body.String()
		if !strings.Contains(body, `class="error"`) {
			t.Errorf("new stylesheet %q was not refused:\n%s", name, body)
		}
	}

	body := get(t, srv, "/config/files/css?file=pol.css").Body.String()
	for _, want := range []string{`id="css-file-select"`, `value="dark.css"`, `value="pol.css" selected`, "/* pol */"} {
		if !strings.Contains(body, want) {
			t.Errorf("editor page missing %q after adding a stylesheet:\n%s", want, body)
		}
	}
}

// TestSwitchActiveCSS checks that the dropdown's persisted choice changes
// which stylesheet the main app's <link> and /config/preview.css serve.
func TestSwitchActiveCSS(t *testing.T) {
	srv, root := configTestServer(t)
	writeCSS(t, root, "a.css", "/* a */\n")
	writeCSS(t, root, "dark.css", "/* dark */\n")

	rec := post(t, srv, "/config/active-css", url.Values{"file": {"dark.css"}})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("select active: status %d: %s", rec.Code, rec.Body)
	}

	body := get(t, srv, "/").Body.String()
	if !strings.Contains(body, `href="/config/preview.css?file=dark.css"`) {
		t.Errorf("app page does not link the newly active stylesheet:\n%s", body)
	}
	rec = get(t, srv, "/config/preview.css")
	if rec.Body.String() != "/* dark */\n" {
		t.Errorf("stylesheet body = %q, want the active file's content", rec.Body.String())
	}

	if rec := post(t, srv, "/config/active-css", url.Values{"file": {"no-such.css"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("select unknown file: status %d, want 400", rec.Code)
	}
}
