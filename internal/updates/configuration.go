package updates

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
)

// RepositoryConfiguration is immutable installer-owned acquisition policy.
// Current database schema is measured at operation time rather than trusted
// from an old installation file. These floors never authorize installation.
type RepositoryConfiguration struct {
	Schema                int    `json:"schema"`
	MetadataURL           string `json:"metadataUrl"`
	TargetsURL            string `json:"targetsUrl"`
	MinimumSequence       int64  `json:"minimumSequence"`
	MinimumCatalogVersion int64  `json:"minimumCatalogVersion"`
}

func (configuration RepositoryConfiguration) Validate() error {
	if configuration.Schema != 1 || configuration.MinimumSequence < 1 || configuration.MinimumCatalogVersion < 1 || len(configuration.MetadataURL) > 2048 || len(configuration.TargetsURL) > 2048 {
		return errReleasePolicy
	}
	metadata, err := newMetadataFetcher(context.Background(), configuration.MetadataURL)
	if err != nil {
		return err
	}
	defer metadata.client.CloseIdleConnections()
	targets, err := newMetadataFetcher(context.Background(), configuration.TargetsURL)
	if err != nil {
		return err
	}
	defer targets.client.CloseIdleConnections()
	if metadata.base.Host != targets.base.Host || metadata.base.Path == targets.base.Path {
		return errDownloadPolicy
	}
	return nil
}

func (configuration RepositoryConfiguration) Policy(currentSchema int) (ReleasePolicy, error) {
	if err := configuration.Validate(); err != nil {
		return ReleasePolicy{}, err
	}
	if currentSchema < 1 || currentSchema > 1024 {
		return ReleasePolicy{}, errors.New("invalid observed state schema")
	}
	return ReleasePolicy{MinimumSequence: configuration.MinimumSequence, MinimumCatalogVersion: configuration.MinimumCatalogVersion, CurrentStateSchema: currentSchema}, nil
}

// ParseRepositoryConfiguration validates bounded, unambiguous configuration
// bytes. It does not establish file ownership: the privileged caller must read
// these bytes from its verified installer journal entry.
func ParseRepositoryConfiguration(data []byte) (RepositoryConfiguration, error) {
	var result RepositoryConfiguration
	if len(data) == 0 || len(data) > 8192 {
		return result, errReleasePolicy
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return result, errReleasePolicy
	}
	allowed := map[string]bool{"schema": true, "metadataUrl": true, "targetsUrl": true, "minimumSequence": true, "minimumCatalogVersion": true}
	seen := map[string]bool{}
	for decoder.More() {
		token, err = decoder.Token()
		name, ok := token.(string)
		if err != nil || !ok || !allowed[name] || seen[name] {
			return result, errReleasePolicy
		}
		seen[name] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return result, errReleasePolicy
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') || len(seen) != len(allowed) {
		return result, errReleasePolicy
	}
	if decoder.Decode(new(any)) != io.EOF {
		return result, errReleasePolicy
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil {
		return RepositoryConfiguration{}, errReleasePolicy
	}
	if err := result.Validate(); err != nil {
		return RepositoryConfiguration{}, err
	}
	return result, nil
}
