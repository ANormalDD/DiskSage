package cleaner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"disksage/internal/models"
)

type Strategy interface {
	Execute(ctx context.Context, item models.Recommendation, opts ExecuteOptions) (int64, error)
}

type ExecuteOptions struct {
	PermanentDelete bool
	ConfirmCommands bool
}

type DeleteStrategy struct{}

type CommandStrategy struct{}

type RedirectStrategy struct{}

func (s DeleteStrategy) Execute(ctx context.Context, item models.Recommendation, opts ExecuteOptions) (int64, error) {
	_ = ctx
	if item.Path == "" {
		return 0, fmt.Errorf("empty path")
	}
	if opts.PermanentDelete {
		if err := os.RemoveAll(item.Path); err != nil {
			return 0, err
		}
		return item.Size, nil
	}
	trashRoot := filepath.Join(os.TempDir(), "disksage", "recycle")
	if err := os.MkdirAll(trashRoot, 0o755); err != nil {
		return 0, err
	}
	dest := filepath.Join(trashRoot, fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(item.Path)))
	if err := os.Rename(item.Path, dest); err != nil {
		if err2 := os.RemoveAll(item.Path); err2 != nil {
			return 0, err
		}
	}
	return item.Size, nil
}

func (s CommandStrategy) Execute(ctx context.Context, item models.Recommendation, opts ExecuteOptions) (int64, error) {
	if !opts.ConfirmCommands {
		return 0, fmt.Errorf("command execution requires explicit confirmation")
	}
	cmdline := strings.TrimSpace(item.Command)
	if cmdline == "" {
		return 0, fmt.Errorf("empty command")
	}
	if !isCommandAllowed(cmdline) {
		return 0, fmt.Errorf("command not allowed: %s", cmdline)
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", cmdline)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdline)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("command failed: %v (%s)", err, string(out))
	}
	return item.Size, nil
}

func (s RedirectStrategy) Execute(ctx context.Context, item models.Recommendation, opts ExecuteOptions) (int64, error) {
	_ = ctx
	_ = opts
	if runtime.GOOS == "windows" {
		_ = exec.Command("explorer.exe", "/select,"+item.Path).Start()
	}
	return 0, nil
}

func isCommandAllowed(command string) bool {
	allow := []string{
		"npm cache clean",
		"docker system prune",
		"go clean -modcache",
	}
	lower := strings.ToLower(command)
	for _, prefix := range allow {
		if strings.HasPrefix(lower, strings.ToLower(prefix)) {
			return true
		}
	}
	return false
}
