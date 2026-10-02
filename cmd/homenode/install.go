package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/hostcheck"
	"github.com/pranavreddyg17/home-node/internal/install"
	"github.com/pranavreddyg17/home-node/internal/networkcheck"
	"github.com/pranavreddyg17/home-node/internal/updates"
)

type preparation struct {
	configuration   install.Configuration
	source, journal string
}

func parsePreparation(args []string, now time.Time) (preparation, error) {
	var p preparation
	flags := flag.NewFlagSet("install-prepare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	key := flags.String("publisher-key", "", "independently verified publisher Ed25519 public key, hex")
	provenanceFile := flags.String("update-provenance", "", "independently reviewed provenance trust policy file")
	provenancePin := flags.String("update-provenance-sha256", "", "independently verified provenance policy SHA256")
	updateRoot := flags.String("update-root", "", "independently trusted TUF bootstrap root file")
	updatePin := flags.String("update-root-sha256", "", "independently verified bootstrap root SHA256; required with update-root")
	updateMetadata := flags.String("update-metadata-url", "", "trusted HTTPS TUF metadata repository")
	updateTargets := flags.String("update-targets-url", "", "trusted HTTPS release target repository on the same host")
	updateSequence := flags.Int64("update-sequence-floor", 0, "independently verified minimum release security sequence")
	flags.StringVar(&p.configuration.BackupRepositoryID, "backup-repository-id", "", "trusted registered backup repository identity; requires provisioned maintenance account")
	file := flags.String("catalog", "", "signed release catalog file")
	flags.Int64Var(&p.configuration.MinimumCatalogVersion, "catalog-floor", 0, "independently verified minimum catalog version")
	flags.StringVar(&p.source, "images", "", "local release image directory")
	flags.StringVar(&p.journal, "journal-dir", "/var/lib/homenode-install", "private installation journal")
	flags.StringVar(&p.configuration.Network.Bind, "bind", "", "local Tailscale IPv4 address")
	flags.StringVar(&p.configuration.Network.Origin, "origin", "", "canonical private HTTPS origin")
	flags.IntVar(&p.configuration.Network.Port, "port", 8787, "HTTPS port")
	flags.Int64Var(&p.configuration.Policy.Generation, "generation", 1, "runtime policy generation")
	flags.IntVar(&p.configuration.Policy.MemoryMiB, "memory-mib", 4096, "workload memory budget")
	flags.IntVar(&p.configuration.Policy.VCPUs, "cpus", 2, "workload CPU budget")
	flags.IntVar(&p.configuration.Policy.MaxInstances, "instances", 3, "maximum simultaneous instances")
	flags.Int64Var(&p.configuration.Policy.DiskReserveBytes, "disk-reserve", 4*catalog.GiB, "minimum free bytes to retain")
	if err := flags.Parse(args); err != nil {
		return p, err
	}
	if flags.NArg() != 0 || p.source == "" || p.journal == "" || *file == "" || p.configuration.MinimumCatalogVersion < 1 {
		return p, install.ErrPlan
	}
	if (*updateRoot == "") != (*updatePin == "") {
		return p, install.ErrPlan
	}
	if *updateRoot != "" {
		data, err := readReleaseCatalog(*updateRoot)
		if err != nil {
			return p, err
		}
		bootstrap := &updates.BootstrapRoot{Data: data, SHA256: *updatePin}
		if _, err = bootstrap.Validate(); err != nil {
			return p, err
		}
		p.configuration.UpdateBootstrap = bootstrap
	}
	if *updateMetadata != "" || *updateTargets != "" || *updateSequence != 0 {
		if p.configuration.UpdateBootstrap == nil {
			return p, install.ErrPlan
		}
		repository := &updates.RepositoryConfiguration{Schema: 1, MetadataURL: *updateMetadata, TargetsURL: *updateTargets, MinimumSequence: *updateSequence, MinimumCatalogVersion: p.configuration.MinimumCatalogVersion}
		if err := repository.Validate(); err != nil {
			return p, err
		}
		p.configuration.UpdateRepository = repository
	}
	if (*provenanceFile == "") != (*provenancePin == "") {
		return p, install.ErrPlan
	}
	if *provenanceFile != "" {
		if p.configuration.UpdateRepository == nil {
			return p, install.ErrPlan
		}
		data, err := readPinnedProvenancePolicy(*provenanceFile, *provenancePin)
		if err != nil {
			return p, err
		}
		p.configuration.UpdateProvenance = data
	}
	pub, err := hex.DecodeString(*key)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return p, install.ErrPlan
	}
	p.configuration.Publisher = ed25519.PublicKey(pub)
	data, err := readReleaseCatalog(*file)
	if err != nil {
		return p, err
	}
	p.configuration.Catalog = data
	manifest, err := catalog.Verify(data, map[string]ed25519.PublicKey{catalog.KeyID(pub): pub}, p.configuration.MinimumCatalogVersion, now)
	if err != nil || len(manifest.Images) != 3 {
		return p, catalog.ErrUntrusted
	}
	if err = checkReleaseSources(p.source, manifest); err != nil {
		return p, err
	}
	// Full planning repeats these checks with verified identities and live capacity.
	p.configuration.Policy.ControllerUID = 100
	p.configuration.Policy.TransferUID = 101
	if err = p.configuration.Policy.Validate(); err != nil {
		return p, err
	}
	if _, err = networkcheck.ValidateConfiguration(p.configuration.Network); err != nil {
		return p, err
	}
	return p, nil
}

