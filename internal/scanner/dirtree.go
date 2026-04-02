package scanner

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"disksage/internal/models"
)

type RenderConfig struct {
	TopNPerLevel int
	MinChildSize int64
}

func RenderCompressedTree(root models.DirNode, cfg RenderConfig) string {
	if cfg.TopNPerLevel <= 0 {
		cfg.TopNPerLevel = 20
	}
	var b strings.Builder
	rootName := root.Path
	if rootName == "" {
		rootName = root.Name
	}
	b.WriteString(rootName)
	if !strings.HasSuffix(rootName, string(filepath.Separator)) {
		b.WriteString(string(filepath.Separator))
	}
	b.WriteString("\n")

	writeChildren(&b, root.Children, "", cfg)
	return strings.TrimRight(b.String(), "\n")
}

func writeChildren(b *strings.Builder, children []models.DirNode, prefix string, cfg RenderConfig) {
	if len(children) == 0 {
		return
	}
	sorted := append([]models.DirNode(nil), children...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Size > sorted[j].Size
	})

	major := make([]models.DirNode, 0, len(sorted))
	var othersCount int
	var othersSize int64
	for idx, child := range sorted {
		if idx < cfg.TopNPerLevel && child.Size >= cfg.MinChildSize {
			major = append(major, collapseSingleChildPath(child))
			continue
		}
		othersCount++
		othersSize += child.Size
	}

	for i, child := range major {
		last := i == len(major)-1 && othersCount == 0
		branch := "|-- "
		nextPrefix := prefix + "|   "
		if last {
			branch = "`-- "
			nextPrefix = prefix + "    "
		}
		line := fmt.Sprintf("%s%s[%s] %s", prefix, branch, humanSize(child.Size), child.Name)
		if !child.IsFile {
			line += "/"
		}
		if len(child.MarkerLabels) > 0 {
			line += " (" + strings.Join(child.MarkerLabels, ",") + ")"
		}
		b.WriteString(line + "\n")
		writeChildren(b, child.Children, nextPrefix, cfg)
	}

	if othersCount > 0 {
		b.WriteString(fmt.Sprintf("%s`-- ... (%d more dirs/files, total %s)\n", prefix, othersCount, humanSize(othersSize)))
	}
}

func collapseSingleChildPath(node models.DirNode) models.DirNode {
	curr := node
	for len(curr.Children) == 1 && !curr.Children[0].IsFile {
		only := curr.Children[0]
		curr.Name = curr.Name + "/" + only.Name
		curr.Children = only.Children
		if len(only.MarkerLabels) > 0 {
			curr.MarkerLabels = append(curr.MarkerLabels, only.MarkerLabels...)
		}
	}
	return curr
}

func humanSize(v int64) string {
	const unit = 1024
	if v < unit {
		return fmt.Sprintf("%dB", v)
	}
	div, exp := int64(unit), 0
	for n := v / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(v)/float64(div), "KMGTPE"[exp])
}
