//go:build !windows

package secrets

// Non-Windows builds exist for CI; sealing is the identity function there.
// The header still marks the file, so the format is uniform and the tests
// exercise the same paths on every platform.

func protect(plaintext []byte) ([]byte, error) { return plaintext, nil }

func unprotect(sealed []byte) ([]byte, error) { return sealed, nil }
