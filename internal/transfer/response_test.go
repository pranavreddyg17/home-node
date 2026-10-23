package transfer

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestGuestResponseRequiresCurrentNonceAfterBoundedKnownLateReplies(t *testing.T) {
	expected, stale, stale2, foreign := state.Random(), state.Random(), state.Random(), state.Random()
	for _, scenario := range []struct {
		name      string
		ids       []string
		pending   []string
		ok        bool
		discarded int
	}{
		{"current", []string{expected}, nil, true, 0},
		{"known-cancelled", []string{stale, expected}, []string{stale}, true, 1},
		{"two-known", []string{stale2, stale, expected}, []string{stale, stale2}, true, 2},
		{"unknown", []string{foreign, expected}, []string{stale}, false, 0},
		{"duplicate-late", []string{stale, stale, expected}, []string{stale}, false, 1},
		{"untracked-old", []string{stale, expected}, nil, false, 0},
		{"too-many-tracked", []string{expected}, []string{stale, stale2, foreign}, false, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var input bytes.Buffer
			for _, id := range scenario.ids {
				if err := guestproto.Write(&input, guestproto.Response{Version: 1, RequestID: id, State: "ready"}); err != nil {
					t.Fatal(err)
				}
			}
			response, _, discarded, err := readGuestResponse(&input, expected, scenario.pending)
			if scenario.ok {
				if err != nil || response.RequestID != expected || response.State != "ready" {
					t.Fatal("current response unavailable", response, err)
				}
			} else if !errors.Is(err, guestproto.ErrProtocol) || response.RequestID != "" {
				t.Fatal("foreign response admitted", response, err)
			}
			if discarded != scenario.discarded {
				t.Fatal("discard bound", discarded)
			}
		})
	}
}

func TestLateReplyStillRequiresFrameAndResponseBounds(t *testing.T) {
	expected, stale := state.Random(), state.Random()
	for _, response := range []guestproto.Response{
		{Version: 2, RequestID: stale},
		{Version: 1, RequestID: stale, Offset: -1},
		{Version: 1, RequestID: stale, Size: 512<<30 + 1},
		{Version: 1, RequestID: stale, Text: string(bytes.Repeat([]byte("x"), 32<<10+1))},
		{Version: 1, RequestID: stale, Data: bytes.Repeat([]byte("x"), guestproto.ChunkSize+1)},
	} {
		var input bytes.Buffer
		if err := guestproto.Write(&input, response); err != nil {
			t.Fatal(err)
		}
		if err := guestproto.Write(&input, guestproto.Response{Version: 1, RequestID: expected}); err != nil {
			t.Fatal(err)
		}
		if response, _, _, err := readGuestResponse(&input, expected, []string{stale}); !errors.Is(err, guestproto.ErrProtocol) || response.RequestID != "" {
			t.Fatal("oversized late response admitted", err)
		}
	}
	var input bytes.Buffer
	if err := guestproto.Write(&input, guestproto.Response{Version: 1, RequestID: stale}); err != nil {
		t.Fatal(err)
	}
	if response, _, count, err := readGuestResponse(&input, expected, []string{stale}); !errors.Is(err, io.EOF) || count != 1 || response.RequestID != "" {
		t.Fatal("late response substituted for missing current response", count, err)
	}
}
