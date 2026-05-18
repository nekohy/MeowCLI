package antigravity

import (
	"fmt"
	"runtime"
)

const (
	tokenRefreshURL                = "https://oauth2.googleapis.com/token"
	userInfoURL                    = "https://www.googleapis.com/oauth2/v1/userinfo?alt=json"
	antigravityBaseURLDaily        = "https://daily-cloudcode-pa.googleapis.com"
	antigravitySandboxBaseURLDaily = "https://daily-cloudcode-pa.sandbox.googleapis.com"
	antigravityBaseURLProd         = "https://cloudcode-pa.googleapis.com"
	codeAssistVersion              = "v1internal"
	defaultProjectID               = "bamboo-precept-lgxtn"
	antigravityVersion             = "1.21.9"

	antigravityClientID     = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	antigravityClientSecret = "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"

	antigravityNodeAPIClientUA = "google-api-nodejs-client/10.3.0"
	antigravityGoogAPIClientUA = "gl-node/22.21.1"
)

func antigravityUserAgent() string {
	return fmt.Sprintf("antigravity/%s %s/%s", antigravityVersion, antigravityOS(), antigravityArch())
}

func antigravityLoadCodeAssistUserAgent() string {
	return antigravityUserAgent() + " " + antigravityNodeAPIClientUA
}

func antigravityOS() string {
	switch runtime.GOOS {
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}

func antigravityArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "x86"
	default:
		return runtime.GOARCH
	}
}
