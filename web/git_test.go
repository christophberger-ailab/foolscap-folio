package web

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/gitrepo"
)

// The Git pane stands in the dock, and its body is fetched when the pane
// is shown rather than with the page.
func TestGitPaneIsInTheDock(t *testing.T) {
	srv, _ := testServer(t)
	body := get(t, srv, "/").Body.String()
	for _, want := range []string{`id="git-pane" data-pane="git"`, `id="git-body"`, `hx-get="/git"`, `hx-trigger="folio-shown from:#git-pane"`} {
		if !strings.Contains(body, want) {
			t.Errorf("git pane missing %q:\n%s", want, body)
		}
	}
}

// With no project open there is nothing to show a working tree of, so the
// pane is not there either.
func TestGitPaneNeedsAProject(t *testing.T) {
	srv := bareServer(t)
	if body := get(t, srv, "/").Body.String(); strings.Contains(body, `id="git-pane"`) {
		t.Errorf("git pane shown without a project:\n%s", body)
	}
}

// /open refreshes the pane along with the others: it belongs to the
// project, and comes with the #main swap.
func TestOpenRefreshesTheGitPane(t *testing.T) {
	srv, _ := testServer(t)
	body := post(t, srv, "/open", url.Values{"path": {fixture(t)}}).Body.String()
	if !strings.Contains(body, `id="git-pane"`) {
		t.Errorf("open does not refresh the git pane:\n%s", body)
	}
}

