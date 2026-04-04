//go:build !windows

package scanner

import (
	"os"
	"time"
)

func fileTimes(info os.FileInfo) (time.Time, time.Time) {
	modified := info.ModTime()
	return modified, modified
}
