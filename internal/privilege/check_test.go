package privilege

import (
	"runtime"
	"testing"
)

func TestNeedsElevation(t *testing.T) {
	if runtime.GOOS == "windows" {
		if !NeedsElevation(`C:\Windows\Temp`) {
			t.Fatalf("expected elevation for windows temp")
		}
	}
	if NeedsElevation("D:/Projects") {
		t.Fatalf("did not expect elevation for user path")
	}
}