// A project that is not a working tree gets an explanation instead of a
// list of changes; the same route is what says so when git is missing.
func TestGitOutsideARepository(t *testing.T) {
	srv, _ := testServer(t)
	rec := get(t, srv, "/git")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="error"`) {
		t.Errorf("no explanation given:\n%s", body)
	}
	if strings.Contains(body, "Commit") {
		t.Errorf("commit offered outside a repository:\n%s", body)
	}
}

// The panel's operations need a project; without one they say so rather
// than run git somewhere unexpected.
func TestGitOperationsNeedAProject(t *testing.T) {
	srv := bareServer(t)
	for _, target := range []string{"/git/stage", "/git/unstage", "/git/commit", "/git/push", "/git/sync"} {
		rec := post(t, srv, target, url.Values{"path": {"index.qmd"}, "message": {"m"}})
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", target, rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), "no project open") {
			t.Errorf("%s does not report the missing project:\n%s", target, rec.Body)
		}
	}
}

// gitProject turns the test fixture into a repository with one commit, so
// the panel has a working tree to report on. It skips where git is not
// installed.
func gitProject(t *testing.T) (*server, string) {
	t.Helper()
	if !gitrepo.Available() {
		t.Skip("git is not installed")
	}
	srv, root := testServer(t)
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "folio@example.com"},
		{"config", "user.name", "QM Test"},
		{"add", "--all"},
		{"commit", "-m", "first"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return srv, root
}

// The panel lists the changes since the last commit, and staging moves a
// file from one list to the other.
func TestGitStageAndUnstage(t *testing.T) {
	srv, root := gitProject(t)
	writeFile(t, root, "chapter2/second.qmd", "---\ntitle: Second\norder: 1\n---\n# Edited\n")

	body := get(t, srv, "/git").Body.String()
	if !strings.Contains(body, "chapter2/second.qmd") {
		t.Fatalf("change not listed:\n%s", body)
	}
	if !strings.Contains(body, "Not staged") {
		t.Errorf("change not reported as unstaged:\n%s", body)
	}

	body = post(t, srv, "/git/stage", url.Values{"path": {"chapter2/second.qmd"}}).Body.String()
	if !strings.Contains(body, "Staged") {
		t.Errorf("file not staged:\n%s", body)
	}
	body = post(t, srv, "/git/unstage", url.Values{"path": {"chapter2/second.qmd"}}).Body.String()
	if strings.Contains(body, "<legend>Staged") {
		t.Errorf("file still staged:\n%s", body)
	}
}

// A panel with changes offers the filter, and every row carries the path
// the script matches it against; a clean tree has nothing to filter.
func TestGitFilter(t *testing.T) {
	srv, root := gitProject(t)
	if body := get(t, srv, "/git").Body.String(); strings.Contains(body, `id="git-filter"`) {
		t.Errorf("filter offered for a clean tree:\n%s", body)
	}
	writeFile(t, root, "chapter2/second.qmd", "---\ntitle: Second\norder: 1\n---\n# Edited\n")
	body := get(t, srv, "/git").Body.String()
	for _, want := range []string{`id="git-filter"`, `data-path="chapter2/second.qmd"`, `class="empty git-nomatch" hidden`} {
		if !strings.Contains(body, want) {
			t.Errorf("panel missing %q:\n%s", want, body)
		}
	}
}

// A commit takes the staged changes; the panel then reports a clean tree.
func TestGitCommit(t *testing.T) {
	srv, root := gitProject(t)
	writeFile(t, root, "chapter2/second.qmd", "---\ntitle: Second\norder: 1\n---\n# Edited\n")
	post(t, srv, "/git/stage", url.Values{"all": {"1"}})

	body := post(t, srv, "/git/commit", url.Values{"message": {"edit second"}}).Body.String()
	if !strings.Contains(body, "No changes since the last commit.") {
		t.Errorf("tree not clean after the commit:\n%s", body)
	}
}

// A commit without a message is refused, and the panel says why instead of
// leaving the user with an unexplained non-event.
func TestGitCommitWithoutMessage(t *testing.T) {
	srv, root := gitProject(t)
	writeFile(t, root, "chapter2/second.qmd", "---\ntitle: Second\norder: 1\n---\n# Edited\n")
	post(t, srv, "/git/stage", url.Values{"all": {"1"}})

	body := post(t, srv, "/git/commit", url.Values{"message": {"  "}}).Body.String()
	if !strings.Contains(body, "a commit needs a message") {
		t.Errorf("empty message not reported:\n%s", body)
	}
	if !strings.Contains(body, "<legend>Staged") {
		t.Errorf("the staged change was lost:\n%s", body)
	}
}

// A click on a path in either list opens that file's diff in the pane, and
// the pane offers to open the file in the editor.
func TestGitDiff(t *testing.T) {
	srv, root := gitProject(t)
	writeFile(t, root, "chapter2/second.qmd", "---\ntitle: Second\norder: 1\n---\n# Edited\n")

	// The list itself is what makes the diff reachable: the path is the
	// button that asks for it.
	body := get(t, srv, "/git").Body.String()
	if !strings.Contains(body, `hx-get="/git/diff?path=chapter2%2Fsecond.qmd"`) {
		t.Errorf("the path does not ask for a diff:\n%s", body)
	}
	if !strings.Contains(body, `id="git-diff"`) {
		t.Errorf("no pane for the diff to land in:\n%s", body)
	}

	body = get(t, srv, "/git/diff?path=chapter2/second.qmd").Body.String()
	if !strings.Contains(body, `class="git-dl git-dl-add"`) || !strings.Contains(body, "# Edited") {
		t.Errorf("the added line is missing:\n%s", body)
	}
	if !strings.Contains(body, `class="git-dl git-dl-del"`) {
		t.Errorf("the removed line is missing:\n%s", body)
	}
	if !strings.Contains(body, `class="git-dl git-dl-hunk"`) {
		t.Errorf("the hunk header is missing:\n%s", body)
	}
	// The pane reads only; the way to change the file is the editor.
	if !strings.Contains(body, `hx-get="/content?path=chapter2%2Fsecond.qmd"`) {
		t.Errorf("no way to open the file in the editor:\n%s", body)
	}
	if strings.Contains(body, "/git/stage") {
		t.Errorf("the diff pane offers to change the index:\n%s", body)
	}
}

// The two sides of a file changed in both places are two diffs, and the
// pane shows the one the list the click came from stands for.
func TestGitDiffStagedAndUnstaged(t *testing.T) {
	srv, root := gitProject(t)
	writeFile(t, root, "chapter2/second.qmd", "---\ntitle: Second\norder: 1\n---\n# Staged\n")
	post(t, srv, "/git/stage", url.Values{"path": {"chapter2/second.qmd"}})
	writeFile(t, root, "chapter2/second.qmd", "---\ntitle: Second\norder: 1\n---\n# Working tree\n")

	staged := get(t, srv, "/git/diff?path=chapter2/second.qmd&staged=1").Body.String()
	if !strings.Contains(staged, "# Staged") || strings.Contains(staged, "# Working tree") {
		t.Errorf("the staged diff is not what the index holds:\n%s", staged)
	}
	unstaged := get(t, srv, "/git/diff?path=chapter2/second.qmd").Body.String()
	if !strings.Contains(unstaged, "# Working tree") {
		t.Errorf("the unstaged diff is not what the working tree holds:\n%s", unstaged)
	}
}

// A file git has never seen has no other side to be diffed against, so its
// diff is its whole content, added.
func TestGitDiffUntracked(t *testing.T) {
	srv, root := gitProject(t)
	writeFile(t, root, "chapter2/third.qmd", "---\ntitle: Third\n---\n# Brand new\n")

	body := get(t, srv, "/git/diff?path=chapter2/third.qmd").Body.String()
	if !strings.Contains(body, "# Brand new") {
		t.Errorf("the new file's content is missing:\n%s", body)
	}
	if !strings.Contains(body, `class="git-dl git-dl-add"`) {
		t.Errorf("the content is not shown as added:\n%s", body)
	}
}

// The diff is a view of the open project and nothing else: a path that
// leads out of it is refused rather than read, which `git diff --no-index`
// would otherwise happily do for any file on the machine.
func TestGitDiffStaysInTheProject(t *testing.T) {
	srv, _ := gitProject(t)
	for _, path := range []string{"../outside.qmd", "/etc/passwd", ""} {
		rec := get(t, srv, "/git/diff?path="+url.QueryEscape(path))
		if rec.Code != http.StatusOK {
			t.Fatalf("%q: status %d: %s", path, rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), `class="error"`) {
			t.Errorf("%q was not refused:\n%s", path, rec.Body)
		}
	}
}

// Without a project there is no working tree to diff against, and the pane
// says so rather than running git somewhere unexpected.
func TestGitDiffNeedsAProject(t *testing.T) {
	srv := bareServer(t)
	body := get(t, srv, "/git/diff?path=index.qmd").Body.String()
	if !strings.Contains(body, "no project open") {
		t.Errorf("the missing project is not reported:\n%s", body)
	}
}

// The lines of a diff are classified by what they are, and the file
// headers above the first hunk are dropped: the pane's own header already
// names the file. A removed line that begins with dashes is a deletion,
// not one of those headers.
func TestDiffLines(t *testing.T) {
	out := strings.Join([]string{
		"diff --git a/a.qmd b/a.qmd",
		"index 1234567..89abcde 100644",
		"--- a/a.qmd",
		"+++ b/a.qmd",
		"@@ -1,4 +1,4 @@",
		" title: A",
		"-- an old note",
		"+new line",
		`\ No newline at end of file`,
	}, "\n")
	lines, added, removed, binary := diffLines(out)
	var kinds []string
	for _, l := range lines {
		kinds = append(kinds, l.Kind)
	}
	want := []string{"hunk", "ctx", "del", "add", "meta"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Errorf("kinds = %v, want %v", kinds, want)
	}
	if added != 1 || removed != 1 {
		t.Errorf("added = %d, removed = %d; want 1, 1", added, removed)
	}
	if binary {
		t.Error("a text diff read as binary")
	}
}

// A binary file has no lines to show and nothing the editor could open, so
// the pane says what git says and offers no more.
func TestDiffLinesBinary(t *testing.T) {
	_, _, _, binary := diffLines("diff --git a/i.png b/i.png\nBinary files a/i.png and b/i.png differ")
	if !binary {
		t.Error("a binary diff not recognized")
	}
}

// gitSubProject opens a project that sits inside a larger repository — a
// Quarto book in a documentation monorepo — rather than being one. The
// paths git then reports are the project's seen from the repository root,
// which is not where the project is.
func gitSubProject(t *testing.T) (srv *server, repo, proj string) {
	t.Helper()
	if !gitrepo.Available() {
		t.Skip("git is not installed")
	}
	repo = t.TempDir()
	proj = filepath.Join(repo, "doc", "Training", "book")
	if err := os.MkdirAll(filepath.Dir(proj), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(proj, os.DirFS(fixture(t))); err != nil {
		t.Fatal(err)
	}
	writeFile(t, repo, "README.md", "# The repository\n")
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "folio@example.com"},
		{"config", "user.name", "QM Test"},
		{"add", "--all"},
		{"commit", "-m", "first"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	srv, err := newServer("")
	if err != nil {
		t.Fatal(err)
	}
	if rec := post(t, srv, "/open", url.Values{"path": {proj}}); rec.Code != http.StatusOK {
		t.Fatalf("open: status %d: %s", rec.Code, rec.Body)
	}
	return srv, repo, proj
}

// A project inside a repository is the case the paths have to survive: git
// reports them from the repository root, and every operation the panel
// offers has to accept them back.
func TestGitDiffInAProjectInsideARepository(t *testing.T) {
	srv, _, proj := gitSubProject(t)
	writeFile(t, proj, "chapter2/second.qmd", "---\ntitle: Second\norder: 1\n---\n# Edited\n")
	const rel = "doc/Training/book/chapter2/second.qmd"

	body := get(t, srv, "/git").Body.String()
	if !strings.Contains(body, rel) {
		t.Fatalf("the change is not listed by its repository path:\n%s", body)
	}

	body = get(t, srv, "/git/diff?path="+url.QueryEscape(rel)).Body.String()
	if strings.Contains(body, `class="error"`) {
		t.Fatalf("the diff failed:\n%s", body)
	}
	if !strings.Contains(body, "# Edited") {
		t.Errorf("the change is missing from the diff:\n%s", body)
	}
	// The editor addresses pages by their path inside the project, which
	// is the repository path with the project's own part taken off.
	if !strings.Contains(body, `hx-get="/content?path=chapter2%2Fsecond.qmd"`) {
		t.Errorf("the editor is not offered the project's own path:\n%s", body)
	}
	// And staging, which has the same paths to deal with.
	body = post(t, srv, "/git/stage", url.Values{"path": {rel}}).Body.String()
	if strings.Contains(body, `class="error"`) {
		t.Errorf("staging failed:\n%s", body)
	}
	if !strings.Contains(body, "<legend>Staged") {
		t.Errorf("the file was not staged:\n%s", body)
	}
}

// A file elsewhere in the repository is part of the same working tree and
// is shown, but the editor has no path to it, so it is not offered.
func TestGitDiffOutsideTheProject(t *testing.T) {
	srv, repo, _ := gitSubProject(t)
	writeFile(t, repo, "README.md", "# The repository, edited\n")

	body := get(t, srv, "/git/diff?path=README.md").Body.String()
	if strings.Contains(body, `class="error"`) {
		t.Fatalf("the diff failed:\n%s", body)
	}
	if !strings.Contains(body, "edited") {
		t.Errorf("the change is missing from the diff:\n%s", body)
	}
	if strings.Contains(body, "git-diff-open") {
		t.Errorf("a file the editor cannot reach was offered to it:\n%s", body)
	}
}

// gitClone is gitProject with a remote: the project is pushed to a bare
// repository and tracks it, and a second clone of that repository stands
// for whoever else pushes there. It returns the server, the project, and
// the second clone.
func gitClone(t *testing.T) (*server, string, string) {
	t.Helper()
	srv, root := gitProject(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	other := filepath.Join(t.TempDir(), "other")
	for _, step := range []struct {
		dir  string
		args []string
	}{
		{root, []string{"init", "--bare", "-b", "main", remote}},
		{root, []string{"remote", "add", "origin", remote}},
		{root, []string{"push", "-u", "origin", "main"}},
		{root, []string{"clone", remote, other}},
		{other, []string{"config", "user.email", "folio@example.com"}},
		{other, []string{"config", "user.name", "QM Test"}},
	} {
		cmd := exec.Command("git", step.args...)
		cmd.Dir = step.dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", step.args, err, out)
		}
	}
	return srv, root, other
}

// The panel offers Sync, which brings in what the remote has and sends
// what it lacks, and says how much of each it moved.
func TestGitSync(t *testing.T) {
	srv, root, other := gitClone(t)
	if body := get(t, srv, "/git").Body.String(); !strings.Contains(body, `hx-post="/git/sync"`) {
		t.Errorf("no Sync button:\n%s", body)
	}

	writeFile(t, other, "theirs.qmd", "---\ntitle: Theirs\n---\n")
	for _, args := range [][]string{{"add", "--all"}, {"commit", "-m", "theirs"}, {"push"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = other
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	body := post(t, srv, "/git/sync", nil).Body.String()
	if !strings.Contains(body, "Pulled 1 commit") {
		t.Errorf("pull not reported:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(root, "theirs.qmd")); err != nil {
		t.Errorf("their page did not arrive: %v", err)
	}

	body = post(t, srv, "/git/sync", nil).Body.String()
	if !strings.Contains(body, "Already in sync") {
		t.Errorf("an idle sync is not reported as such:\n%s", body)
	}
}

// A branch without an upstream is told how to get one rather than shown a
// failure from git.
func TestGitSyncNeedsAnUpstream(t *testing.T) {
	srv, _ := gitProject(t)
	body := post(t, srv, "/git/sync", nil).Body.String()
	if !strings.Contains(body, "tracks no remote branch") {
		t.Errorf("missing upstream not reported:\n%s", body)
	}
}

// Push and Sync talk to a remote and can take a while. While one runs, the
// pane's action buttons are greyed out, so a second click cannot start a
// second push or sync, and a Refresh cannot swap the pane under it; the
// answer re-renders the buttons as the outcome leaves them.
func TestGitRemoteButtonsDisableWhileRunning(t *testing.T) {
	srv, _, _ := gitClone(t)
	body := get(t, srv, "/git").Body.String()
	for _, target := range []string{"/git/push", "/git/sync"} {
		want := `hx-post="` + target + `" hx-target="#git-body" hx-swap="innerHTML" hx-disabled-elt=".git-actions button"`
		if !strings.Contains(body, want) {
			t.Errorf("%s does not disable the buttons while it runs:\n%s", target, body)
		}
	}
}
