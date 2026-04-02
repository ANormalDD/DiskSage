package privilege

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func RequestElevation() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("elevation is only supported on windows")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := strings.Join(os.Args[1:], " ")
	ps := fmt.Sprintf("Start-Process -FilePath '%s' -ArgumentList '%s' -Verb RunAs", exe, args)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	return cmd.Start()
}
