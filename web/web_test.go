package web

import (
	"net"
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

// The settings' web section names where the UI listens; --addr, when it
// is given, wins over it and is used as given, while the address from the
// settings is a starting point the port search counts up from.
func TestTheListenAddressComesFromTheFlagOrTheSettings(t *testing.T) {
	cases := []struct {
		name, flag string
		web        webConfig
		want       string
		search     bool
	}{
		{"the defaults", "", defaultConfig().Web, "localhost:8199", true},
		{"the settings", "", webConfig{BaseURL: "0.0.0.0", Port: 8200}, "0.0.0.0:8200", true},
		{"the flag over the settings", ":9000", webConfig{BaseURL: "0.0.0.0", Port: 8200}, ":9000", false},
	}
	for _, c := range cases {
		addr, search := listenTarget(c.flag, c.web)
		if addr != c.want || search != c.search {
			t.Errorf("%s: listenTarget = %q, %v; want %q, %v", c.name, addr, search, c.want, c.search)
		}
	}
}
