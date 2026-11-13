// Package web implements the `qm web` command: a local web app for
// reordering the pages of a qm project by drag and drop, editing them,
// and rendering its book folders.
//
// Spec: spec-web.yaml.
//
//	qm web                 -> serve the project at --project on the default port
//	qm web <path>          -> serve the project at <path>
//	qm web --addr :9000    -> listen elsewhere (wins over web: in folio.yml)
//
// The package is the quarto-sorter tool
// (https://github.com/christophberger-ailab/quarto-sorter) merged into qm;
// its render panel drives the same internal/build flow that
// `qm render` runs from the command line.
package web

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/selfupdate"
	"github.com/christophberger/start"
	flag "github.com/spf13/pflag"
)

// listenTarget is the address to listen on and whether to search for a
// free port from it. An address the user named with --addr is the one
// they want, and serving elsewhere would be a surprise. Without it, the
// web section of the project's folio.yml names where to start, as the
// default does, and is searched from. A folio.yml that does not load
// leaves the default in place and is returned as the error, for the
// caller to warn about: the app reports what is wrong with the file once
// it runs, and refusing to start would leave nowhere to see that.
func listenTarget(flagAddr, root string) (string, bool, error) {
	if flagAddr != "" {
		return flagAddr, false, nil
	}
	w, err := config.DefaultWeb, error(nil)
	if root != "" {
		var cfg *config.Config
		if cfg, err = config.Load(root); err == nil {
			w = cfg.Web
		}
	}
	return net.JoinHostPort(w.BaseURL, strconv.Itoa(w.Port)), true, err
}

// maxPortTries bounds the search for a free port, so a machine that has
// every port above the default taken gets an error rather than a scan.
const maxPortTries = 100

var (
	projectPath *string
	addr        *string
	// version is the running release, which the Update button compares
	// new releases against.
	version string
)

// Register wires the top-level `web` command into start. Like `lint` and
// `render`, `web` carries no sub-commands and is registered as a leaf
// (SUBCOMMANDS.3 does not apply when there is nothing to dispatch to).
func Register(projectFlag *string, release string) {
	projectPath = projectFlag
	version = release
	addr = flag.String("addr", "",
		"Address to listen on, as host:port; without it, the baseurl and the first free port from port up in the project's folio.yml, under web: for folio web (localhost, 8199) and preview: for folio preview (localhost, 8198)")

	start.Add(&start.Command{
		Name:  "web",
		Short: "Serve the web UI for sorting, editing, and rendering pages",
		Long: "Serve a local web app that shows the project's page tree, " +
			"reorders pages by drag and drop, edits their content, and " +
			"renders the project's book folders. Usage: folio web [<path>]. " +
			"Without a path, the tree at --project is opened.",
		Flags: []string{"addr"},
		Cmd:   cmd,
	})
}

func cmd(c *start.Command) error {
	if projectPath == nil {
		return fmt.Errorf("web: --project flag not initialised")
	}
	// A positional path wins over --project so that `qm web ../book` reads
	// as it looks; both end up as the initially opened project.
	root := *projectPath
	if len(c.Args) >= 1 && c.Args[0] != "" {
		root = c.Args[0]
	}
	docPath, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("cannot resolve project path: %w", err)
	}
	return Serve(*addr, docPath)
}

// Serve runs the web UI with the project at root open. It listens on
// flagAddr if that is not empty, or else at the web address in the project's
// folio.yml, and
// then, when that port is in use, on the next free one up. It blocks until
// the server stops.
func Serve(flagAddr string, root string) error {
	restartAddr, restartRoot := takeRestartEnv()
	if restartRoot != "" {
		root = restartRoot
	}
	srv, err := newServer(defaultConfigFile())
	if err != nil {
		return fmt.Errorf("web: %w", err)
	}
	if root != "" {
		if err := srv.setRoot(root); err != nil {
			return fmt.Errorf("web: cannot open %s: %w", root, err)
		}
	}
	srv.startUpdateChecks(version)
	go srv.showChangesSince(context.Background(), takeUpdatedFrom())
	listenAddr, searchPort, err := listenTarget(flagAddr, root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "folio web: %v; listening on the default address\n", err)
	}
	if restartAddr != "" {
		listenAddr, searchPort = restartAddr, true
	}
	ln, err := listen(listenAddr, searchPort)
	if err != nil {
		return fmt.Errorf("web: %w", err)
	}
	served := servedAddr(listenAddr, ln.Addr())
	fmt.Fprintf(os.Stderr, "folio web running on http://%s\n", served)

	srv.restart = make(chan struct{}, 1)
	errc := make(chan error, 1)
	go func() { errc <- http.Serve(ln, srv) }()
	select {
	case err := <-errc:
		return fmt.Errorf("web: %w", err)
	case <-srv.restart:
		return restart(srv, ln, served)
	}
}

// restart hands the address and the open project to the updated binary
// and runs it in place of this process. It returns only when that fails,
// and then the user has to start folio web again.
func restart(srv *server, ln net.Listener, addr string) error {
	ln.Close()
	srv.mu.Lock()
	root := srv.root
	srv.mu.Unlock()
	srv.update.mu.Lock()
	exe, next, from := srv.update.exe, srv.update.installed, srv.update.current
	srv.update.mu.Unlock()
	os.Setenv(restartFromEnv, from)
	os.Setenv(restartAddrEnv, addr)
	os.Setenv(restartRootEnv, root)
	fmt.Fprintf(os.Stderr, "folio web: restarting with %s\n", next)
	err := selfupdate.Restart(exe)
	return fmt.Errorf("web: updated to %s, but could not restart (%v); start folio web again to use it", next, err)
}

// listen opens addr, or with searchPort the first port from addr's up that
// is not in use. Any other failure -- a host that does not resolve, a port
// the user may not bind -- is returned at once: counting up would not fix
// it.
func listen(addr string, searchPort bool) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err == nil || !searchPort || !addrInUse(err) {
		return ln, err
	}
	host, portStr, splitErr := net.SplitHostPort(addr)
	port, atoiErr := strconv.Atoi(portStr)
	if splitErr != nil || atoiErr != nil || port == 0 {
		return nil, err
	}
	first := err
	for i := 1; i < maxPortTries && port+i <= 65535; i++ {
		ln, err = net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port+i)))
		if err == nil {
			return ln, nil
		}
		if !addrInUse(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("no free port among the %d from %d: %w", maxPortTries, port, first)
}

// wsaeAddrInUse is Windows' "address already in use", which its sockets
// report instead of the EADDRINUSE the syscall package names.
const wsaeAddrInUse = 10048

// addrInUse reports whether a listen failed because the port is taken.
func addrInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == wsaeAddrInUse
}

// servedAddr is the address to tell the user: the host as they gave it --
// "localhost" reads better than the 127.0.0.1 it resolved to -- with the
// port actually listened on, which a search may have moved.
func servedAddr(requested string, got net.Addr) string {
	host, _, err := net.SplitHostPort(requested)
	tcp, ok := got.(*net.TCPAddr)
	if err != nil || host == "" || !ok {
		return got.String()
	}
	return net.JoinHostPort(host, strconv.Itoa(tcp.Port))
}
