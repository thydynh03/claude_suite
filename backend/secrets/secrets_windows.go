//go:build windows

package secrets

import (
	"syscall"
	"unsafe"
)

var (
	crypt32           = syscall.NewLazyDLL("crypt32.dll")
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procProtectData   = crypt32.NewProc("CryptProtectData")
	procUnprotectData = crypt32.NewProc("CryptUnprotectData")
	procLocalFree     = kernel32.NewProc("LocalFree")
)

// dataBlob is the Win32 DATA_BLOB struct.
type dataBlob struct {
	cbData uint32
	pbData *byte
}

func blobOf(d []byte) dataBlob {
	if len(d) == 0 {
		return dataBlob{}
	}
	return dataBlob{cbData: uint32(len(d)), pbData: &d[0]}
}

// copyAndFree copies the API-allocated output and releases it. The copy is
// not optional: the buffer belongs to LocalAlloc, not the Go heap.
func copyAndFree(b dataBlob) []byte {
	if b.pbData == nil || b.cbData == 0 {
		return nil
	}
	out := make([]byte, b.cbData)
	copy(out, unsafe.Slice(b.pbData, b.cbData))
	_, _, _ = procLocalFree.Call(uintptr(unsafe.Pointer(b.pbData)))
	return out
}

// cryptprotectUIForbidden: never pop a dialog — this runs in a GUI app's
// startup path and in tests.
const cryptprotectUIForbidden = 0x1

func protect(plaintext []byte) ([]byte, error) {
	in := blobOf(plaintext)
	var out dataBlob
	r, _, callErr := procProtectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0,
		uintptr(cryptprotectUIForbidden),
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, callErr
	}
	return copyAndFree(out), nil
}

func unprotect(sealed []byte) ([]byte, error) {
	in := blobOf(sealed)
	var out dataBlob
	r, _, callErr := procUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0,
		uintptr(cryptprotectUIForbidden),
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, callErr
	}
	return copyAndFree(out), nil
}
