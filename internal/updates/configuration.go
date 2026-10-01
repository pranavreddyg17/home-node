package updates

import (
	"context"
	"errors"
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
