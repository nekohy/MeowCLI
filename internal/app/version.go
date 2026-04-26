package app

import "strings"

type BuildInfo struct {
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
}

func CurrentBuildInfo() BuildInfo {
	return BuildInfo{
		Version:   valueOrDefault(Version, "dev"),
		BuildTime: valueOrDefault(BuildTime, "unknown"),
	}
}

func valueOrDefault(value, defaultValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	return value
}
