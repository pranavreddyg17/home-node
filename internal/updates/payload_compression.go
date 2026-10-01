package updates

import (
	"compress/gzip"
	"context"
	"errors"
	"io"

	"github.com/klauspost/compress/zstd"
)

// ValidateCompressedPayload streams bounded data through the payload inventory
// validator. Complete-stream errors and cancellation reject the result. This
// does not establish signed release identity or installation authority. xz
// remains pending integration with the constrained inspection worker.
func ValidateCompressedPayload(ctx context.Context, reader io.Reader, compression string) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	limited := &io.LimitedReader{R: contextualReader{ctx, reader}, N: 512<<20 + 1}
	var decoded io.Reader = limited
	switch compression {
	case "none":
	case "gzip":
		gzipReader, err := gzip.NewReader(decoded)
		if err != nil {
			return errors.Join(ErrPackagePayload, err)
		}
		defer func() { resultErr = errors.Join(resultErr, gzipReader.Close()) }()
		decoded = gzipReader
	case "zstd":
		zstdReader, err := zstd.NewReader(decoded, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(64<<20), zstd.WithDecoderMaxWindow(32<<20))
		if err != nil {
			return errors.Join(ErrPackagePayload, err)
		}
		defer zstdReader.Close()
		decoded = zstdReader
	default:
		return ErrPackagePayload
	}
	err := ValidatePayloadArchive(ctx, decoded)
	if limited.N <= 0 {
		return errors.Join(ErrPackagePayload, err)
	}
	return errors.Join(err, ctx.Err())
}
