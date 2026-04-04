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
	quotedArgs := make([]string, 0, len(os.Args)-1)
	for _, arg := range os.Args[1:] {
		quotedArgs = append(quotedArgs, quotePSSingle(arg))
	}
	argumentList := "@()"
	if len(quotedArgs) > 0 {
		argumentList = "@(" + strings.Join(quotedArgs, ",") + ")"
	}
	ps := fmt.Sprintf("Start-Process -FilePath %s -ArgumentList %s -Verb RunAs", quotePSSingle(exe), argumentList)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	return cmd.Start()
}

func quotePSSingle(value string) string {
	escaped := strings.ReplaceAll(value, "'", "''")
	return "'" + escaped + "'"
}
