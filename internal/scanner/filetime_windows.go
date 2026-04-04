//go:build windows

package scanner

import (
	"os"
	"syscall"
	"time"
)

func fileTimes(info os.FileInfo) (time.Time, time.Time) {
	modified := info.ModTime()
	created := modified

	if attrs, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		created = time.Unix(0, attrs.CreationTime.Nanoseconds())
		if created.IsZero() {
			created = modified
		}
	}

	return created, modified
}
