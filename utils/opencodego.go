package utils

import (
	"net/mail"
	"regexp"
	"strings"
)

const openCodeGoCredentialIDSeparator = "--"

var openCodeGoWorkspaceIDPattern = regexp.MustCompile(`^wrk_[A-Za-z0-9]+$`)

func NormalizeOpenCodeGoEmail(raw string) string {
	email := strings.ToLower(strings.TrimSpace(raw))
	parsed, err := mail.ParseAddress(email)
	if err != nil || !strings.EqualFold(parsed.Address, email) {
		return ""
	}
	return email
}

func DefaultOpenCodeGoCredentialID(email, workspaceID string) string {
	email = NormalizeOpenCodeGoEmail(email)
	workspaceID = strings.TrimSpace(workspaceID)
	if email == "" || !openCodeGoWorkspaceIDPattern.MatchString(workspaceID) {
		return ""
	}
	return email + openCodeGoCredentialIDSeparator + workspaceID
}

func OpenCodeGoIdentityFromCredentialID(credentialID string) (email, workspaceID string, ok bool) {
	credentialID = strings.TrimSpace(credentialID)
	index := strings.LastIndex(credentialID, openCodeGoCredentialIDSeparator)
	if index <= 0 || index+len(openCodeGoCredentialIDSeparator) >= len(credentialID) {
		return "", "", false
	}
	email = NormalizeOpenCodeGoEmail(credentialID[:index])
	workspaceID = strings.TrimSpace(credentialID[index+len(openCodeGoCredentialIDSeparator):])
	if DefaultOpenCodeGoCredentialID(email, workspaceID) != credentialID {
		return "", "", false
	}
	return email, workspaceID, true
}

func OpenCodeGoEmailFromCredentialID(credentialID string) string {
	email, _, _ := OpenCodeGoIdentityFromCredentialID(credentialID)
	return email
}

func OpenCodeGoWorkspaceIDFromCredentialID(credentialID string) string {
	_, workspaceID, _ := OpenCodeGoIdentityFromCredentialID(credentialID)
	return workspaceID
}
