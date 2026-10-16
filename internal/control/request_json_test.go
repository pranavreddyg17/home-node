package control

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

func FuzzRequestObject(f *testing.F) {
	for _, seed := range []string{`{}`, `{"title":"fixture"}`, `{"x":1,"x":2}`, `{"items":[null,true,3,"text"]}`, `null`, "{\"x\":\"\xff\"}"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 512<<10 {
			return
		}
		if !unambiguousRequestObject(body) {
			return
		}
		if !utf8.Valid(body) || !json.Valid(body) {
			t.Fatal("accepted invalid JSON or UTF-8")
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(body, &object); err != nil || object == nil {
			t.Fatal("accepted a non-object request")
		}
	})
}

func TestAmbiguousConversationRequestsHaveNoSideEffects(t *testing.T) {
	s := testServer(t)
	token := seedSession(t, s, `["ai"]`)
	for _, body := range []string{
		`{"title":"first","title":"second"}`,
		`{"title":"first","tit\u006ce":"second"}`,
		`{"title":"first","TITLE":"second"}`,
		`null`, `[]`, `{"title":"first"} {"title":"second"}`,
		"{\"title\":\"invalid\xff\"}",
	} {
		response := request(s, "POST", "http://localhost:8787/api/v1/ai/conversations", body, token, "http://localhost:8787")
		if response.Code != http.StatusBadRequest {
			t.Fatal("ambiguous request accepted", response.Code)
		}
	}
	var count int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM conversations").Scan(&count); err != nil || count != 0 {
		t.Fatal("refused request created a conversation", count, err)
	}
	if response := request(s, "POST", "http://localhost:8787/api/v1/ai/conversations", `{"title":"A normal fixture"}`, token, "http://localhost:8787"); response.Code != http.StatusCreated {
		t.Fatal("unambiguous request refused", response.Code, response.Body.String())
	}
}

func TestRequestObjectRejectsNestedAndUnicodeAliasesWithinBounds(t *testing.T) {
	for _, body := range []string{
		`{"nested":{"size":1,"\u017fize":2}}`,
		`{"nested":[{"key":1,"\u212aey":2}]}`,
		`{"nested":[{"same":1,"same":2}]}`,
		`{"nested":` + strings.Repeat("[", 40) + `0` + strings.Repeat("]", 40) + `}`,
	} {
		if unambiguousRequestObject([]byte(body)) {
			t.Fatal("ambiguous or excessive nesting accepted")
		}
	}
	if !unambiguousRequestObject([]byte(`{"items":[{"size":1},{"size":2}],"text":"duplicate-looking title title"}`)) {
		t.Fatal("separate objects or string content mistaken for duplicate fields")
	}
}
