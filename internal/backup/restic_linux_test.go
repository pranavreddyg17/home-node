//go:build linux

package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRepositoryDirectoryRefusesSymlink(t *testing.T) {
	parentPath := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(parentPath, "homenode-backup")); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if repo, err := repositoryDirectory(parent, false); err == nil {
		repo.Close()
		t.Fatal("symlink repository admitted")
	}
}

func TestPasswordDescriptorIsSealedAndAnonymous(t *testing.T) {
	password := []byte("a private test password")
	file, err := passwordDescriptor(password)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(data, password) {
		t.Fatal("password descriptor", err)
	}
	if _, err = file.WriteAt([]byte("x"), 0); err == nil {
		t.Fatal("password memory is writable")
	}
	for _, invalid := range [][]byte{nil, []byte("new\nline"), []byte("nul\x00byte"), make([]byte, 8193)} {
		if descriptor, err := passwordDescriptor(invalid); err == nil {
			descriptor.Close()
			t.Fatal("invalid password accepted")
		}
	}
}

func TestRealResticRepositoryAuthentication(t *testing.T) {
	if os.Getenv("HOMENODE_RESTIC_INTEGRATION") != "1" {
		t.Skip("requires disposable Linux restic integration environment")
	}
	directory, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	password := []byte("repository fixture password")
	// Initialize only this disposable test repository; no registered-drive writes.
	if _, err = resticConfig(context.Background(), directory, password, true); err != nil {
		t.Fatal("real restic init", err)
	}
	data, err := resticConfig(context.Background(), directory, password, false)
	if err != nil {
		t.Fatal("real restic config", err)
	}
	var config struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	target := registeredTarget()
	target.RepositoryID = config.ID
	if err = VerifyRepository(context.Background(), directory, target, password); err != nil {
		t.Fatal(err)
	}
	target.RepositoryID = string(bytes.Repeat([]byte("0"), 64))
	if err = VerifyRepository(context.Background(), directory, target, password); err == nil {
		t.Fatal("wrong registered repository admitted")
	}
	target.RepositoryID = config.ID
	if err = VerifyRepository(context.Background(), directory, target, []byte("wrong password")); err == nil {
		t.Fatal("wrong password admitted")
	}
	repository, err := openRepository(context.Background(), directory, target, password)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	overlap, overlapErr := openRepository(context.Background(), directory, target, password)
	if overlap != nil {
		overlap.Close()
	}
	if overlapErr == nil || overlap != nil {
		t.Fatal("second authenticated repository handle admitted")
	}
	if err = os.Rename(filepath.Join(directory.Name(), "homenode-backup"), filepath.Join(directory.Name(), "original")); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(directory.Name(), "homenode-backup"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = repository.Check(context.Background()); err != nil {
		t.Fatal("pinned repository check followed replacement", err)
	}
	if err = VerifyRepository(context.Background(), directory, target, password); err == nil {
		t.Fatal("replacement directory authenticated")
	}
	if err = repository.Close(); err != nil {
		t.Fatal(err)
	}
	if err = repository.Check(context.Background()); err == nil {
		t.Fatal("closed repository retained credentials")
	}
}

func TestRealResticRecoverySnapshotRoundTrip(t *testing.T) {
	if os.Getenv("HOMENODE_RESTIC_INTEGRATION") != "1" {
		t.Skip("requires disposable Linux restic integration environment")
	}
	parent, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	password := []byte("snapshot integration password")
	if _, err = resticConfig(context.Background(), parent, password, true); err != nil {
		t.Fatal(err)
	}
	data, err := resticConfig(context.Background(), parent, password, false)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	target := registeredTarget()
	target.RepositoryID = config.ID
	repository, err := openRepository(context.Background(), parent, target, password)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	emptyInventory, err := repository.Snapshots(context.Background())
	if err != nil || emptyInventory == nil || len(emptyInventory) != 0 {
		t.Fatal("empty encrypted repository inventory", emptyInventory, err)
	}
	cancelledInventory, cancelInventory := context.WithCancel(context.Background())
	cancelInventory()
	if entries, err := repository.Snapshots(cancelledInventory); entries != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled inventory returned candidates", entries, err)
	}
	_, manifest, policy, path := recoverySet(t)

	diskPath := filepath.Join(path, "files.raw")
	disk, err := os.OpenFile(diskPath, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := disk.Truncate(16 << 20); err != nil {
		disk.Close()
		t.Fatal(err)
	}
	if err := disk.Close(); err != nil {
		t.Fatal(err)
	}
	formatCtx, cancelFormat := context.WithTimeout(context.Background(), time.Minute)
	format := exec.CommandContext(formatCtx, "/usr/sbin/mkfs.ext4", "-q", "-F", "-m", "0", "-E", "nodiscard,lazy_itable_init=0,lazy_journal_init=0", diskPath)
	output, formatErr := format.CombinedOutput()
	cancelFormat()
	if formatErr != nil {
		t.Fatalf("format recovery disk: %v: %s", formatErr, output)
	}
	diskBytes, err := os.ReadFile(diskPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(diskBytes)
	manifest.Files[1].Bytes = int64(len(diskBytes))
	manifest.Files[1].SHA256 = hex.EncodeToString(digest[:])
	stage, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Close()
	snapshot, err := repository.Snapshot(context.Background(), stage, manifest, policy)
	if err != nil || !repositoryPattern.MatchString(snapshot) {
		t.Fatal("real snapshot", snapshot, err)
	}
	if err = repository.Check(context.Background()); err != nil {
		t.Fatal("encrypted packs", err)
	}
	inventory, err := repository.Snapshots(context.Background())
	if err != nil || len(inventory) != 1 || inventory[0].ID != snapshot || inventory[0].CreatedAt.IsZero() {
		t.Fatal("published snapshot not selectable", inventory, err)
	}
	page, err := repository.SnapshotPage(context.Background(), "")
	if err != nil || len(page.Snapshots) != 1 || page.Snapshots[0].ID != snapshot || page.Next != "" {
		t.Fatal("published snapshot page", page, err)
	}
	finalPage, err := repository.SnapshotPage(context.Background(), snapshot)
	if err != nil || finalPage.Snapshots == nil || len(finalPage.Snapshots) != 0 || finalPage.Next != "" {
		t.Fatal("terminal cursor page", finalPage, err)
	}
	if _, err := repository.SnapshotPage(context.Background(), string(bytes.Repeat([]byte("f"), 64))); !errors.Is(err, ErrRepository) {
		t.Fatal("missing repository cursor accepted", err)
	}
	if _, err := repository.SnapshotPage(cancelledInventory, ""); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled page accepted", err)
	}
	preview, err := repository.InspectSnapshot(context.Background(), snapshot, policy)
	if err != nil || preview.Release != manifest.Release || !preview.CreatedAt.Equal(manifest.CreatedAt) || len(preview.Files) != len(manifest.Files) {
		t.Fatal("selected manifest preview", preview, err)
	}
	compatibilitySummary, err := repository.PreviewSnapshot(context.Background(), snapshot, policy)
	if err != nil || compatibilitySummary.SnapshotID != snapshot || compatibilitySummary.Validation != "metadata-compatible" || compatibilitySummary.Release != manifest.Release || len(compatibilitySummary.Files) != len(manifest.Files) {
		t.Fatal("selected compatibility compatibilitySummary", compatibilitySummary, err)
	}
	for index, file := range compatibilitySummary.Files {
		if file.Workload != manifest.Files[index].Workload || file.Bytes != manifest.Files[index].Bytes {
			t.Fatal("declared workload compatibilitySummary differs", file)
		}
	}
	previewRequestID := "encrypted-preview-request-fixture"
	previewWire, err := EncodeSnapshotPreviewResponse(previewRequestID, compatibilitySummary)
	if err != nil {
		t.Fatal("encrypted snapshot summary did not fit preview transport", err)
	}
	decodedPreview, err := DecodeSnapshotPreviewResponse(previewWire, previewRequestID, snapshot)
	if err != nil || decodedPreview.Release != manifest.Release || decodedPreview.CatalogVersion != manifest.CatalogVersion || len(decodedPreview.Files) != len(manifest.Files) {
		t.Fatal("encrypted summary wire round trip", decodedPreview, err)
	}
	incompatiblePreviewPolicy := policy
	incompatiblePreviewPolicy.MinimumCatalogVersion = manifest.CatalogVersion + 1
	if _, err := repository.InspectSnapshot(context.Background(), snapshot, incompatiblePreviewPolicy); !errors.Is(err, ErrManifest) {
		t.Fatal("incompatible preview admitted", err)
	}
	if compatibilitySummary, err := repository.PreviewSnapshot(context.Background(), snapshot, incompatiblePreviewPolicy); !errors.Is(err, ErrManifest) || compatibilitySummary.Files != nil {
		t.Fatal("incompatible compatibilitySummary admitted", compatibilitySummary, err)
	}
	if _, err := repository.InspectSnapshot(cancelledInventory, snapshot, policy); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled preview admitted", err)
	}
	// Verify actual encrypted storage returns the exact sanitized database bytes.
	// Physical guest consistency and drive admission remain separate gates.
	var restored bytes.Buffer
	args := []string{"--repo", "/proc/self/fd/3", "--password-file", "/proc/self/fd/4", "--no-cache", "dump", snapshot, "/proc/self/fd/5/snapshot.db"}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = resticProcess(ctx, args, []*os.File{repository.directory, repository.secret}, &restored); err != nil {
		t.Fatal("restore bytes", err)
	}
	original, err := os.ReadFile(filepath.Join(path, "snapshot.db"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored.Bytes(), original) {
		t.Fatal("encrypted snapshot round trip changed bytes")
	}
	restorePath := t.TempDir()
	restoreStage, err := os.Open(restorePath)
	if err != nil {
		t.Fatal(err)
	}
	defer restoreStage.Close()
	// testing.TempDir creates child directories with 0777 filtered by umask;
	// explicitly exercise and then satisfy restore's private-staging contract.
	if err = os.Chmod(restorePath, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.Restore(context.Background(), snapshot, restoreStage, policy); err == nil {
		t.Fatal("public staging accepted")
	}
	if err = os.Chmod(restorePath, 0700); err != nil {
		t.Fatal(err)
	}

	cancelled, cancelRestore := context.WithCancel(context.Background())
	cancelRestore()
	if _, err := repository.Restore(cancelled, snapshot, restoreStage, policy); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled restore did not report cancellation", err)
	}
	if entries, err := os.ReadDir(restorePath); err != nil || len(entries) != 0 {
		t.Fatal("cancelled restore wrote staging", entries, err)
	}
	if _, err = repository.Restore(context.Background(), snapshot, restoreStage, policy); err != nil {
		t.Fatal("validated set restore", err)
	}
	installRoot, err := os.OpenRoot(restorePath)
	if err != nil {
		t.Fatal(err)
	}
	defer installRoot.Close()
	installInventory, err := RecoveryInstallInventory(context.Background(), installRoot, manifest, policy)
	if err != nil || len(installInventory) != 1 {
		t.Fatal("qualified restored disk inventory", installInventory, err)
	}
	diskPlan := installInventory[0]
	if diskPlan.Workload != "files" || diskPlan.SourceName != "files.raw" || diskPlan.Bytes != manifest.Files[1].Bytes || diskPlan.SourceSHA256 != manifest.Files[1].SHA256 || diskPlan.ImageSHA256 != policy.ApprovedImages["files"] || diskPlan.InstanceID == "" {
		t.Fatal("restored installation mapping differs", diskPlan)
	}
	replanned, err := RecoveryInstallInventory(context.Background(), installRoot, manifest, policy)
	if err != nil || len(replanned) != 1 || replanned[0].InstanceID == diskPlan.InstanceID {
		t.Fatal("transient plans reused target identity", replanned, err)
	}
	copyPath := t.TempDir()
	if err = os.Chmod(copyPath, 0700); err != nil {
		t.Fatal(err)
	}
	copyRoot, err := os.OpenRoot(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer copyRoot.Close()
	copyName := ".recovery-" + diskPlan.InstanceID + ".stage"
	if err = copyRecoveryDisk(context.Background(), installRoot, copyRoot, diskPlan, copyName); err != nil {
		t.Fatal("recovered disk private copy", err)
	}
	copyEntry := manifest.Files[1]
	copyEntry.Name = copyName
	if err = verifyBackupFile(context.Background(), copyRoot, copyEntry); err != nil {
		t.Fatal("copied recovered payload identity", err)
	}
	copiedDisk, err := copyRoot.Open(copyName)
	if err != nil {
		t.Fatal(err)
	}
	checkCopyErr := QualifyExt4Disk(context.Background(), copiedDisk)
	if err = errors.Join(checkCopyErr, copiedDisk.Close()); err != nil {
		t.Fatal("copied recovered filesystem", err)
	}
	if err = copyRecoveryDisk(context.Background(), installRoot, copyRoot, diskPlan, copyName); !errors.Is(err, os.ErrExist) {
		t.Fatal("recovered staging copy overwritten", err)
	}
	restoredDatabase, err := os.ReadFile(filepath.Join(restorePath, "snapshot.db"))
	if err != nil || !bytes.Equal(restoredDatabase, original) {
		t.Fatal("restored database mismatch", err)
	}
	if _, err = repository.Restore(context.Background(), snapshot, restoreStage, policy); err == nil {
		t.Fatal("existing restore files overwritten")
	}
	failedPath := t.TempDir()
	if err = os.Chmod(failedPath, 0700); err != nil {
		t.Fatal(err)
	}
	failedStage, err := os.Open(failedPath)
	if err != nil {
		t.Fatal(err)
	}
	defer failedStage.Close()
	wrongPolicy := policy
	wrongPolicy.MinimumCatalogVersion = manifest.CatalogVersion + 1
	if _, err = repository.Restore(context.Background(), snapshot, failedStage, wrongPolicy); err == nil {
		t.Fatal("incompatible recovery installed")
	}
	if entries, err := os.ReadDir(failedPath); err != nil || len(entries) != 0 {
		t.Fatal("failed restore left payload", entries, err)
	}
	// Create a deliberately incomplete repository snapshot using the real tool.
	// Its manifest names a disk absent from the snapshot, so restore must remove
	// the already retrieved database when the later disk lookup fails.
	incomplete := &boundedOutput{maximum: 32768}
	partialArgs := []string{"--repo", "/proc/self/fd/3", "--password-file", "/proc/self/fd/4", "--no-cache", "--json", "backup", "--quiet", "--host", "fixture", "--", "/proc/self/fd/5/manifest.json", "/proc/self/fd/5/snapshot.db"}
	if err = resticProcess(ctx, partialArgs, []*os.File{repository.directory, repository.secret, stage}, incomplete); err != nil {
		t.Fatal(err)
	}
	var summary struct {
		ID string `json:"snapshot_id"`
	}
	if err = json.Unmarshal(incomplete.data, &summary); err != nil || !repositoryPattern.MatchString(summary.ID) {
		t.Fatal("incomplete fixture summary", err)
	}
	filtered, err := repository.Snapshots(context.Background())
	if err != nil || len(filtered) != 1 || filtered[0].ID != snapshot {
		t.Fatal("untagged snapshot became a HomeNode candidate", filtered, err)
	}
	if _, err = repository.Restore(context.Background(), summary.ID, failedStage, policy); err == nil {
		t.Fatal("missing disk restored successfully")
	}
	if entries, err := os.ReadDir(failedPath); err != nil || len(entries) != 0 {
		t.Fatal("partial restore retained files", entries, err)
	}

	// Publish a compatible manifest whose declared disk cannot fit while keeping
	// the host reserve. No huge disk is created: the capacity check must precede
	// retrieval, independently of the absent payload in this repository snapshot.
	if errors.Is(requireStagingSpace(failedStage, 512<<30), ErrStagingCapacity) {
		oversized := manifest
		oversized.Files = append([]BackupFile(nil), manifest.Files...)
		oversized.Files[1].Bytes = 512 << 30
		encoded, err := json.Marshal(oversized)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "manifest.json"), encoded, 0600); err != nil {
			t.Fatal(err)
		}
		capacityOutput := &boundedOutput{maximum: 32768}
		capacityCtx, cancelCapacity := context.WithTimeout(context.Background(), time.Minute)
		defer cancelCapacity()
		if err := resticProcess(capacityCtx, partialArgs, []*os.File{repository.directory, repository.secret, stage}, capacityOutput); err != nil {
			t.Fatal("capacity fixture snapshot", err)
		}
		var capacitySummary struct {
			ID string `json:"snapshot_id"`
		}
		if err := json.Unmarshal(capacityOutput.data, &capacitySummary); err != nil || !repositoryPattern.MatchString(capacitySummary.ID) {
			t.Fatal("capacity fixture summary", err)
		}
		if _, err := repository.Restore(capacityCtx, capacitySummary.ID, failedStage, policy); !errors.Is(err, ErrStagingCapacity) {
			t.Fatal("restore did not refuse capacity before disk retrieval", err)
		}
		if entries, err := os.ReadDir(failedPath); err != nil || len(entries) != 0 {
			t.Fatal("capacity refusal retained partial database", entries, err)
		}
	} else {
		t.Log("capacity refusal fixture requires less than 512GiB plus reserve available")
	}

	// Corrupt the root inode without changing the clean superblock state, then
	// honestly checksum it. Restore must reject filesystem structure, rather than
	// relying on a manifest hash mismatch to detect this failure.
	corruptCtx, cancelCorrupt := context.WithTimeout(context.Background(), time.Minute)
	defer cancelCorrupt()
	corrupt := exec.CommandContext(corruptCtx, "/usr/sbin/debugfs", "-w", "-R", "set_inode_field <2> mode 0", diskPath)
	if output, err := corrupt.CombinedOutput(); err != nil {
		t.Fatalf("corrupt restored disk fixture: %v: %s", err, output)
	}
	corruptDisk, err := os.Open(diskPath)
	if err != nil {
		t.Fatal(err)
	}
	headerErr := requireCleanExt4Header(corruptDisk)
	corruptDisk.Close()
	if headerErr != nil {
		t.Fatal("corruption fixture did not retain clean header", headerErr)
	}
	corruptBytes, err := os.ReadFile(diskPath)
	if err != nil {
		t.Fatal(err)
	}
	corruptManifest := manifest
	corruptManifest.Files = append([]BackupFile(nil), manifest.Files...)
	corruptDigest := sha256.Sum256(corruptBytes)
	corruptManifest.Files[1].SHA256 = hex.EncodeToString(corruptDigest[:])
	encodedCorrupt, err := json.Marshal(corruptManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "manifest.json"), encodedCorrupt, 0600); err != nil {
		t.Fatal(err)
	}
	corruptRoot, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	metadataErr := ValidateRecoverySet(corruptCtx, corruptRoot, corruptManifest, policy)
	corruptRoot.Close()
	if metadataErr != nil {
		t.Fatal("corruption fixture did not pass independent metadata validation", metadataErr)
	}
	corruptOutput := &boundedOutput{maximum: 32768}
	corruptArgs := append(append([]string(nil), partialArgs...), "/proc/self/fd/5/files.raw")
	if err := resticProcess(corruptCtx, corruptArgs, []*os.File{repository.directory, repository.secret, stage}, corruptOutput); err != nil {
		t.Fatal("corrupt encrypted snapshot", err)
	}
	corruptID, err := parseSnapshotSummary(corruptOutput.data)
	if err != nil {
		t.Fatal("corrupt fixture summary", err)
	}
	// Metadata inspection is intentionally weaker than recovered filesystem
	// qualification. It must not be presented as a successful restore test.
	corruptPreview, err := repository.InspectSnapshot(corruptCtx, corruptID, policy)
	if err != nil || len(corruptPreview.Files) != len(corruptManifest.Files) || corruptPreview.Files[1].SHA256 != corruptManifest.Files[1].SHA256 {
		t.Fatal("compatible metadata preview unexpectedly certified or rejected disk contents", corruptPreview, err)
	}
	corruptSummary, err := repository.PreviewSnapshot(corruptCtx, corruptID, policy)
	if err != nil || corruptSummary.Validation != "metadata-compatible" || corruptSummary.SnapshotID != corruptID {
		t.Fatal("declared compatibility summary mistaken for filesystem qualification", corruptSummary, err)
	}
	if entries, err := os.ReadDir(failedPath); err != nil || len(entries) != 0 {
		t.Fatal("preview extracted recovery files", entries, err)
	}
	if _, err := repository.Restore(corruptCtx, corruptID, failedStage, policy); !errors.Is(err, ErrManifest) {
		t.Fatal("checksummed corrupt filesystem restored", err)
	}
	if entries, err := os.ReadDir(failedPath); err != nil || len(entries) != 0 {
		t.Fatal("filesystem refusal retained recovery files", entries, err)
	}
	if err = os.WriteFile(filepath.Join(path, "files.raw"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if id, err := repository.Snapshot(context.Background(), stage, manifest, policy); err == nil || id != "" {
		t.Fatal("corrupt payload reported backed up")
	}
}
