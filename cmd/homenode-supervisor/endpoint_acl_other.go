//go:build !linux

package main

import (
	"errors"
	"os"
)

func qualifyEndpointParentACL(*os.File) error {
	return errors.New("endpoint parents require Linux")
}
