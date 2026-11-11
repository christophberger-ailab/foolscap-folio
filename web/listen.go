package web

import "net"

// Listen opens addr as folio web does: with search, on the first free port
// from addr's up. It returns the listener and the address to show the user.
func Listen(addr string, search bool) (net.Listener, string, error) {
	ln, err := listen(addr, search)
	if err != nil {
		return nil, "", err
	}
	return ln, servedAddr(addr, ln.Addr()), nil
}
