package api

import (
	"strings"
	"testing"
)

func TestValidateCustomNginxConfig(t *testing.T) {
	ok := []string{
		"",
		"client_max_body_size 12m;",
		"proxy_read_timeout 300s;\nproxy_send_timeout 300s;",
		"location /uploads/ {\n    client_max_body_size 50m;\n}",
		`add_header X-Note "a } inside quotes";`,
		"# a comment with { brace\nclient_max_body_size 1m;",
		`return 200 'it\'s fine';`,
	}
	for _, cfg := range ok {
		if err := validateCustomNginxConfig(cfg); err != nil {
			t.Errorf("expected %q to be valid, got %v", cfg, err)
		}
	}

	bad := map[string]string{
		"closes the server block": "client_max_body_size 1m;\n}\nserver {\n listen 8080;",
		"unclosed location":       "location / {\n  proxy_pass http://x;",
		"unterminated quote":      `add_header X "oops;`,
		"too long":                strings.Repeat("a", maxCustomNginxConfigLen+1),
	}
	for name, cfg := range bad {
		if err := validateCustomNginxConfig(cfg); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestGenerateNginxConfigCustomPlacement(t *testing.T) {
	custom := "client_max_body_size 12m;"

	// SSL + force SSL: only the HTTPS block serves traffic, the HTTP block just redirects.
	proxy := ProxyHost{Domain: "rider.example.com", UpstreamTarget: "app:3000", SSLEnabled: true, ForceSSL: true, CustomConfig: custom}
	cfg := generateNginxConfig(proxy, SecurityHeaders{}, nil)
	if n := strings.Count(cfg, "    "+custom); n != 1 {
		t.Fatalf("force-SSL: expected custom config once (HTTPS block), got %d\n%s", n, cfg)
	}
	if strings.Index(cfg, custom) < strings.Index(cfg, "listen 443") {
		t.Fatalf("force-SSL: custom config should be in the HTTPS block\n%s", cfg)
	}

	// SSL without force: both blocks serve traffic.
	proxy.ForceSSL = false
	if n := strings.Count(generateNginxConfig(proxy, SecurityHeaders{}, nil), "    "+custom); n != 2 {
		t.Fatalf("SSL without force: expected custom config in both blocks, got %d", n)
	}

	// Plain HTTP.
	proxy.SSLEnabled = false
	if n := strings.Count(generateNginxConfig(proxy, SecurityHeaders{}, nil), "    "+custom); n != 1 {
		t.Fatalf("HTTP only: expected custom config once, got %d", n)
	}

	// Empty custom config adds nothing.
	proxy.CustomConfig = ""
	if strings.Contains(generateNginxConfig(proxy, SecurityHeaders{}, nil), "# Custom configuration") {
		t.Fatal("empty custom config should not add a section")
	}
}
