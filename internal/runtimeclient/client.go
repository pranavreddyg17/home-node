// Package runtimeclient only speaks typed operations over fixed local sockets.
package runtimeclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type Client struct{ runtime, transfer *http.Client }
type GuestRequest struct {
	InstanceID string             `json:"instanceId"`
	Request    guestproto.Request `json:"request"`
}

func socketClient(socket string) *http.Client {
	return &http.Client{Timeout: 95 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", socket)
	}, MaxConnsPerHost: 8, MaxIdleConnsPerHost: 4, ResponseHeaderTimeout: 90 * time.Second}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("local service redirect denied") }}
}

// Authenticate the listener's kernel identity before sending operation bodies.
// Filesystem permissions alone cannot authenticate the process behind a socket.
func authenticatedSocketClient(socket string, listenerUID uint32) *http.Client {
	client := socketClient(socket)
	transport := client.Transport.(*http.Transport)
	transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		if !filepath.IsAbs(socket) || filepath.Clean(socket) != socket || listenerUID > 1<<31-1 {
			return nil, errors.New("invalid local workload peer")
		}
		connection, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", socket)
		if err != nil {
			return nil, err
		}
		unix, ok := connection.(*net.UnixConn)
		if !ok {
			connection.Close()
			return nil, errors.New("invalid local workload peer")
		}
		peer, err := supervisor.PeerUID(unix)
		if err != nil || peer != listenerUID {
			connection.Close()
			return nil, errors.New("invalid local workload peer")
		}
		return connection, nil
	}
	return client
}

func New(runtimeSocket, transferSocket string, transferUID uint32) *Client {
	// An absent transfer endpoint is used by the transfer service itself. Its
	// only remote authority is the root supervisor; no guest calls are enabled.
	if transferSocket != "" && transferUID == 0 {
		transferUID = ^uint32(0) // Invalid: never accept root as the transfer role.
	}
	return &Client{runtime: authenticatedSocketClient(runtimeSocket, 0), transfer: authenticatedSocketClient(transferSocket, transferUID)}
}
func call(ctx context.Context, client *http.Client, path string, request, response any) (resultErr error) {
	// Keep local socket/address errors private while retaining cancellation as
	// a typed cause for callers reconciling interrupted operations.
	defer func() {
		if resultErr != nil && ctx.Err() != nil {
			resultErr = errors.Join(resultErr, ctx.Err())
		}
	}()
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://local"+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	result, err := client.Do(req)
	if err != nil {
		return errors.New("local workload service unavailable")
	}
	defer result.Body.Close()
	if result.StatusCode != 200 {
		return errors.New("local workload service rejected operation")
	}
	decoder := json.NewDecoder(io.LimitReader(result.Body, guestproto.MaxFrame+1))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(response); err != nil {
		return err
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return guestproto.ErrProtocol
	}
	return nil
}
func (c *Client) Apply(ctx context.Context, request supervisor.Request) (supervisor.Instance, error) {
	var result supervisor.Instance
	err := call(ctx, c.runtime, "/v1/runtime", request, &result)
	return result, err
}
func (c *Client) Call(ctx context.Context, id string, request guestproto.Request) (guestproto.Response, error) {
	var result guestproto.Response
	if err := guestproto.Validate(request); err != nil {
		return result, err
	}
	err := call(ctx, c.transfer, "/v1/guest", GuestRequest{InstanceID: id, Request: request}, &result)
	if err != nil {
		return result, err
	}
	if result.Version != 1 || result.RequestID != request.RequestID {
		return result, guestproto.ErrProtocol
	}
	return result, nil
}
