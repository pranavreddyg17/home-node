package updates

import (
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
)

var ErrPackageArchive = errors.New("Debian package archive is outside HomeNode release policy")

// DebianArchive exposes bounded sections of an already-open package. The
// caller must keep its verified descriptor open. Structure alone establishes
// neither payload safety nor permission to install the package.
type DebianArchive struct {
	Control            *io.SectionReader
	Data               *io.SectionReader
	ControlCompression string
	DataCompression    string
}

// InspectDebianArchive accepts the exact three-member HomeNode package layout.
// It never extracts files, decompresses bytes, executes hooks or changes offset.
// Signed package digest verification must precede privileged use of the result.
func InspectDebianArchive(file *os.File) (DebianArchive, error) {
	if file == nil {
		return DebianArchive{}, ErrPackageArchive
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return DebianArchive{}, ErrPackageArchive
	}
	return inspectDebianArchive(file, info.Size())
}

func inspectDebianArchive(reader io.ReaderAt, size int64) (DebianArchive, error) {
	var result DebianArchive
	if size < 8+3*60+4 || size > 512<<20 {
		return result, ErrPackageArchive
	}
	var magic [8]byte
	if _, err := reader.ReadAt(magic[:], 0); err != nil || string(magic[:]) != "!<arch>\n" {
		return result, ErrPackageArchive
	}
	offset := int64(8)
	for index := 0; index < 3; index++ {
		var header [60]byte
		if offset > size-60 {
			return DebianArchive{}, ErrPackageArchive
		}
		if _, err := reader.ReadAt(header[:], offset); err != nil || string(header[58:]) != "`\n" {
			return DebianArchive{}, ErrPackageArchive
		}
		name := strings.TrimSuffix(strings.TrimRight(string(header[:16]), " "), "/")
		sizeText := strings.TrimRight(string(header[48:58]), " ")
		if sizeText == "" {
			return DebianArchive{}, ErrPackageArchive
		}
		for _, digit := range sizeText {
			if digit < '0' || digit > '9' {
				return DebianArchive{}, ErrPackageArchive
			}
		}
		length, err := strconv.ParseInt(sizeText, 10, 64)
		offset += 60
		if err != nil || length < 1 || length > size-offset {
			return DebianArchive{}, ErrPackageArchive
		}
		switch index {
		case 0:
			var version [4]byte
			if name != "debian-binary" || length != 4 {
				return DebianArchive{}, ErrPackageArchive
			}
			if _, err = reader.ReadAt(version[:], offset); err != nil || string(version[:]) != "2.0\n" {
				return DebianArchive{}, ErrPackageArchive
			}
		case 1, 2:
			prefix := "control.tar"
			if index == 2 {
				prefix = "data.tar"
			}
			compression := ""
			switch name {
			case prefix:
				compression = "none"
			case prefix + ".gz":
				compression = "gzip"
			case prefix + ".xz":
				compression = "xz"
			case prefix + ".zst":
				compression = "zstd"
			default:
				return DebianArchive{}, ErrPackageArchive
			}
			section := io.NewSectionReader(reader, offset, length)
			if index == 1 {
				if length > 1<<20 {
					return DebianArchive{}, ErrPackageArchive
				}
				result.Control, result.ControlCompression = section, compression
			} else {
				result.Data, result.DataCompression = section, compression
			}
		}
		offset += length
		if length%2 != 0 {
			var padding [1]byte
			if offset >= size {
				return DebianArchive{}, ErrPackageArchive
			}
			if _, err := reader.ReadAt(padding[:], offset); err != nil || padding[0] != '\n' {
				return DebianArchive{}, ErrPackageArchive
			}
			offset++
		}
	}
	if offset != size {
		return DebianArchive{}, ErrPackageArchive
	}
	return result, nil
}
