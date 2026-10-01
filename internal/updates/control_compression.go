package updates

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"

	"github.com/klauspost/compress/zstd"
)

type contextualReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextualReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

// ValidateCompressedControl decodes a bounded control section, verifies the
// complete compressed stream, then applies HomeNode control-content policy.
// xz requires a constrained worker and is deliberately refused here. This is
// not a payload inspection or install authorization boundary.
func ValidateCompressedControl(ctx context.Context, reader io.Reader, compression string, release ReleaseMetadata) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	compressed, err := io.ReadAll(io.LimitReader(contextualReader{ctx, reader}, 1<<20+1))
	if err != nil || len(compressed) > 1<<20 {
		return errors.Join(ErrPackageControl, err)
	}
	var decoded io.Reader = bytes.NewReader(compressed)
	switch compression {
	case "none":
	case "gzip":
		gzipReader, err := gzip.NewReader(decoded)
		if err != nil {
			return errors.Join(ErrPackageControl, err)
		}
		defer func() { resultErr = errors.Join(resultErr, gzipReader.Close()) }()
		decoded = gzipReader
	case "zstd":
		zstdReader, err := zstd.NewReader(decoded, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(16<<20), zstd.WithDecoderMaxWindow(16<<20))
		if err != nil {
			return errors.Join(ErrPackageControl, err)
		}
		defer zstdReader.Close()
		decoded = zstdReader
	default:
		return ErrPackageControl
	}
	data, err := io.ReadAll(io.LimitReader(contextualReader{ctx, decoded}, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return errors.Join(ErrPackageControl, err)
	}
	return errors.Join(ValidateControlArchive(bytes.NewReader(data), release), ctx.Err())
}
