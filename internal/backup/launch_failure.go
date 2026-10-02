package backup

import "errors"

// ErrLaunchRepositoryAdmission classifies a worker failure while qualifying the
// repository before any runtime acquisition attempt. It is not a wire reply,
// stopped-worker acknowledgement or permission to clear maintenance barriers.
// Transport failure, cancellation during acquisition and lost acknowledgements
// must never be inferred to have this meaning from an empty runtime token.
var ErrLaunchRepositoryAdmission = errors.New("backup launch repository admission failed before runtime acquisition")
