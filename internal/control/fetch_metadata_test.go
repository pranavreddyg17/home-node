package control

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFirstPartyFetchMetadataRejectsSiblingOriginsAndAmbiguousHeaders(t *testing.T) {
	s := testServer(t)
	token := seedSession(t, s, `["files"]`)
	for _, test := range []struct {
		sites, origins []string
		allowed        bool
	}{
		{nil, nil, true}, {[]string{"same-origin"}, nil, true}, {[]string{"none"}, nil, true},
		{[]string{"same-origin"}, []string{s.config.Origin}, true},
		{[]string{"same-site"}, nil, false}, {[]string{"cross-site"}, nil, false},
		{[]string{"unknown"}, nil, false}, {[]string{""}, nil, false},
		{[]string{"same-origin", "cross-site"}, nil, false},
		{[]string{"same-origin"}, []string{s.config.Origin, "https://foreign.example"}, false},
		{[]string{"same-origin"}, []string{s.config.Origin, s.config.Origin}, false},
	} {
		r := httptest.NewRequest("GET", "/api/v1/files/export", nil)
		r.Host = "localhost:8787"
		r.AddCookie(&http.Cookie{Name: s.cookie, Value: token})
		for _, v := range test.sites {
			r.Header.Add("Sec-Fetch-Site", v)
		}
		for _, v := range test.origins {
			r.Header.Add("Origin", v)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if test.allowed && (w.Code != 200 || w.Header().Get("Content-Type") != "application/zip") || !test.allowed && w.Code != 403 {
			t.Fatal("first-party boundary mismatch", test, w.Code, w.Body.String())
		}
		if !test.allowed && w.Header().Get("Content-Disposition") != "" {
			t.Fatal("refused request reached archive dispatch")
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("access decision could be cached")
		}
	}
}
