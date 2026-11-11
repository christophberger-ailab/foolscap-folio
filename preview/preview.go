// Package preview implements the `folio preview` command: a local web
// server that serves the project's website and renders it again, and
// reloads the browser, whenever a file of the project changes.
//
// Spec: spec-preview.yaml.
//
//	folio preview                    -> the first website on localhost:8198
//	folio preview ../handbook        -> that project instead
//	folio preview --audience pol     -> the pol variant of the site
//	folio preview --addr :9000       -> listen elsewhere (wins over preview: in folio.yml)
//
// The whole site is rendered up front and again after every change: every
// page's navigation, previous/next links and search index depend on all
// the others, so rendering one page on demand would leave the rest stale,
// and a full render is fast. A render goes into a fresh temporary
// directory and replaces the served one only when it is complete.
package preview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"html"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/christophberger-ailab/foolscap-folio/internal/build"
	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/web"
	"github.com/christophberger/start"
	flag "github.com/spf13/pflag"
)

// pollEvery is how often the project is checked for changes.
const pollEvery = 300 * time.Millisecond

// livePath is the server-sent event stream the pages listen to.
const livePath = "/_folio/live"

var projectPath *string

// Register wires the `preview` command into start. It shares --addr with
// `web` and --topic, --format and --audience with `render`, which register
// them; it must therefore be registered after both.
func Register(projectFlag *string) {
	projectPath = projectFlag
	start.Add(&start.Command{
		Name:  "preview",
		Short: "Serve the website and update it live as pages change",
		Long: "Render the project's website, serve it on localhost:8198 (or the " +
			"address in folio.yml's preview section, or the next free port up), " +
			"and render it again whenever a file of the project changes; open " +
			"pages reload themselves. Usage: folio preview [<path>]. --topic, " +
			"--format and --audience pick the site as for `folio render`; " +
			"--addr sets the listen address.",
		Flags: []string{"addr", "topic", "format", "audience"},
		Cmd:   cmd,
	})
}

func cmd(c *start.Command) error {
	root := *projectPath
	if len(c.Args) >= 1 && c.Args[0] != "" {
		root = c.Args[0]
	}
	docPath, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("cannot resolve project path: %w", err)
	}
	str := func(name string) string { v, _ := flag.CommandLine.GetString(name); return v }
	req := config.Request{
		Topics:    split(str("topic")),
		Formats:   split(str("format")),
		Audiences: split(str("audience")),
	}
	return Serve(str("addr"), docPath, req)
}

// listenTarget is the address to listen on and whether to search for a
// free port from it: flagAddr as given, or else the preview section of the
// project's folio.yml, searched from. A folio.yml that does not load leaves
// the default in place and is returned as the error, for the caller to warn
// about; the render reports what is wrong with the file.
func listenTarget(flagAddr, root string) (string, bool, error) {
	if flagAddr != "" {
		return flagAddr, false, nil
	}
	w := config.DefaultPreview
	cfg, err := config.Load(root)
	if err == nil {
		w = cfg.Preview
	}
	return net.JoinHostPort(w.BaseURL, strconv.Itoa(w.Port)), true, err
}

// Serve renders the site, serves it on addr (or the address in folio.yml
// and the next free port up), and keeps it up to date until interrupted.
func Serve(addr, root string, req config.Request) error {
	addr, search, err := listenTarget(addr, root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "folio preview: %v; listening on the default address\n", err)
	}
	ln, shown, err := web.Listen(addr, search)
	if err != nil {
		return fmt.Errorf("preview: %w", err)
	}
	s := New(root, req, func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	})
	defer s.Close()
	_ = s.Build() // a failure is logged and shown in the browser
	fmt.Fprintf(os.Stderr, "folio preview running on http://%s\n", shown)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go s.Watch(ctx, pollEvery)
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		s.shutdown()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("preview: %w", err)
	}
	return nil
}

// Server renders a project's site and serves the latest good render.
type Server struct {
	root string
	req  config.Request
	log  func(format string, args ...any)

	mu     sync.Mutex
	dir    string // the render being served; "" before the first good one
	gen    int    // number of the render being served
	err    error  // the last render's error, nil if it succeeded
	skip   map[string]bool
	subs   map[chan struct{}]bool
	closed bool
	others string // the sites not served, last reported
}

// New returns a server for the project at root; nothing is rendered until
// Build. log receives the progress lines.
func New(root string, req config.Request, log func(format string, args ...any)) *Server {
	return &Server{root: root, req: req, log: log, subs: map[chan struct{}]bool{},
		skip: map[string]bool{filepath.Join(root, "_output"): true}}
}

// Build renders the site afresh and, if that succeeds, serves it in place
// of the previous render. Either way the open pages are told.
func (s *Server) Build() error {
	began := time.Now()
	dir, sel, err := s.render()
	s.mu.Lock()
	old := ""
	if err == nil {
		old, s.dir = s.dir, dir
		s.gen++
		s.skip = map[string]bool{filepath.Join(s.root, "_build"): true}
		if out, e := sel.OutputDir(); e == nil {
			// The site's own directory and the root of all outputs.
			s.skip[out] = true
			s.skip[filepath.Join(s.root, filepath.FromSlash(sel.Config.Output))] = true
		}
	}
	s.err = err
	s.notify()
	s.mu.Unlock()
	if old != "" {
		os.RemoveAll(old)
	}
	if err != nil {
		s.log("preview: %v", err)
		return err
	}
	s.log("rendered %s in %v", sel, time.Since(began).Round(time.Millisecond))
	return nil
}

