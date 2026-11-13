package selfupdate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// releaseList serves a GitHub-shaped repository: the newest release at
// /r/releases/latest and all of them at /r/releases. Without list, the
// listing answers 404.
func releaseList(t *testing.T, list bool) *httptest.Server {
	t.Helper()
	all := []map[string]any{
		{"tag_name": "v0.1.4", "body": "four"},
		{"tag_name": "v0.2.0-rc1", "body": "pre", "prerelease": true},
		{"tag_name": "v0.1.3", "body": "three", "html_url": "https://x/v0.1.3"},
		{"tag_name": "v0.1.9", "body": "draft", "draft": true},
		{"tag_name": "v0.1.1", "body": "one"},
		{"tag_name": "v0.1.2", "body": "two"},
		{"tag_name": "v0.1.0", "body": "zero"},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /r/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(all[0])
	})
	if list {
		mux.HandleFunc("GET /r/releases", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("per_page") != "100" {
				t.Errorf("listing asked without per_page=100: %s", r.URL)
			}
			json.NewEncoder(w).Encode(all)
		})
	}
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func versions(rs []Release) []string {
	var v []string
	for _, r := range rs {
		v = append(v, r.Version)
	}
	return v
}

func TestChangesListsEveryReleaseInBetweenNewestFirst(t *testing.T) {
	srv := releaseList(t, true)
	got, err := Changes(context.Background(), srv.Client(), srv.URL+"/r/releases/latest", "v0.1.0", "v0.1.3")
	if err != nil {
		t.Fatal(err)
	}
	if v := versions(got); len(v) != 3 || v[0] != "v0.1.3" || v[1] != "v0.1.2" || v[2] != "v0.1.1" {
		t.Errorf("Changes = %v, want v0.1.3 v0.1.2 v0.1.1", v)
	}
	if got[0].Notes != "three" || got[0].URL != "https://x/v0.1.3" {
		t.Errorf("notes not carried: %+v", got[0])
	}
}

func TestChangesForADevelopmentBuildAreTheTargetsOnly(t *testing.T) {
	srv := releaseList(t, true)
	got, err := Changes(context.Background(), srv.Client(), srv.URL+"/r/releases/latest", "dev", "v0.1.4")
	if err != nil || len(got) != 1 || got[0].Version != "v0.1.4" || got[0].Notes != "four" {
		t.Errorf("Changes from dev = %v, %v", versions(got), err)
	}
}

// A source that is not a releases/latest URL -- a static file -- has no
// listing; its one release is all there is to show.
func TestChangesWithoutAListing(t *testing.T) {
	srv := releaseList(t, false)
	for _, source := range []string{srv.URL + "/r/releases/latest"} {
		got, err := Changes(context.Background(), srv.Client(), source, "v0.1.0", "v0.1.4")
		if len(got) != 1 || got[0].Version != "v0.1.4" {
			t.Errorf("%s: Changes = %v, %v; want the newest release alone", source, versions(got), err)
		}
	}
}
