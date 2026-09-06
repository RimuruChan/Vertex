//go:build !linux

package instance

import "fmt"

// Lock is unavailable off Linux because the native sandbox is Linux-only.
type Lock struct{}

// Acquire fails closed on systems that cannot provide the Linux flock contract.
func Acquire(string) (*Lock, error) {
	return nil, fmt.Errorf("sandbox instance locking requires Linux")
}

// Close is a no-op for the unsupported placeholder.
func (lock *Lock) Close() error {
	return nil
}
