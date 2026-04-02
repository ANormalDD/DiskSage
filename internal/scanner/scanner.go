package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"disksage/internal/models"
)

var knownMarkers = map[string]string{
	"package.json":       "nodejs",
	"go.mod":             "golang",
	"Cargo.toml":         "rust",
	"build.gradle":       "gradle",
	"pom.xml":            "maven",
	".git":               "git-repo",
	"docker-compose.yml": "docker",
}

type Scanner struct {
	cfg models.ScanConfig

	mu               sync.Mutex
	onProgress       func(models.ScanProgress)
	dirsSeen         int64
	filesSeen        int64
	bytesSeen        int64
	lastProgressAt   time.Time
	progressInterval time.Duration
}

func NewScanner(cfg models.ScanConfig) *Scanner {
	return &Scanner{cfg: cfg, progressInterval: 120 * time.Millisecond}
}

func (s *Scanner) SetProgressCallback(cb func(models.ScanProgress)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onProgress = cb
}

func (s *Scanner) ScanDrive(root string) (models.DirNode, error) {
	if root == "" {
		return models.DirNode{}, fmt.Errorf("empty root")
	}
	if strings.HasSuffix(root, ":") {
		root += string(filepath.Separator)
	}
	cleanRoot := filepath.Clean(root)
	s.resetProgress()
	node, err := s.scanPath(cleanRoot, 0)
	if err == nil {
		s.emitProgress(cleanRoot, true, true)
	}
	return node, err
}

func (s *Scanner) ScanDir(path string, depth int) (models.DirNode, error) {
	cfg := s.cfg
	cfg.MaxDepth = depth
	tmp := &Scanner{cfg: cfg}
	return tmp.scanPath(filepath.Clean(path), 0)
}

func (s *Scanner) scanPath(path string, depth int) (models.DirNode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return models.DirNode{}, err
	}

	node := models.DirNode{
		Path:    path,
		Name:    info.Name(),
		IsFile:  !info.IsDir(),
		ModTime: info.ModTime(),
	}

	if !info.IsDir() {
		node.Size = info.Size()
		s.recordFile(path, info.Size())
		return node, nil
	}

	s.recordDir(path)

	entries, err := os.ReadDir(path)
	if err != nil {
		return models.DirNode{}, err
	}

	if depth >= s.cfg.MaxDepth {
		size, modTime := estimateDirSize(path)
		node.Size = size
		node.ModTime = modTime
		node.MarkerLabels = detectMarkers(entries)
		return node, nil
	}

	var total int64
	latest := info.ModTime()
	children := make([]models.DirNode, 0, len(entries))

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() && shouldSkipDirName(name, s.cfg.SkipDirs) {
			continue
		}
		childPath := filepath.Join(path, name)
		if isSystemBlacklisted(childPath) {
			continue
		}

		child, childErr := s.scanPath(childPath, depth+1)
		if childErr != nil {
			continue
		}
		total += child.Size
		if child.ModTime.After(latest) {
			latest = child.ModTime
		}
		if child.IsFile || child.Size >= s.cfg.MinDirSize || depth == 0 {
			children = append(children, child)
		}
	}

	sort.Slice(children, func(i, j int) bool {
		return children[i].Size > children[j].Size
	})

	node.Size = total
	node.ModTime = latest
	node.Children = children
	node.MarkerLabels = detectMarkers(entries)

	return node, nil
}

func (s *Scanner) resetProgress() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirsSeen = 0
	s.filesSeen = 0
	s.bytesSeen = 0
	s.lastProgressAt = time.Time{}
}

func (s *Scanner) recordDir(path string) {
	s.mu.Lock()
	s.dirsSeen++
	s.mu.Unlock()
	s.emitProgress(path, false, false)
}

func (s *Scanner) recordFile(path string, size int64) {
	s.mu.Lock()
	s.filesSeen++
	s.bytesSeen += size
	s.mu.Unlock()
	s.emitProgress(path, false, false)
}

func (s *Scanner) emitProgress(path string, force bool, done bool) {
	s.mu.Lock()
	cb := s.onProgress
	if cb == nil {
		s.mu.Unlock()
		return
	}
	now := time.Now()
	if !force && !done && !s.lastProgressAt.IsZero() && now.Sub(s.lastProgressAt) < s.progressInterval {
		s.mu.Unlock()
		return
	}
	s.lastProgressAt = now
	progress := models.ScanProgress{
		Path:      path,
		DirsSeen:  s.dirsSeen,
		FilesSeen: s.filesSeen,
		BytesSeen: s.bytesSeen,
		Done:      done,
	}
	s.mu.Unlock()

	cb(progress)
}

func (s *Scanner) CheckDirContent(path string) (models.FileTypeDistribution, error) {
	stats := map[string]*models.FileTypeStat{}
	maxFiles := 3000
	seen := 0

	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if shouldSkipDirName(name, s.cfg.SkipDirs) {
				return fs.SkipDir
			}
			return nil
		}
		seen++
		if seen > maxFiles {
			return fs.SkipAll
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		pattern := classifyPattern(d.Name())
		st, ok := stats[pattern]
		if !ok {
			st = &models.FileTypeStat{Pattern: pattern}
			stats[pattern] = st
		}
		st.Count++
		st.Size += info.Size()
		return nil
	})
	if err != nil && err != fs.SkipAll {
		return models.FileTypeDistribution{}, err
	}

	out := make([]models.FileTypeStat, 0, len(stats))
	for _, st := range stats {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Size > out[j].Size
	})
	if len(out) > 10 {
		out = out[:10]
	}
	return models.FileTypeDistribution{Path: path, Stats: out}, nil
}

func detectMarkers(entries []os.DirEntry) []string {
	labels := make([]string, 0, 2)
	for _, entry := range entries {
		if label, ok := knownMarkers[entry.Name()]; ok {
			labels = append(labels, label)
		}
	}
	sort.Strings(labels)
	return labels
}

func classifyPattern(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return "(no-ext)"
	}
	return "*" + ext
}

func estimateDirSize(path string) (int64, time.Time) {
	var total int64
	latest := time.Time{}
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		total += info.Size()
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
		return nil
	})
	if latest.IsZero() {
		latest = time.Now()
	}
	return total, latest
}
