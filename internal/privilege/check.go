package privilege

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func IsElevated() bool {
	if runtime.GOOS != "windows" {
		return true
	}
	cmd := exec.Command("cmd", "/C", "net", "session")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func NeedsElevation(path string) bool {
	normalized := strings.ToLower(filepath.Clean(path))
	if runtime.GOOS != "windows" {
		return false
	}
	restricted := []string{
		strings.ToLower(filepath.Clean(`C:\Windows`)),
		strings.ToLower(filepath.Clean(`C:\Program Files`)),
		strings.ToLower(filepath.Clean(`C:\Program Files (x86)`)),
		strings.ToLower(filepath.Clean(`C:\ProgramData`)),
	}
	for _, p := range restricted {
		if strings.HasPrefix(normalized, p) {
			return true
		}
	}
	return false
}
