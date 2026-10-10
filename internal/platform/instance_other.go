//go:build !linux

package platform

import (
	"errors"
	"os"
)

func LockInstance(path string) (*os.File, error) {
	return nil, errors.New("Me Tracker currently supports Linux desktop sessions only")
}
