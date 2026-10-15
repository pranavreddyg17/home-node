package control

import (
	"errors"
	"strings"
)

var errDownloadRange = errors.New("invalid or unsatisfiable download range")

// Bounds are half-open. Unsupported range units and multipart requests are
// ignored in favor of the complete representation; only one byte range is
// served. Saturating decimal parsing handles arbitrarily large valid numbers
// within the bounded header without integer overflow.
func downloadRange(value string, size int64) (start, end int64, partial bool, err error) {
	end = size
	if value == "" {
		return
	}
	unit, bounds, found := strings.Cut(strings.TrimSpace(value), "=")
	if !found || !strings.EqualFold(unit, "bytes") {
		return
	}
	if len(value) > 128 || size <= 0 {
		return 0, 0, false, errDownloadRange
	}
	if strings.Contains(bounds, ",") {
		return
	}
	first, last, found := strings.Cut(strings.TrimSpace(bounds), "-")
	if !found {
		return 0, 0, false, errDownloadRange
	}
	if first == "" {
		length, ok := rangeNumber(last, size)
		if !ok || length == 0 {
			return 0, 0, false, errDownloadRange
		}
		return size - length, size, true, nil
	}
	start, found = rangeNumber(first, size)
	if !found || start >= size {
		return 0, 0, false, errDownloadRange
	}
	if last != "" {
		final, ok := rangeNumber(last, size)
		if !ok || final < start {
			return 0, 0, false, errDownloadRange
		}
		end = min(final, size-1) + 1
	}
	return start, end, true, nil
}

func rangeNumber(value string, ceiling int64) (int64, bool) {
	if value == "" {
		return 0, false
	}
	var number int64
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, false
		}
		digit := int64(character - '0')
		if number > ceiling/10 || (number == ceiling/10 && digit > ceiling%10) {
			number = ceiling
		} else {
			number = number*10 + digit
		}
	}
	return number, true
}
