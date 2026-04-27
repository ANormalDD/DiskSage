package cleaner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

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

var redirectDeleteFallback DeleteStrategy

func (s DeleteStrategy) Execute(ctx context.Context, item models.Recommendation, opts ExecuteOptions) (int64, error) {
	_ = ctx
	_ = opts
	if item.Path == "" {
		return 0, fmt.Errorf("empty path")
	}
	if err := os.RemoveAll(item.Path); err != nil {
		return 0, err
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
	// Backward compatibility: historical "redirect" recommendations should still
	// be cleaned automatically instead of requiring manual explorer interaction.
	return redirectDeleteFallback.Execute(ctx, item, opts)
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
