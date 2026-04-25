package app

import "strings"

type BuildInfo struct {
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
}

func CurrentBuildInfo() BuildInfo {
	return BuildInfo{
		Version:   defaultIfBlank(Version, "dev"),
		BuildTime: defaultIfBlank(BuildTime, "unknown"),
	}
}

func defaultIfBlank(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
