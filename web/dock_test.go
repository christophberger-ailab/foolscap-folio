package web

import (
	"net/url"
	"strings"
	"testing"
)

// UI.14: the panes beside the editor stand in a dock of tabbed groups. The
// page carries the dock and every pane, each named by data-pane, and none
// of the buttons, popups, or pane heads the dock replaces.
func TestPanesStandInTheDock(t *testing.T) {
	srv, _ := testServer(t)
	post(t, srv, "/open", url.Values{"path": {fixture(t)}})
	body := get(t, srv, "/").Body.String()
	for _, want := range []string{
		`id="dock"`,
		`id="dock-divider"`,
		`id="pane-menu-button"`,
		`id="comments-pane" data-pane="comments"`,
		`id="help-pane" data-pane="help"`,
		`id="render-pane" data-pane="render"`,
		`id="git-pane" data-pane="git"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not carry %s", want)
		}
	}
	for _, unwanted := range []string{`<details class="render"`, `<details class="git"`, `class="pane-toggle"`, `class="side-pane-head"`, `id="render-panel"`, `id="git-panel"`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("page still carries %s", unwanted)
		}
	}
	// The dock stands beside the editor, not inside what a page switch
	// replaces: the Git and Render panes outlive the page.
	content := strings.Index(body, `<div id="content">`)
	dock := strings.Index(body, `id="dock"`)
	if content < 0 || dock < 0 {
		t.Fatalf("content at %d, dock at %d", content, dock)
	}
	if git := strings.Index(body, `id="git-pane"`); git < content {
		t.Error("the Git pane is not placed after the editor")
	}
}

// UI.14-1: the panes of a page -- the preview and the copyedit tool -- come
// with the page, the others with the project.
func TestPagePanesComeWithThePage(t *testing.T) {
	srv, _ := testServer(t)
	body := get(t, srv, "/content?path=chapter2/second.qmd").Body.String()
	for _, want := range []string{`id="preview-pane" data-pane="preview"`, `id="copyedit" data-pane="copyedit"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page switch does not bring %s", want)
		}
	}
	for _, unwanted := range []string{`id="git-pane"`, `id="render-pane"`, `id="help-pane"`, `id="comments-pane"`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("page switch replaces %s, which belongs to the project", unwanted)
		}
	}
}

// UI.14-2: the Render and Git panes belong to the project, so opening one
// brings them in the #main swap; the top bar no longer holds them.
func TestOpenBringsTheProjectPanes(t *testing.T) {
	srv, _ := testServer(t)
	body := post(t, srv, "/open", url.Values{"path": {fixture(t)}}).Body.String()
	for _, want := range []string{`id="render-pane" data-pane="render"`, `id="git-pane" data-pane="git"`, `id="dock"`} {
		if !strings.Contains(body, want) {
			t.Errorf("open does not bring %s", want)
		}
	}
	for _, unwanted := range []string{`innerHTML:#render-panel`, `innerHTML:#git-panel`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("open still swaps %s out of band", unwanted)
		}
	}
}

// UI.14-3: without a project there is no Git or Render pane to offer, but
// the help and the dock are there.
func TestDockWithoutAProject(t *testing.T) {
	srv := bareServer(t)
	body := get(t, srv, "/").Body.String()
	for _, want := range []string{`id="dock"`, `id="help-pane" data-pane="help"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page without a project does not carry %s", want)
		}
	}
	for _, unwanted := range []string{`id="git-pane"`, `id="render-pane"`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("page without a project carries %s", unwanted)
		}
	}
}

// UI.14: what moves tabs, splits groups, and remembers the layout runs in
// the browser; the Go test checks that the scripts and the stylesheet carry
// it.
func TestDockScripts(t *testing.T) {
	for file, wants := range map[string][]string{
		"assets/static/dock.js": {"function renderDock(", "function moveTab(", "function splitGroup(", "function closePane(", "function openPane(",
			"localStorage.setItem(DOCK_KEY", "'folio-shown'", "function dropZone(", "function migrateLayout("},
		"assets/static/app.js":  {"function updateComments(", "focusComment(", "replyToComment(", "removeComment("},
		"assets/static/app.css": {".dock-group", ".dock-tab", ".dock-tab.active", ".dock-split", ".dock-sash", ".dock-drop", ".comments-entry", ".pane-menu"},
		"assets/templates/app.html.tmpl": {`<script src="/static/dock.js" defer></script>`},
	} {
		src, err := assets.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range wants {
			if !assetContains(file, string(src), want) {
				t.Errorf("%s does not carry %q", file, want)
			}
		}
	}
}
