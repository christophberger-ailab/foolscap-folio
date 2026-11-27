//go:build !windows

package web

// osProxy is "" outside Windows: there, the proxy a system is set up for
// is in the environment, and git reads it from there.
func osProxy() string { return "" }
