package control

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/workload"
)

func archiveFileName(index int, name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(name, ". ")
	if name == "" {
		name = "file"
	}
	// A unique numeric prefix prevents duplicate/case-normalized names and
	// Windows reserved device names. No user-controlled path components exist.
	return fmt.Sprintf("files/%05d-%s", index+1, name)
}

func (s *Server) exportFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		fail(w, 405, "METHOD_NOT_ALLOWED", "Use GET to export saved files.")
		return
	}
	device := actor(r).Device.ID
	if !s.downloads.acquire(device) {
		w.Header().Set("Retry-After", "1")
		fail(w, 429, "DOWNLOAD_BUSY", "Download capacity is busy. Wait for an active download to finish, then retry.")
		return
	}
	defer s.downloads.release(device)
	files, err := s.Workloads.ExportableFiles(r.Context())
	if err != nil {
		if errors.Is(err, workload.ErrCapacity) {
			fail(w, 507, "EXPORT_LIMIT", "This archive exceeds the supported file count or size. Download files individually instead.")
		} else {
			workloadError(w, err)
		}
		return
	}
	type entry struct {
		workload.File
		ArchivePath string `json:"archivePath"`
	}
	entries := make([]entry, 0, len(files))
	var total int64
	for i, file := range files {
		digest, err := hex.DecodeString(file.SHA256)
		if !guestproto.ValidID(file.ID) || !utf8.ValidString(file.Name) || len(file.Name) > 200 || file.Size < 0 || file.Size > workload.MaxFileBytes || err != nil || len(digest) != sha256.Size || hex.EncodeToString(digest) != file.SHA256 {
			workloadError(w, workload.ErrConflict)
			return
		}
		total += file.Size
		if total > workload.StorageQuota {
			fail(w, 507, "EXPORT_LIMIT", "This archive exceeds the supported file count or size. Download files individually instead.")
			return
		}
		entries = append(entries, entry{file, archiveFileName(i, file.Name)})
	}
	var first []byte
	if len(files) > 0 {
		first, err = s.Workloads.Download(r.Context(), files[0].ID, 0)
		if err != nil {
			workloadError(w, err)
			return
		}
		if int64(len(first)) == files[0].Size {
			digest := sha256.Sum256(first)
			if hex.EncodeToString(digest[:]) != files[0].SHA256 {
				workloadError(w, workload.ErrConflict)
				return
			}
		}
	}
	authorized := func() bool {
		session, err := s.Identity.Authenticate(r.Context(), s.readCookie(r, ""))
		return err == nil && session.Allows("files")
	}
	if !authorized() {
		fail(w, 401, "AUTH_REQUIRED", "Sign in again before exporting.")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="homenode-saved-files.zip"`)
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	controller := http.NewResponseController(w)
	archive := zip.NewWriter(w)
	// Any failure after streaming starts must terminate HTTP and leave no ZIP
	// central directory, never a successfully completed partial export.
	abort := func() { panic(http.ErrAbortHandler) }
	for i, file := range files {
		if !authorized() {
			abort()
		}
		_ = controller.SetWriteDeadline(time.Now().Add(15 * time.Second))
		output, err := archive.CreateHeader(&zip.FileHeader{Name: entries[i].ArchivePath, Method: zip.Store})
		if err != nil {
			abort()
		}
		hash := sha256.New()
		offset := int64(0)
		for {
			if !authorized() {
				abort()
			}
			data := first
			if i != 0 || offset != 0 {
				data, err = s.Workloads.Download(r.Context(), file.ID, offset)
			}
			if err != nil || !authorized() {
				abort()
			}
			_, _ = hash.Write(data)
			next := offset + int64(len(data))
			if next > file.Size || next == offset && offset != file.Size {
				abort()
			}
			if next == file.Size && (hex.EncodeToString(hash.Sum(nil)) != file.SHA256 || !authorized()) {
				abort()
			}
			_ = controller.SetWriteDeadline(time.Now().Add(15 * time.Second))
			if _, err = output.Write(data); err != nil {
				abort()
			}
			offset = next
			if offset == file.Size {
				break
			}
		}
	}
	if !authorized() {
		abort()
	}
	_ = controller.SetWriteDeadline(time.Now().Add(15 * time.Second))
	manifest, err := archive.Create("manifest.json")
	if err != nil {
		abort()
	}
	if err = json.NewEncoder(manifest).Encode(struct {
		Schema int     `json:"schema"`
		Scope  string  `json:"scope"`
		Files  []entry `json:"files"`
	}{1, "saved-files-excluding-trash", entries}); err != nil {
		abort()
	}
	if !authorized() {
		abort()
	}
	_ = controller.SetWriteDeadline(time.Now().Add(15 * time.Second))
	if err = archive.Close(); err != nil {
		abort()
	}
}
