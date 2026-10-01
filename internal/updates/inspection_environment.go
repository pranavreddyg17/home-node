package updates

import "fmt"

// InspectionEnvironment renders only the two fixed service inputs from the
// protected caller's retained identity. Its restricted alphabets exclude shell,
// systemd specifier, quoting and environment-file injection characters. The
// privileged launch coordinator must publish it as an owned private file and
// authenticate service completion; generating bytes activates no worker.
func InspectionEnvironment(identity InspectionIdentity) ([]byte, error) {
	if !validInspectionIdentity(identity) {
		return nil, ErrInspectionResult
	}
	return []byte(fmt.Sprintf("INSPECTION_RELEASE=%s\nINSPECTION_OPERATION_ID=%s\n", identity.Release, identity.OperationID)), nil
}
