package api

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/infrapilot/backend/internal/config"
)

func TestCertCoversDomain(t *testing.T) {
	cases := []struct {
		sans   []string
		domain string
		want   bool
	}{
		{[]string{"learnova.pk"}, "learnova.pk", true},
		{[]string{"learnova.pk"}, "app.learnova.pk", false},
		{[]string{"*.learnova.pk", "learnova.pk"}, "app.learnova.pk", true},
		{[]string{"*.learnova.pk"}, "a.b.learnova.pk", false},
		{[]string{"*.learnova.pk"}, "*.learnova.pk", true},
		{[]string{"learnova.pk"}, "*.learnova.pk", false},
		{[]string{"*.other.pk"}, "*.learnova.pk", false},
		{[]string{"*.Learnova.PK"}, "App.learnova.pk", true},
	}
	for _, tc := range cases {
		if got := certCoversDomain(tc.sans, tc.domain); got != tc.want {
			t.Errorf("certCoversDomain(%v, %q) = %v, want %v", tc.sans, tc.domain, got, tc.want)
		}
	}
}

func TestAutoCertCandidates(t *testing.T) {
	if got, want := autoCertCandidates("*.learnova.pk"), []string{"*.learnova.pk", "learnova.pk"}; !reflect.DeepEqual(got, want) {
		t.Errorf("wildcard candidates = %v, want %v", got, want)
	}
	if got, want := autoCertCandidates("app.learnova.pk"), []string{"app.learnova.pk", "*.learnova.pk", "learnova.pk"}; !reflect.DeepEqual(got, want) {
		t.Errorf("subdomain candidates = %v, want %v", got, want)
	}
	if got, want := autoCertCandidates("learnova.pk"), []string{"learnova.pk"}; !reflect.DeepEqual(got, want) {
		t.Errorf("apex candidates = %v, want %v", got, want)
	}
	if got, want := certDirName("*.learnova.pk"), "_wildcard.learnova.pk"; got != want {
		t.Errorf("certDirName = %q, want %q", got, want)
	}
}

func TestInternalServiceRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	run := func(token, method, path string, headers map[string]string) bool {
		h := &Handler{cfg: &config.Config{InternalServiceToken: token}}
		var got bool
		r := gin.New()
		r.Handle(method, "/api/v1/agents/:id/proxies", func(c *gin.Context) { got = h.isInternalServiceRequest(c) })
		r.Handle(method, "/api/v1/agents/:id", func(c *gin.Context) { got = h.isInternalServiceRequest(c) })
		req := httptest.NewRequest(method, path, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		r.ServeHTTP(httptest.NewRecorder(), req)
		return got
	}

	legacy := map[string]string{"X-Internal-Service": "agent"}
	good := map[string]string{"X-Internal-Token": "s3cret"}

	if run("", http.MethodGet, "/api/v1/agents/x/proxies", legacy) {
		t.Error("bare X-Internal-Service header must not authenticate")
	}
	if run("", http.MethodGet, "/api/v1/agents/x/proxies", map[string]string{"X-Internal-Token": ""}) {
		t.Error("empty configured token must disable internal auth")
	}
	if run("s3cret", http.MethodGet, "/api/v1/agents/x/proxies", map[string]string{"X-Internal-Token": "wrong"}) {
		t.Error("wrong token must not authenticate")
	}
	if run("s3cret", http.MethodPost, "/api/v1/agents/x/proxies", good) {
		t.Error("internal auth must be read-only")
	}
	if run("s3cret", http.MethodGet, "/api/v1/agents/x", good) {
		t.Error("internal auth must be limited to the proxy list route")
	}
	if !run("s3cret", http.MethodGet, "/api/v1/agents/x/proxies", good) {
		t.Error("valid token on the proxy list route should authenticate")
	}
}
