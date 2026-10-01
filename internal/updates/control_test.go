package updates

import (
	"archive/tar"
	"bytes"
	"strings"
	"testing"
)

func controlFixture(t *testing.T, control, extra string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	if err := writer.WriteHeader(&tar.Header{Name: "./control", Mode: 0644, Size: int64(len(control)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(control)); err != nil {
		t.Fatal(err)
	}
	if extra != "" {
		if err := writer.WriteHeader(&tar.Header{Name: extra, Mode: 0755, Size: 0, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestControlArchiveBindsReleaseIdentityAndRejectsInstallHooks(t *testing.T) {
	control := "Package: homenode\nVersion: 0.1.0\nArchitecture: amd64\nMaintainer: fixture\nDepends: " + homeNodeDependencies + "\nDescription: fixture\n continuation\n"
	release := ReleaseMetadata{Release: "0.1.0", Platform: "ubuntu-24.04-amd64"}
	if err := ValidateControlArchive(bytes.NewReader(controlFixture(t, control, "")), release); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{"./postinst", "./preinst", "./postrm", "./prerm", "./triggers", "./control", "../control"} {
		if err := ValidateControlArchive(bytes.NewReader(controlFixture(t, control, extra)), release); err == nil {
			t.Fatal("install hook/extra control entry accepted", extra)
		}
	}
	for _, data := range []string{
		strings.Replace(control, "homenode", "other", 1),
		strings.Replace(control, "0.1.0", "0.2.0", 1),
		strings.Replace(control, "amd64", "arm64", 1),
		strings.Replace(control, homeNodeDependencies, "unexpected-package", 1),
		control + "Package: homenode\n",
		control + "Breaks: other\n",
		strings.Replace(control, "Package:", "package:", 1),
		strings.Replace(control, "Maintainer: fixture", "Maintainer: fixture\n illegal continuation", 1),
		control + "\x00",
	} {
		if err := ValidateControlArchive(bytes.NewReader(controlFixture(t, data, "")), release); err == nil {
			t.Fatal("invalid package control accepted")
		}
	}
	valid := controlFixture(t, control, "")
	for _, data := range [][]byte{valid[:300], append(append([]byte(nil), valid...), 'x'), append(append([]byte(nil), valid...), make([]byte, 1<<20)...)} {
		if err := ValidateControlArchive(bytes.NewReader(data), release); err == nil {
			t.Fatal("truncated or unbounded archive accepted")
		}
	}
}
