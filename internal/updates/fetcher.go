// Package updates implements the trusted product release acquisition path.
// Fetching bytes is not release authorization: TUF must verify all metadata
// and targets before the installer can act on them.
package updates

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/theupdateframework/go-tuf/v2/metadata"
)

const maxMetadataDownload int64 = 16 << 20

var errDownloadPolicy = errors.New("update download is outside repository policy")

// metadataFetcher is scoped to one update operation and one trusted metadata
// repository. It must not be reused for large package or image downloads.
type metadataFetcher struct {
	ctx    context.Context
	base   *url.URL
	client *http.Client
}

func newMetadataFetcher(ctx context.Context, repository string) (*metadataFetcher, error) {
	base, err := url.Parse(repository)
	if ctx == nil || err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil || base.RawQuery != "" || base.ForceQuery || base.Fragment != "" || base.RawPath != "" || strings.Contains(base.Path, "\\") || path.Clean(base.Path+"/") != strings.TrimSuffix(base.Path, "/") && base.Path != "" && base.Path != "/" {
		return nil, errDownloadPolicy
	}
	base.Path = strings.TrimSuffix(base.Path, "/") + "/"
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DisableCompression = true
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS13}
	transport.ResponseHeaderTimeout = 15 * time.Second
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errDownloadPolicy }}
	return &metadataFetcher{ctx: ctx, base: base, client: client}, nil
}

func (f *metadataFetcher) DownloadFile(address string, maximum int64, _ time.Duration) ([]byte, error) {
	if err := f.ctx.Err(); err != nil {
		return nil, err
	}
	location, err := url.Parse(address)
	if err != nil || maximum < 1 || maximum > maxMetadataDownload || location.Scheme != f.base.Scheme || location.Host != f.base.Host || location.User != nil || location.RawQuery != "" || location.ForceQuery || location.Fragment != "" || location.RawPath != "" || strings.Contains(location.Path, "\\") || path.Clean(location.Path) != location.Path || !strings.HasPrefix(location.Path, f.base.Path) {
		return nil, errDownloadPolicy
	}
	ctx, cancel := context.WithTimeout(f.ctx, 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, location.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", "HomeNode-update")
	response, err := f.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, &metadata.ErrDownloadHTTP{StatusCode: response.StatusCode, URL: address}
	}
	if response.ContentLength > maximum || response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity" {
		return nil, errDownloadPolicy
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maximum+1))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, errDownloadPolicy
	}
	return data, nil
}
