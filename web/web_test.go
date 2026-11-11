package web

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// A taken port is skipped when searching, and is an error when the address
// was asked for by name.
func TestListenCountsUpPastATakenPort(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	addr := taken.Addr().String()
	port := taken.Addr().(*net.TCPAddr).Port

	if ln, err := listen(addr, false); err == nil {
		ln.Close()
		t.Fatalf("listen(%s, false) succeeded on a taken port", addr)
	}

	ln, err := listen(addr, true)
	if err != nil {
		t.Fatalf("listen(%s, true): %v", addr, err)
	}
	defer ln.Close()
	got := ln.Addr().(*net.TCPAddr).Port
	if got <= port || got >= port+maxPortTries {
		t.Errorf("got port %d, want one above %d", got, port)
	}
	if _, p, _ := net.SplitHostPort(ln.Addr().String()); p != strconv.Itoa(got) {
		t.Errorf("address %s does not name port %d", ln.Addr(), got)
	}
}

// The project's folio.yml names where the UI listens; --addr, when it is
// given, wins over it and is used as given, while the address from the
// project is a starting point the port search counts up from.
func TestTheListenAddressComesFromTheFlagOrTheProject(t *testing.T) {
	withYML := func(yml string) string {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "folio.yml"), []byte(yml), 0o644); err != nil {
			t.Fatal(err)
		}
		return root
	}
	configured := withYML("web: {baseurl: 0.0.0.0, port: 8200}\n")
	cases := []struct {
		name, flag, root string
		want             string
		search, warn     bool
	}{
		{"no project", "", "", "localhost:8199", true, false},
		{"a project without a web section", "", withYML("title: T\n"), "localhost:8199", true, false},
		{"the project", "", configured, "0.0.0.0:8200", true, false},
		{"the flag over the project", ":9000", configured, ":9000", false, false},
		{"a broken folio.yml", "", withYML("web: {port: 70000}\n"), "localhost:8199", true, true},
	}
	for _, c := range cases {
		addr, search, err := listenTarget(c.flag, c.root)
		if addr != c.want || search != c.search || (err != nil) != c.warn {
			t.Errorf("%s: listenTarget = %q, %v, %v; want %q, %v, warning %v", c.name, addr, search, err, c.want, c.search, c.warn)
		}
	}
}
