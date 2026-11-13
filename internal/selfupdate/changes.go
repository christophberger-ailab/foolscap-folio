package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// maxList limits the release listing: a hundred releases with their notes.
const maxList = 8 << 20

// Changes returns the releases an update from release `from` to release
// `to` brings, newest first, each with its notes: every release after
// `from` up to and including `to`, so that an update that skips releases
// still shows what each of them changed. A development build has no place
// among the releases, so for it the changes are those of `to` alone.
//
// The releases are listed from the URL beside source -- .../releases for a
// source of .../releases/latest, as GitHub, Gitea and Forgejo have it. A
// source that has no listing, or whose listing fails, gives the newest
// release alone, when that is `to`.
func Changes(ctx context.Context, c *http.Client, source, from, to string) ([]Release, error) {
	all, err := list(ctx, c, source)
	if err != nil {
		latest, lerr := Latest(ctx, c, source)
		if lerr != nil {
			return nil, err
		}
		all = []Release{latest}
	}
	var out []Release
	for _, r := range all {
		if r.Draft || r.Prerelease || !IsRelease(r.Version) || Newer(to, r.Version) {
			continue
		}
		if r.Version == to || (IsRelease(from) && Newer(from, r.Version)) {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b Release) int {
		if Newer(a.Version, b.Version) {
			return 1
		}
		if Newer(b.Version, a.Version) {
			return -1
		}
		return 0
	})
	return out, nil
}

// list fetches every release the source's repository has, as far as one
// page of a hundred goes.
func list(ctx context.Context, c *http.Client, source string) ([]Release, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, err
	}
	dir, ok := strings.CutSuffix(u.Path, "/releases/latest")
	if !ok {
		return nil, fmt.Errorf("%s: no release listing beside it", u.Redacted())
	}
	u.Path = dir + "/releases"
	u.RawQuery = "per_page=100"
	b, err := get(ctx, c, u.String(), maxList)
	if err != nil {
		return nil, err
	}
	var rs []Release
	if err := json.Unmarshal(b, &rs); err != nil {
		return nil, fmt.Errorf("%s: not a release list: %w", u.Redacted(), err)
	}
	return rs, nil
}
