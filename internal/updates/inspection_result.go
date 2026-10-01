package updates

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
)

const InspectionOperationPattern = `^[a-zA-Z0-9_-]{20,64}$`

var inspectionOperation = regexp.MustCompile(InspectionOperationPattern)

var ErrInspectionResult = errors.New("inspection result does not match trusted package identity")

// InspectionIdentity combines the protected maintenance operation with signed
// target identity, never values copied from the worker response or browser.
type InspectionIdentity struct {
	OperationID, Release, PackageSHA256 string
	PackageLength                       int64
}

type InspectionResult struct {
	OperationID       string `json:"operationId"`
	Schema            int    `json:"schema"`
	Release           string `json:"release"`
	PackageSHA256     string `json:"packageSha256"`
	PackageLength     int64  `json:"packageLength"`
	ContentValid      bool   `json:"contentValid"`
	InstallAuthorized bool   `json:"installAuthorized"`
}

// ValidateInspectionResult checks bounded, unambiguous output against trusted
// identity. The caller still must authenticate worker execution and completion.
// Content validity grants no installation or release-promotion authority.
func ValidateInspectionResult(data []byte, expected InspectionIdentity) (InspectionResult, error) {
	var zero InspectionResult
	hash, err := hex.DecodeString(expected.PackageSHA256)
	if !inspectionOperation.MatchString(expected.OperationID) || err != nil || len(hash) != 32 || hex.EncodeToString(hash) != expected.PackageSHA256 || expected.PackageLength < 1 || expected.PackageLength > 512<<20 || !releaseName.MatchString(expected.Release) || len(data) == 0 || len(data) > 2048 {
		return zero, ErrInspectionResult
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return zero, ErrInspectionResult
	}
	allowed := map[string]bool{"operationId": true, "schema": true, "release": true, "packageSha256": true, "packageLength": true, "contentValid": true, "installAuthorized": true}
	seen := map[string]bool{}
	for decoder.More() {
		token, err = decoder.Token()
		name, ok := token.(string)
		if err != nil || !ok || !allowed[name] || seen[name] {
			return zero, ErrInspectionResult
		}
		seen[name] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return zero, ErrInspectionResult
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') || len(seen) != len(allowed) {
		return zero, ErrInspectionResult
	}
	if decoder.Decode(new(any)) != io.EOF {
		return zero, ErrInspectionResult
	}
	var result InspectionResult
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || result.OperationID != expected.OperationID || result.Schema != 1 || result.Release != expected.Release || result.PackageSHA256 != expected.PackageSHA256 || result.PackageLength != expected.PackageLength || !result.ContentValid || result.InstallAuthorized {
		return zero, ErrInspectionResult
	}
	return result, nil
}
