package transfer

import (
	"io"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

const maxAbandonedResponses = 2

// A cancelled virtio read can leave a complete reply queued across socket
// reconnection. Discard only nonce IDs this instance's host previously sent
// without receiving an acknowledged response. Never replay the guest request,
// accept an unknown nonce, relax frame limits, or accept a mismatched reply.
// The caller retains the original I/O deadline and per-instance exclusion.
func readGuestResponse(reader io.Reader, expected string, abandoned []string) (response guestproto.Response, remaining []string, discarded int, err error) {
	if !guestproto.ValidID(expected) || len(abandoned) > maxAbandonedResponses {
		return response, nil, 0, guestproto.ErrProtocol
	}
	for _, id := range abandoned {
		if !guestproto.ValidID(id) {
			return response, nil, 0, guestproto.ErrProtocol
		}
		if id == expected {
			continue
		}
		for _, previous := range remaining {
			if previous == id {
				return response, nil, 0, guestproto.ErrProtocol
			}
		}
		remaining = append(remaining, id)
	}
	for {
		response = guestproto.Response{}
		if err = guestproto.Read(reader, &response); err != nil {
			return guestproto.Response{}, remaining, discarded, err
		}
		if response.Version != 1 || !guestproto.ValidID(response.RequestID) || len(response.Data) > guestproto.ChunkSize || len(response.Text) > 32<<10 || len(response.Error) > 128 || response.Offset < 0 || response.Size < 0 || response.Size > 512<<30 {
			return guestproto.Response{}, remaining, discarded, guestproto.ErrProtocol
		}
		if response.RequestID == expected {
			return response, remaining, discarded, nil
		}
		found := -1
		for index, id := range remaining {
			if response.RequestID == id {
				found = index
				break
			}
		}
		if found < 0 || discarded == maxAbandonedResponses {
			return guestproto.Response{}, remaining, discarded, guestproto.ErrProtocol
		}
		remaining = append(remaining[:found], remaining[found+1:]...)
		discarded++
	}
}
