package shortcode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFormatDate(t *testing.T) {
	d := time.Date(2026, time.March, 5, 0, 0, 0, 0, time.UTC)
	for layout, want := range map[string]string{
		"":            "2026-03-05",
		"iso":         "2026-03-05",
		"DD.MM.YYYY":  "05.03.2026",
		"MM/DD/YY":    "03/05/26",
		"MON D, YYYY": "March 5, 2026",
		"D. MON YYYY": "5. March 2026",
		"M/D/YY":      "3/5/26",
		"YYYYMMDD":    "20260305",
	} {
		if got := FormatDate(d, layout, "en"); got != want {
			t.Errorf("%q: %q, want %q", layout, got, want)
		}
	}
	for lang, want := range map[string]string{"de": "5. März 2026", "de-AT": "5. März 2026", "fr": "5. mars 2026", "xx": "5. March 2026"} {
		if got := FormatDate(d, "D. MON YYYY", lang); got != want {
			t.Errorf("%s: %q, want %q", lang, got, want)
		}
	}
}

// The variables have a date: the vars' or _variables.yml's date, else
// today, formatted by date_format.
func TestVariablesDate(t *testing.T) {
	defer func(f func() time.Time) { now = f }(now)
	now = func() time.Time { return time.Date(2026, time.November, 18, 23, 0, 0, 0, time.UTC) }

	root := t.TempDir()
	date := func(lang string, vars ...map[string]string) string {
		v, err := Variables(root, lang, nil, vars...)
		if err != nil {
			t.Fatal(err)
		}
		return v["date"]
	}
	if got := date("en"); got != "2026-11-18" {
		t.Errorf("today: %q", got)
	}
	if got := date("en", map[string]string{"date_format": "MON D, YYYY"}); got != "November 18, 2026" {
		t.Errorf("today, formatted: %q", got)
	}
	if err := os.WriteFile(filepath.Join(root, "_variables.yml"), []byte("date: 2026-03-05\ndate_format: DD.MM.YYYY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := date("de"); got != "05.03.2026" {
		t.Errorf("file's date: %q", got)
	}
	if got := date("de", map[string]string{"date": "spring 2026"}); got != "spring 2026" {
		t.Errorf("a date that is not one stays as written: %q", got)
	}
}

// Variables resolve from the page's front matter, then _variables.yml,
// then the vars of folio.yml (issue #115).
func TestVariablesPrecedence(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "_variables.yml"), []byte("version: 1.3\nbuild: 7\nmeta: {owner: File}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vars, err := Variables(root, "en", map[string]string{"version": "0.9", "build": "1", "owner": "Acme"}, map[string]string{"build": "8"})
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"version": "1.3", "build": "8", "owner": "Acme", "meta.owner": "File"} {
		if vars[k] != want {
			t.Errorf("%s = %q, want %q", k, vars[k], want)
		}
	}
}

// A _variables.yml that is not YAML is an error, not silently ignored.
func TestVariablesBadFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "_variables.yml"), []byte("version: [1.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vars, err := Variables(root, "en", map[string]string{"owner": "Acme"})
	if err == nil || !strings.Contains(err.Error(), "_variables.yml") {
		t.Errorf("err %v", err)
	}
	if vars["owner"] != "Acme" || vars["date"] == "" {
		t.Errorf("vars %v", vars)
	}
}
