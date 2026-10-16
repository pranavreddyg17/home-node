package control

import (
	"context"
	"net/http"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestChatHistoryAndExportDistinguishMissingFromEmptyConversation(t *testing.T) {
	s := testServer(t)
	token := seedSession(t, s, `["ai"]`)
	conversation, err := s.Workloads.CreateConversationWithKey(context.Background(), "device", state.Random(), "Empty fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"history", "export"} {
		url := "http://localhost:8787/api/v1/ai/conversations/" + conversation.ID + "/" + suffix
		if response := request(s, "GET", url, "", token, ""); response.Code != http.StatusOK {
			t.Fatal("empty existing chat unavailable without guest", suffix, response.Code, response.Body.String())
		}
	}
	if _, err = s.Store.DB.Exec("DELETE FROM conversations WHERE id=?", conversation.ID); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"history", "export"} {
		url := "http://localhost:8787/api/v1/ai/conversations/" + conversation.ID + "/" + suffix
		if response := request(s, "GET", url, "", token, ""); response.Code != http.StatusNotFound {
			t.Fatal("missing chat presented as empty", suffix, response.Code, response.Body.String())
		}
		if response := request(s, "GET", url, "", "", ""); response.Code != http.StatusUnauthorized {
			t.Fatal("missing chat metadata exposed without authentication", response.Code)
		}
		url = "http://localhost:8787/api/v1/ai/conversations/invalid/" + suffix
		if response := request(s, "GET", url, "", token, ""); response.Code != http.StatusBadRequest {
			t.Fatal("malformed chat ID accepted", suffix, response.Code)
		}
	}
}