func readReleaseCatalog(name string) ([]byte, error) {
	f, err := os.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > catalog.MaxManifestBytes {
		return nil, catalog.ErrUntrusted
	}
	data, err := io.ReadAll(io.LimitReader(f, catalog.MaxManifestBytes+1))
	if err != nil || len(data) > catalog.MaxManifestBytes {
		return nil, catalog.ErrUntrusted
	}
	return data, nil
}

func installPrepare(args []string) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		fatal(errors.New("installation preparation requires Linux and root"))
	}
	p, err := parsePreparation(args, time.Now())
	if err != nil {
		fatal(err)
	}
	report := hostcheck.Inspect("/var/lib")
	if !report.PreparationPrerequisitesMet() {
		_ = json.NewEncoder(os.Stdout).Encode(report)
		fatal(errors.New("host prerequisites failed; installation was not started"))
	}
	var engine *install.Engine
	if p.journal == "/var/lib/homenode-install" {
		engine, err = install.OpenSystem()
	} else {
		engine, err = install.Open("/", p.journal)
	}
	if err != nil {
		fatal(err)
	}
	defer engine.Close()
	interrupted, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(interrupted, 30*time.Minute)
	defer cancel()
	if _, err = engine.ProvisionAccounts(ctx); err != nil {
		fatal(err)
	}
	preview, err := engine.Configure(ctx, p.configuration)
	if err != nil {
		fatal(err)
	}
	if err = engine.PlaceImages(ctx, p.source, p.configuration.Publisher, p.configuration.MinimumCatalogVersion); err != nil {
		fatal(err)
	}
	pending := make([]string, 0, len(preview.Pending))
	for _, item := range preview.Pending {
		if item != "place and verify immutable guest images" {
			pending = append(pending, item)
		}
	}
	preview.Pending = pending
	if err = json.NewEncoder(os.Stdout).Encode(struct {
		Prepared          bool                         `json:"prepared"`
		ServicesActivated bool                         `json:"servicesActivated"`
		Preview           install.ConfigurationPreview `json:"preview"`
	}{true, false, preview}); err != nil {
		fatal(err)
	}
}

func checkReleaseSources(directory string, manifest catalog.Manifest) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, image := range manifest.Images {
		f, err := root.OpenFile(image.SHA256+".raw", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		info, err := f.Stat()
		f.Close()
		if err != nil || !info.Mode().IsRegular() || info.Size() != image.Bytes {
			return catalog.ErrUntrusted
		}
	}
	return nil
}

func readPinnedProvenancePolicy(name, pin string) ([]byte, error) {
	if len(pin) != 64 {
		return nil, install.ErrPlan
	}
	file, err := os.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 16384 {
		return nil, errors.Join(install.ErrPlan, err, file.Close())
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 16385))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if len(data) > 16384 || hex.EncodeToString(sum[:]) != pin {
		return nil, install.ErrPlan
	}
	if _, err := updates.ParseProvenancePolicy(data); err != nil {
		return nil, err
	}
	return data, nil
}
