//go:build windows

package crypto

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func masterKeyProtectionRequired() bool { return true }

func protectMasterKey(plain []byte) ([]byte, error) {
	if len(plain) == 0 {
		return nil, fmt.Errorf("empty master key")
	}
	in := windows.DataBlob{Size: uint32(len(plain)), Data: &plain[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	protected := make([]byte, out.Size)
	copy(protected, unsafe.Slice(out.Data, out.Size))
	return protected, nil
}

func unprotectMasterKey(protected []byte) ([]byte, error) {
	if len(protected) == 0 {
		return nil, fmt.Errorf("empty protected master key")
	}
	in := windows.DataBlob{Size: uint32(len(protected)), Data: &protected[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	plain := make([]byte, out.Size)
	copy(plain, unsafe.Slice(out.Data, out.Size))
	return plain, nil
}
