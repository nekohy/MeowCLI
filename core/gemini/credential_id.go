package gemini

import "strings"

const credentialIDProjectSeparator = "__"

func DefaultCredentialID(email, projectID string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	projectID = strings.TrimSpace(projectID)
	if email == "" || projectID == "" {
		return ""
	}
	return email + credentialIDProjectSeparator + projectID
}

func CredentialProjectID(credentialID string) string {
	id := strings.TrimSpace(credentialID)
	separatorIndex := strings.LastIndex(id, credentialIDProjectSeparator)
	if separatorIndex >= 0 {
		projectID := strings.TrimSpace(id[separatorIndex+len(credentialIDProjectSeparator):])
		if projectID != "" {
			return projectID
		}
	}
	return ""
}
