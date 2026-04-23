package gemini

import (
	"fmt"
	"runtime"
)

const (
	tokenRefreshURL          = "https://oauth2.googleapis.com/token"
	userInfoURL              = "https://www.googleapis.com/oauth2/v1/userinfo?alt=json"
	generativeLanguage       = "https://generativelanguage.googleapis.com"
	cloudResourceMgr         = "https://cloudresourcemanager.googleapis.com"
	codeAssistEndpoint       = "https://cloudcode-pa.googleapis.com"
	codeAssistVersion        = "v1internal"
	geminiCLIVersion         = "0.31.0"
	geminiCLIApiClientHeader = "google-genai-sdk/1.41.0 gl-node/v22.19.0"

	clientID     = "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com"
	clientSecret = "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"
)

func geminiCLIUserAgent(model string) string {
	if model == "" {
		model = "unknown"
	}
	return fmt.Sprintf("GeminiCLI/%s/%s (%s; %s)", geminiCLIVersion, model, geminiCLIOS(), geminiCLIArch())
}

func geminiCLIOS() string {
	switch runtime.GOOS {
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}

func geminiCLIArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "x86"
	default:
		return runtime.GOARCH
	}
}