func (s *Server) render() (string, config.Selection, error) {
	cfg, err := config.Load(s.root)
	if err != nil {
		return "", config.Selection{}, err
	}
	sel, others, err := selectSite(cfg, s.req)
	if err != nil {
		return "", sel, err
	}
	if o := strings.Join(others, ", "); o != s.others {
		s.others = o
		if o != "" {
			s.log("serving %s; also available: %s", sel, o)
		}
	}
	dir, err := os.MkdirTemp("", "folio-preview-")
	if err != nil {
		return "", sel, err
	}
	err = build.Site(sel, dir, func(format string, args ...any) {
		if strings.Contains(format, "warning") {
			s.log(format, args...)
		}
	})
	if err != nil {
		os.RemoveAll(dir)
		return "", sel, err
	}
	return dir, sel, nil
}

// selectSite picks the website the request addresses: the first of the
// website selections, and the flags that would pick each of the others.
func selectSite(cfg *config.Config, req config.Request) (config.Selection, []string, error) {
	sels, err := cfg.Expand(req)
	if err != nil {
		return config.Selection{}, nil, err
	}
	var sites []config.Selection
	for _, sel := range sels {
		if sel.Format.IsWeb() {
			sites = append(sites, sel)
		}
	}
	if len(sites) == 0 {
		return config.Selection{}, nil, fmt.Errorf("the selection holds no website format")
	}
	var others []string
	for _, o := range sites[1:] {
		others = append(others, fmt.Sprintf("--format %s --audience %s", o.Format.Name, o.Audience.Name))
	}
	return sites[0], others, nil
}

// Watch renders the site again whenever the project changes, checking
// every interval until ctx is done.
func (s *Server) Watch(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	last := snapshot(s.root, s.skipped())
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if now := snapshot(s.root, s.skipped()); now != last {
			last = now
			_ = s.Build()
		}
	}
}

func (s *Server) skipped() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.skip
}

// snapshot is a hash over the path, size and modification time of every
// file below root, leaving out hidden entries, _build and the skipped
// directories.
func snapshot(root string, skip map[string]bool) uint64 {
	h := fnv.New64a()
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p != root && (strings.HasPrefix(d.Name(), ".") || skip[p] ||
			d.IsDir() && d.Name() == "_build" && filepath.Dir(p) == root) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\n", p, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return h.Sum64()
}

// notify tells every open page that the render changed. s.mu is held.
func (s *Server) notify() {
	for c := range s.subs {
		select {
		case c <- struct{}{}:
		default: // a notice is pending already
		}
	}
}

// shutdown ends the event streams, so the server can stop.
func (s *Server) shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for c := range s.subs {
		close(c)
		delete(s.subs, c)
	}
}

// Close ends the event streams and removes the render being served.
func (s *Server) Close() error {
	s.shutdown()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == "" {
		return nil
	}
	return os.RemoveAll(s.dir)
}

func (s *Server) state() (dir string, gen int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dir, s.gen, s.err
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == livePath {
		s.live(w, r)
		return
	}
	dir, gen, renderErr := s.state()
	if dir == "" {
		msg := "the site has not been rendered yet"
		if renderErr != nil {
			msg = renderErr.Error()
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "<!doctype html><html><body><h1>folio preview</h1><pre>%s</pre>%s</body></html>",
			html.EscapeString(msg), liveScript(gen))
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" || strings.HasSuffix(r.URL.Path, "/") {
		name = path.Join(name, "index.html")
	}
	if !fs.ValidPath(name) {
		http.NotFound(w, r)
		return
	}
	site := os.DirFS(dir)
	if !strings.HasSuffix(name, ".html") {
		if fi, err := fs.Stat(site, name); err == nil && fi.IsDir() {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusFound)
			return
		}
		http.ServeFileFS(w, r, site, name)
		return
	}
	page, err := fs.ReadFile(site, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(inject(page, liveScript(gen)))
}

// inject puts script before the page's closing body tag, or at its end.
func inject(page []byte, script string) []byte {
	s := string(page)
	if i := strings.LastIndex(s, "</body>"); i >= 0 {
		return []byte(s[:i] + script + s[i:])
	}
	return []byte(s + script)
}

// liveScript reloads the page when the render it came from (gen) is no
// longer the current one, and shows a failed render as a banner.
func liveScript(gen int) string {
	return `<script data-gen="` + strconv.Itoa(gen) + `">(() => {
  const gen = "` + strconv.Itoa(gen) + `";
  const es = new EventSource("` + livePath + `");
  es.addEventListener("render", e => { if (e.data !== gen) location.reload(); });
  es.addEventListener("failed", e => {
    let b = document.getElementById("folio-preview-error");
    if (!b) {
      b = document.createElement("pre");
      b.id = "folio-preview-error";
      b.style.cssText = "position:fixed;left:0;right:0;bottom:0;z-index:99999;margin:0;padding:1em;" +
        "max-height:40vh;overflow:auto;white-space:pre-wrap;background:#b00020;color:#fff;font:13px/1.4 monospace";
      document.body.appendChild(b);
    }
    b.textContent = "folio preview: render failed\n" + JSON.parse(e.data);
  });
})();</script>`
}

// live is the event stream a page listens to: the current render's number
// at once and after every good render, and the error of a failed one.
func (s *Server) live(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	c := make(chan struct{}, 1)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	}
	s.subs[c] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subs, c)
		s.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	send := func() {
		_, gen, err := s.state()
		if err != nil {
			msg, _ := json.Marshal(err.Error())
			fmt.Fprintf(w, "event: failed\ndata: %s\n\n", msg)
		} else {
			fmt.Fprintf(w, "event: render\ndata: %d\n\n", gen)
		}
		fl.Flush()
	}
	send()
	for {
		select {
		case <-r.Context().Done():
			return
		case _, open := <-c:
			if !open {
				return
			}
			send()
		}
	}
}

func split(arg string) []string {
	var out []string
	for _, p := range strings.Split(arg, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
