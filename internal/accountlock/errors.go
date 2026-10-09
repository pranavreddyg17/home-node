package accountlock

import "errors"

var ErrInvalid = errors.New("invalid account lock scope")
var ErrConflict = errors.New("account lock authority changed or busy")
