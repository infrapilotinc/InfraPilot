package api

import "testing"

// host.docker.internal only exists as a static /etc/hosts entry, never a real DNS
// record -- nginx's resolver directive (needed for variable-based proxy_pass, so nginx
// doesn't crash if a *container* target is stopped) performs actual DNS queries and
// never consults /etc/hosts, so it always NXDOMAINs on this hostname. A literal
// proxy_pass is resolved once via the system resolver instead, which does read
// /etc/hosts.
func TestProxyPassDirectiveUsesLiteralForHostDockerInternal(t *testing.T) {
	got := proxyPassDirective("http://host.docker.internal:11500")
	want := "        proxy_pass http://host.docker.internal:11500;\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestProxyPassDirectiveUsesVariableForContainerNames(t *testing.T) {
	got := proxyPassDirective("http://myapp:3000")
	want := "        set $upstream \"http://myapp:3000\";\n        proxy_pass $upstream;\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
