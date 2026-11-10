//go:build !(linux || darwin || freebsd || netbsd || openbsd)

package server

import (
	"io"
	"os"
)

// mapFile reads a file into memory where it cannot be mapped.
func mapFile(f *os.File, size int) ([]byte, func(), error) {
	b := make([]byte, size)
	if _, err := io.ReadFull(f, b); err != nil {
		return nil, nil, err
	}
	return b, func() {}, nil
}
