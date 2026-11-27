package web

import "golang.org/x/sys/windows/registry"

// osProxy is the proxy set under Windows' Internet Options, which
// browsers use and Git for Windows does not.
func osProxy() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	if on, _, err := k.GetIntegerValue("ProxyEnable"); err != nil || on == 0 {
		return ""
	}
	p, _, _ := k.GetStringValue("ProxyServer")
	return proxyServer(p)
}
