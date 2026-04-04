//go:build windows

package scanner

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procGetCompressedFileSizeW  = kernel32.NewProc("GetCompressedFileSizeW")
	invalidCompressedFileSize32 = uintptr(0xFFFFFFFF)
)

func localFileSize(path string, info os.FileInfo) int64 {
	if info == nil || info.IsDir() {
		return 0
	}
	logical := info.Size()
	if logical <= 0 {
		return 0
	}
	if allocated, ok := allocatedFileSize(path); ok {
		if allocated < 0 {
			return 0
		}
		return allocated
	}
	return logical
}

func allocatedFileSize(path string) (int64, bool) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, false
	}

	var high uint32
	low, _, callErr := procGetCompressedFileSizeW.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&high)),
	)

	if low == invalidCompressedFileSize32 && callErr != syscall.Errno(0) {
		return 0, false
	}

	size := (uint64(high) << 32) | uint64(uint32(low))
	return int64(size), true
}
