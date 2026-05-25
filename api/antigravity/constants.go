package antigravity

import "github.com/nekohy/MeowCLI/internal/useragent"

const (
	tokenRefreshURL                = "https://oauth2.googleapis.com/token"
	userInfoURL                    = "https://www.googleapis.com/oauth2/v1/userinfo?alt=json"
	antigravityBaseURLDaily        = "https://daily-cloudcode-pa.googleapis.com"
	antigravitySandboxBaseURLDaily = "https://daily-cloudcode-pa.sandbox.googleapis.com"
	antigravityBaseURLProd         = "https://cloudcode-pa.googleapis.com"
	codeAssistVersion              = "v1internal"
	defaultProjectID               = "bamboo-precept-lgxtn"
	antigravityVersion             = useragent.AntigravityVersion
	antigravityClientID            = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	antigravityClientSecret        = "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"

	antigravityNodeAPIClientUA = useragent.AntigravityNodeAPIClient
	antigravityGoogAPIClientUA = "gl-node/22.21.1"
)

func antigravityUserAgent() string {
	return useragent.AntigravityIDE()
}

func antigravityLoadCodeAssistUserAgent() string {
	return useragent.AntigravityLoadCodeAssist()
}
