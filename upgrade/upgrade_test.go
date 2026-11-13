package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// opts runs against a source whose newest release is latest. The release
// has no assets, so an install attempt fails with "has no build for".
func opts(t *testing.T, current, latest, answer string) (Options, *bytes.Buffer) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"tag_name": latest, "html_url": "https://example.com/r", "assets": []any{}})
	}))
	t.Cleanup(srv.Close)
	exe := filepath.Join(t.TempDir(), "folio")
	os.WriteFile(exe, []byte("old"), 0o755)
	var out bytes.Buffer
	return Options{
		Current: current, Source: srv.URL, Exe: exe, Client: srv.Client(),
		In: strings.NewReader(answer), Out: &out,
	}, &out
}

func TestUpToDate(t *testing.T) {
	o, out := opts(t, "v0.4.0", "v0.4.0", "")
	if err := Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "v0.4.0 is the newest release") {
		t.Errorf("output: %s", out)
	}
}

func TestCheckOnlyReportsAndInstallsNothing(t *testing.T) {
	o, out := opts(t, "v0.4.0", "v0.5.0", "")
	o.CheckOnly = true
	if err := Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "folio v0.5.0 is available (you have v0.4.0)") {
		t.Errorf("output: %s", out)
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Errorf("--check asked to update: %s", out)
	}
}

func TestDecliningLeavesTheBinary(t *testing.T) {
	for _, answer := range []string{"n\n", "\n", ""} {
		o, out := opts(t, "v0.4.0", "v0.5.0", answer)
		if err := Run(context.Background(), o); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Update now? [y/N]") || !strings.Contains(out.String(), "Not updated") {
			t.Errorf("answer %q: %s", answer, out)
		}
	}
}

func TestAcceptingInstalls(t *testing.T) {
	for _, o := range []func(*Options){
		func(o *Options) { o.In = strings.NewReader("y\n") },
		func(o *Options) { o.Yes = true },
	} {
		opt, _ := opts(t, "v0.4.0", "v0.5.0", "")
		o(&opt)
		// The fake release carries no build, so getting this far is
		// getting to the install.
		if err := Run(context.Background(), opt); err == nil || !strings.Contains(err.Error(), "has no build for") {
			t.Errorf("Run: %v, want the install to be attempted", err)
		}
	}
}

func TestADevelopmentBuildIsNotUpdated(t *testing.T) {
	o, _ := opts(t, "dev", "v0.5.0", "y\n")
	if err := Run(context.Background(), o); err == nil || !strings.Contains(err.Error(), "development build") {
		t.Errorf("Run on dev: %v", err)
	}
}
