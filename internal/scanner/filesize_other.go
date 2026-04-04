//go:build !windows

package scanner

import "os"

func localFileSize(path string, info os.FileInfo) int64 {
	_ = path
	if info == nil || info.IsDir() {
		return 0
	}
	if info.Size() < 0 {
		return 0
	}
	return info.Size()
}
