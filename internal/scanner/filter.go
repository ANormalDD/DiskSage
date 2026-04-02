package scanner

import (
	"path/filepath"
	"strings"
)

var systemBlacklist = []string{
	`C:\Windows`,
	`C:\Program Files`,
	`C:\Program Files (x86)`,
	`$Recycle.Bin`,
	`System Volume Information`,
}

func shouldSkipDirName(name string, skipDirs []string) bool {
	for _, skip := range skipDirs {
		if strings.EqualFold(name, skip) {
			return true
		}
	}
	return false
}

func isSystemBlacklisted(path string) bool {
	normalized := strings.ToLower(filepath.Clean(path))
	for _, blocked := range systemBlacklist {
		if strings.Contains(normalized, strings.ToLower(filepath.Clean(blocked))) {
			return true
		}
	}
	return false
}
