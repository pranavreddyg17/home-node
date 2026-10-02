package backup

import "errors"

// ErrLaunchRepositoryAdmission classifies a worker failure while qualifying the
// repository before any runtime acquisition attempt. It is not a wire reply,
// stopped-worker acknowledgement or permission to clear maintenance barriers.
// Transport failure, cancellation during acquisition and lost acknowledgements
// must never be inferred to have this meaning from an empty runtime token.
var ErrLaunchRepositoryAdmission = errors.New("backup launch repository admission failed before runtime acquisition")

// ErrLaunchRepositoryRefusedStopped is returned only after the activated client
// receives the exact job-bound refusal reply on its authenticated worker socket.
// It is distinct from a worker-local admission error. Controller recovery still
// needs to validate and record the owned durable checkpoint before app effects.
var ErrLaunchRepositoryRefusedStopped = errors.New("backup worker acknowledged stopped repository refusal")
