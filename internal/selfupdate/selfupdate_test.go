package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		current, latest string
		want            bool
	}{
		{"v0.1.0", "v0.1.1", true},
		{"v0.1.9", "v0.2.0", true},
		{"v0.9.0", "v0.10.0", true},
		{"0.1.0", "v0.1.1", true},
		{"v1.0.0", "v0.9.9", false},
		{"v0.2.0", "v0.2.0", false},
		{"v0.3.0", "v0.2.9", false},
		{"dev", "v0.2.0", false},
		{"v0.2.0", "nightly", false},
		{"v0.2.0", "v0.3.0-rc1", false},
	} {
		if got := Newer(c.current, c.latest); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestIsRelease(t *testing.T) {
	if IsRelease("dev") || !IsRelease("v0.4.2") {
		t.Error("IsRelease tells a build from a release wrongly")
	}
}

// fakeRelease serves a release in GitHub's format: the release JSON at
// /latest and its assets beside it. tamper changes the archive after its
// checksum was taken.
func fakeRelease(t *testing.T, version, binary string, tamper bool) *httptest.Server {
	t.Helper()
	name := AssetName(version, runtime.GOOS, runtime.GOARCH)
	archive := packArchive(t, name, binary)
	sum := sha256.Sum256(archive)
	sums := hex.EncodeToString(sum[:]) + "  " + name + "\nffff  other.zip\n"
	if tamper {
		archive = append(archive, 0)
	}
	mux := http.NewServeMux()
	srv := httptest.NewTLSServer(mux)
	mux.HandleFunc("GET /latest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": version,
			"html_url": srv.URL + "/page",
			"assets": []map[string]any{
				{"name": name, "browser_download_url": srv.URL + "/a/" + name},
				{"name": checksumsName, "browser_download_url": srv.URL + "/a/" + checksumsName},
			},
		})
	})
	mux.HandleFunc("GET /a/"+name, func(w http.ResponseWriter, r *http.Request) { w.Write(archive) })
	mux.HandleFunc("GET /a/"+checksumsName, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(sums)) })
	t.Cleanup(srv.Close)
	return srv
}

// packArchive builds the archive the release workflow builds: the binary
// alone, in a .zip on Windows and a .tar.gz elsewhere.
func packArchive(t *testing.T, name, binary string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if strings.HasSuffix(name, ".zip") {
		zw := zip.NewWriter(&buf)
		f, err := zw.Create(binaryName("windows"))
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(binary))
		zw.Close()
		return buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "folio", Mode: 0o755, Size: int64(len(binary)), Typeflag: tar.TypeReg})
	tw.Write([]byte(binary))
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestLatest(t *testing.T) {
	srv := fakeRelease(t, "v0.5.0", "new", false)
	rel, err := Latest(context.Background(), srv.Client(), srv.URL+"/latest")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "v0.5.0" || rel.URL != srv.URL+"/page" || len(rel.Assets) != 2 {
		t.Errorf("Latest = %+v", rel)
	}
}

func TestLatestRefusesPlainHTTP(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if _, err := Latest(context.Background(), srv.Client(), srv.URL+"/latest"); err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("Latest over http: %v, want an error about https", err)
	}
}

func TestLatestReportsHTTPErrors(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	if _, err := Latest(context.Background(), srv.Client(), srv.URL+"/latest"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("Latest on a 404: %v", err)
	}
}

func TestClientRefusesRedirectToHTTP(t *testing.T) {
	plain := httptest.NewServer(http.NotFoundHandler())
	defer plain.Close()
	srv := httptest.NewTLSServer(http.RedirectHandler(plain.URL, http.StatusFound))
	defer srv.Close()
	c := Client()
	c.Transport = srv.Client().Transport
	if _, err := Latest(context.Background(), c, srv.URL); err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("redirect to http: %v, want an error about https", err)
	}
}

// exeFile writes a stand-in for the running binary.
func exeFile(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), binaryName(runtime.GOOS))
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestInstallReplacesTheBinary(t *testing.T) {
	srv := fakeRelease(t, "v0.5.0", "new", false)
	ctx := context.Background()
	rel, err := Latest(ctx, srv.Client(), srv.URL+"/latest")
	if err != nil {
		t.Fatal(err)
	}
	exe := exeFile(t)
	if err := Install(ctx, srv.Client(), rel, exe); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Errorf("binary holds %q after the update", b)
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(exe); st.Mode().Perm()&0o100 == 0 {
			t.Errorf("updated binary is not executable: %v", st.Mode())
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	if len(entries) != 1 {
		t.Errorf("update left files behind: %v", entries)
	}
}

func TestInstallRefusesABadChecksum(t *testing.T) {
	srv := fakeRelease(t, "v0.5.0", "evil", true)
	ctx := context.Background()
	rel, err := Latest(ctx, srv.Client(), srv.URL+"/latest")
	if err != nil {
		t.Fatal(err)
	}
	exe := exeFile(t)
	if err := Install(ctx, srv.Client(), rel, exe); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("Install with a tampered archive: %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Errorf("binary was replaced despite the bad checksum: %q", b)
	}
}

func TestInstallNeedsABuildForThisPlatform(t *testing.T) {
	rel := Release{Version: "v0.5.0", Assets: []Asset{{Name: checksumsName, URL: "https://example.invalid/s"}}}
	err := Install(context.Background(), http.DefaultClient, rel, exeFile(t))
	if err == nil || !strings.Contains(err.Error(), runtime.GOOS+"/"+runtime.GOARCH) {
		t.Errorf("Install without a matching asset: %v", err)
	}
}

func TestInstallNeedsChecksums(t *testing.T) {
	name := AssetName("v0.5.0", runtime.GOOS, runtime.GOARCH)
	rel := Release{Version: "v0.5.0", Assets: []Asset{{Name: name, URL: "https://example.invalid/a"}}}
	err := Install(context.Background(), http.DefaultClient, rel, exeFile(t))
	if err == nil || !strings.Contains(err.Error(), checksumsName) {
		t.Errorf("Install without checksums: %v", err)
	}
}

func TestChecksumFor(t *testing.T) {
	sums := []byte("aa  a.zip\nbb *b.tar.gz\n")
	if got, err := checksumFor(sums, "b.tar.gz"); err != nil || got != "bb" {
		t.Errorf("checksumFor = %q, %v", got, err)
	}
	if _, err := checksumFor(sums, "c.zip"); err == nil {
		t.Error("checksumFor finds a file the list lacks")
	}
}

func TestAssetName(t *testing.T) {
	if got := AssetName("v0.5.0", "linux", "amd64"); got != "qm_v0.5.0_linux_amd64.tar.gz" {
		t.Errorf("linux asset: %s", got)
	}
	if got := AssetName("v0.5.0", "windows", "arm64"); got != "qm_v0.5.0_windows_arm64.zip" {
		t.Errorf("windows asset: %s", got)
	}
}
