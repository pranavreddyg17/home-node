package updates

import (
	"bytes"
	"testing"
)

func FuzzDebianArchiveStructure(f *testing.F) {
	f.Add(debianFixture("control.tar.gz", "data.tar.gz"))
	f.Add([]byte("!<arch>\n"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 2<<20 {
			t.Skip()
		}
		archive, err := inspectDebianArchive(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return
		}
		if archive.Control == nil || archive.Data == nil || archive.Control.Size() < 1 || archive.Control.Size() > 1<<20 || archive.Data.Size() < 1 || archive.Data.Size() > int64(len(data)) {
			t.Fatal("accepted archive escaped section bounds")
		}
		_, controlOffset, controlLength := archive.Control.Outer()
		_, dataOffset, dataLength := archive.Data.Outer()
		if controlOffset < 8 || controlLength > int64(len(data))-controlOffset || dataOffset <= controlOffset || dataLength > int64(len(data))-dataOffset {
			t.Fatal("accepted sections escaped input")
		}
	})
}

func FuzzDebianControlContent(f *testing.F) {
	control := "Package: homenode\nVersion: 0.1.0\nArchitecture: amd64\nMaintainer: fixture\nDepends: " + homeNodeDependencies + "\nDescription: fixture\n"
	f.Add(controlFixture(f, control, ""))
	f.Add([]byte{})
	f.Add(make([]byte, 1024))
	f.Add([]byte("Package: homenode\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		_ = ValidateControlArchive(bytes.NewReader(data), ReleaseMetadata{Release: "0.1.0", Platform: "ubuntu-24.04-amd64"})
	})
}
