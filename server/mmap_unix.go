//go:build linux || darwin || freebsd || netbsd || openbsd

package server

import (
	"os"
	"syscall"
)

// mapFile maps a file into memory, read only: its pages are read when used and stay in the page cache.
func mapFile(f *os.File, size int) ([]byte, func(), error) {
	b, err := syscall.Mmap(int(f.Fd()), 0, size, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, nil, err
	}
	return b, func() { syscall.Munmap(b) }, nil
}
