package useragent

import (
	"fmt"
	"runtime"
)

const (
	AntigravityVersion       = "2.0.0"
	CodexCLI                 = "codex_cli_rs/0.76.0 (Debian 13.0.0; x86_64) WindowsTerminal"
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
