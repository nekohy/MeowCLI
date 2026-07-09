package useragent

import (
	"fmt"
	"runtime"
)

const (
	AntigravityVersion       = "2.0.0"
	CodexCLI                 = "codex-tui/0.144.0 (Windows 10.0.26100; x86_64) unknown (codex-tui; 0.144.0)"
	AntigravityNodeAPIClient = "google-api-nodejs-client/10.3.0"
)

func AntigravityIDE() string {
	return fmt.Sprintf("antigravity/ide/%s %s/%s", AntigravityVersion, runtimeOS(), runtimeArch())
}

func AntigravityLoadCodeAssist() string {
	return AntigravityIDE() + " " + AntigravityNodeAPIClient
}

func runtimeOS() string {
	switch runtime.GOOS {
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}

func runtimeArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "x86"
	default:
		return runtime.GOARCH
	}
}
