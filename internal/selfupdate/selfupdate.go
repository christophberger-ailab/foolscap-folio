// Package selfupdate finds folio's newest release and puts it in place of
// the running binary.
//
// A release is described the way GitHub's REST API describes one
// (GET /repos/{owner}/{repo}/releases/latest): a tag name and a list of
// named assets with their download URLs. Gitea and Forgejo answer in the
// same shape, and so does a static JSON file, so the source is a URL rather
// than a repository name.
//
// What makes an update safe to apply:
//
//   - Every request goes over HTTPS, redirects included.
//   - The archive is checked against the SHA-256 the release lists for it in
//     SHA256SUMS.txt before anything is unpacked; a release without that
//     file, or without the archive in it, is not installed.
//   - Every download is size-limited.
//   - The new binary is written next to the old one and renamed over it, so
//     a failure at any point leaves the old binary as it was.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DefaultSource is where folio's releases are published.
const DefaultSource = "https://api.github.com/repos/christophberger-ailab/foolscap-folio/releases/latest"

// checksumsName is the release asset that lists every archive's SHA-256,
// in the format sha256sum writes.
const checksumsName = "SHA256SUMS.txt"

// Download limits. A folio archive is a few tens of megabytes.
const (
	maxJSON    = 1 << 20
	maxSums    = 64 << 10
	maxArchive = 256 << 20
)

// Release is one published version of folio.
type Release struct {
	Version string  `json:"tag_name"`
	URL     string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// Asset is one downloadable file of a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// Client returns the HTTP client updates should use: it gives up after a
// few minutes and refuses to follow a redirect away from HTTPS.
func Client() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" {
				return fmt.Errorf("redirect to %s: updates are only fetched over https", req.URL.Redacted())
			}
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
}

// Latest fetches the description of the newest release from source.
func Latest(ctx context.Context, c *http.Client, source string) (Release, error) {
	b, err := get(ctx, c, source, maxJSON)
	if err != nil {
		return Release{}, err
	}
	var rel Release
	if err := json.Unmarshal(b, &rel); err != nil {
		return Release{}, fmt.Errorf("%s: not a release description: %w", source, err)
	}
	if rel.Version == "" {
		return Release{}, fmt.Errorf("%s: the release names no version", source)
	}
	return rel, nil
}

// get fetches url over HTTPS, reading at most limit bytes.
func get(ctx context.Context, c *http.Client, rawURL string, limit int64) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("%s: updates are only fetched over https", u.Redacted())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json, application/octet-stream;q=0.9, */*;q=0.8")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", u.Redacted(), resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s: larger than %d bytes", u.Redacted(), limit)
	}
	return b, nil
}

// IsRelease reports whether version is a release folio can compare, as
// opposed to a development build ("dev").
func IsRelease(version string) bool {
	_, ok := parse(version)
	return ok
}

// Newer reports whether latest is a later release than current. Either one
// not being a plain vMAJOR.MINOR.PATCH -- a development build, a
// pre-release -- makes it false: there is nothing to compare.
func Newer(current, latest string) bool {
	c, ok1 := parse(current)
	l, ok2 := parse(latest)
	if !ok1 || !ok2 {
		return false
	}
	for i := range c {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// parse reads [v]MAJOR.MINOR.PATCH.
func parse(v string) ([3]int, bool) {
	var n [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return n, false
	}
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil || x < 0 {
			return n, false
		}
		n[i] = x
	}
	return n, true
}

// AssetName is the archive the release workflow publishes for a platform.
func AssetName(version, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("qm_%s_%s_%s%s", version, goos, goarch, ext)
}

// binaryName is the file the archive for goos holds.
func binaryName(goos string) string {
	if goos == "windows" {
		return "folio.exe"
	}
	return "folio"
}

// Executable is the path of the running binary, with symlinks resolved so
// that an update replaces the file rather than the link to it.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// Tidy removes what an update on Windows had to leave behind: the old
// binary, which cannot be deleted while it runs.
func Tidy() {
	if exe, err := Executable(); err == nil {
		os.Remove(oldName(exe))
	}
}

func oldName(exe string) string { return exe + ".old" }

// Install downloads rel's build for this platform, checks it against the
// release's checksums, and puts it in place of the binary at exe.
func Install(ctx context.Context, c *http.Client, rel Release, exe string) error {
	name := AssetName(rel.Version, runtime.GOOS, runtime.GOARCH)
	archive, sums := rel.asset(name), rel.asset(checksumsName)
	if archive == nil {
		return fmt.Errorf("release %s has no build for %s/%s", rel.Version, runtime.GOOS, runtime.GOARCH)
	}
	if sums == nil {
		return fmt.Errorf("release %s has no %s to check the download against", rel.Version, checksumsName)
	}
	list, err := get(ctx, c, sums.URL, maxSums)
	if err != nil {
		return err
	}
	want, err := checksumFor(list, name)
	if err != nil {
		return err
	}
	data, err := get(ctx, c, archive.URL, maxArchive)
	if err != nil {
		return err
	}
	if got := sha256.Sum256(data); hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("%s: checksum mismatch; the download was not installed", name)
	}
	bin, err := unpack(data, name)
	if err != nil {
		return err
	}
	return replace(exe, bin)
}

func (r Release) asset(name string) *Asset {
	for i := range r.Assets {
		if r.Assets[i].Name == name {
			return &r.Assets[i]
		}
	}
	return nil
}

// checksumFor finds name's SHA-256 in a list written by sha256sum.
func checksumFor(list []byte, name string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(list))
	for sc.Scan() {
		sum, file, ok := strings.Cut(sc.Text(), " ")
		if ok && strings.TrimPrefix(strings.TrimSpace(file), "*") == name {
			return strings.ToLower(sum), nil
		}
	}
	return "", fmt.Errorf("%s does not list %s", checksumsName, name)
}

// unpack returns the folio binary from the archive called name.
func unpack(data []byte, name string) ([]byte, error) {
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if path.Base(f.Name) == binaryName("windows") && !f.FileInfo().IsDir() {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return readLimited(rc)
			}
		}
		return nil, fmt.Errorf("%s holds no %s", name, binaryName("windows"))
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%s holds no %s", name, binaryName(runtime.GOOS))
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && path.Base(h.Name) == binaryName(runtime.GOOS) {
			return readLimited(tr)
		}
	}
}

// readLimited reads an unpacked binary, which can be no larger than the
// archive limit: a compressed archive can expand to far more than it is.
func readLimited(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxArchive+1))
	if err == nil && len(b) > maxArchive {
		err = errors.New("unpacked binary is too large")
	}
	return b, err
}

// replace writes bin next to exe and renames it over exe. Windows does not
// let a running binary be overwritten, but it lets it be renamed: there the
// old one is moved aside first and removed by Tidy on a later start.
func replace(exe string, bin []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".folio-update-*")
	if err != nil {
		return fmt.Errorf("cannot write next to %s: %w", exe, err)
	}
	defer func() {
		if err != nil {
			os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp.Name(), 0o755); err != nil { // #nosec G302 -- an executable must be executable
		return err
	}
	if runtime.GOOS == "windows" {
		old := oldName(exe)
		os.Remove(old)
		if err = os.Rename(exe, old); err != nil {
			return err
		}
		if err = os.Rename(tmp.Name(), exe); err != nil {
			os.Rename(old, exe)
		}
		return err
	}
	return os.Rename(tmp.Name(), exe)
}
