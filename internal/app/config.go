package app

import (
	"net/url"
	"os"
	"regexp"
	"strings"
)

// Set via -ldflags at build time.
var (
	Version   = "dev"
	BuildTime = "unknown"
)

type Config struct {
	ListenAddr  string
	DatabaseURL string
	DBType      string
}

func LoadConfig() Config {
	cfg := Config{
		ListenAddr:  strings.TrimSpace(os.Getenv("LISTEN_ADDR")),
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		DBType:      normalizeDBType(os.Getenv("DB_TYPE")),
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":3000"
	}
	if cfg.DatabaseURL == "" {
		switch cfg.DBType {
		case "", "sqlite":
			cfg.DBType = "sqlite"
			cfg.DatabaseURL = "meowcli.db"
		}
	} else if cfg.DBType == "" {
		cfg.DBType = "postgres"
	}
	return cfg
}

func normalizeDBType(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

var keywordDSNSecretPattern = regexp.MustCompile(`(?i)\b(password|passwd|pwd)\s*=\s*('[^']*'|"[^"]*"|\S+)`)

func RedactedDatabaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err == nil {
			if parsed.User != nil {
				username := parsed.User.Username()
				if username != "" {
					parsed.User = url.User(username)
				} else {
					parsed.User = nil
				}
			}

			query := parsed.Query()
			redacted := false
			for _, key := range []string{"password", "passwd", "pwd"} {
				if query.Has(key) {
					query.Set(key, "***")
					redacted = true
				}
			}
			if redacted {
				parsed.RawQuery = query.Encode()
			}

			return parsed.String()
		}
	}

	return keywordDSNSecretPattern.ReplaceAllString(raw, `${1}=***`)
}
