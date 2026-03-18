package handler

import (
	"context"
	"github.com/nekohy/MeowCLI/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

const overviewRecentLogsLimit int32 = 6

type overviewResponse struct {
	Summary    overviewSummary   `json:"summary"`
	Handlers   []handlerOverview `json:"handlers"`
	RecentLogs []LogRow          `json:"recent_logs"`
}

type overviewSummary struct {
	CredentialsEnabled int64 `json:"credentials_enabled"`
	CredentialsTotal   int   `json:"credentials_total"`
	ModelsTotal        int   `json:"models_total"`
	LogsTotal          int64 `json:"logs_total"`
	AuthKeysTotal      int64 `json:"auth_keys_total"`
}

type handlerOverview struct {
	Key                     utils.HandlerType `json:"key"`
	Label                   string            `json:"label"`
	Summary                 string            `json:"summary"`
	Status                  string            `json:"status"`
	SupportedAPI            []utils.APIType   `json:"supported_api_types"`
	SupportsCredentials     bool              `json:"supports_credentials"`
	CredentialEndpoint      string            `json:"credential_endpoint,omitempty"`
	CredentialFields        []credentialField `json:"credential_fields,omitempty"`
	CredentialStatusOptions []string          `json:"credential_status_options,omitempty"`
	ModelsTotal             int               `json:"models_total"`
	CredentialsTotal        int               `json:"credentials_total"`
	CredentialsEnabled      int64             `json:"credentials_enabled"`
}

type credentialField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Kind        string `json:"kind"`
	Placeholder string `json:"placeholder,omitempty"`
	HelpText    string `json:"help_text,omitempty"`
	Optional    bool   `json:"optional,omitempty"`
	Preferred   bool   `json:"preferred,omitempty"`
}

func (a *AdminHandler) Overview(c *gin.Context) {
	resp, err := a.buildOverview(c.Request.Context())
	if err != nil {
		writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (a *AdminHandler) buildOverview(ctx context.Context) (overviewResponse, error) {
	handlers := defaultHandlerOverview()
	handlerIndex := make(map[utils.HandlerType]int, len(handlers))
	for i := range handlers {
		handlerIndex[handlers[i].Key] = i
	}

	enabledCreds, err := a.store.CountEnabledCodex(ctx)
	if err != nil {
		return overviewResponse{}, err
	}

	codexTotal, err := a.store.CountCodex(ctx)
	if err != nil {
		return overviewResponse{}, err
	}

	modelRows, err := a.store.ListModels(ctx)
	if err != nil {
		return overviewResponse{}, err
	}

	logCount, err := a.countLogs(ctx)
	if err != nil {
		return overviewResponse{}, err
	}

	recentLogs, err := a.listLogs(ctx, ListLogsParams{
		Limit:  overviewRecentLogsLimit,
		Offset: 0,
	})
	if err != nil {
		return overviewResponse{}, err
	}

	authKeys, err := a.store.ListAuthKeys(ctx)
	if err != nil {
		return overviewResponse{}, err
	}

	for _, row := range modelRows {
		key := utils.HandlerType(row.Handler)
		if index, ok := handlerIndex[key]; ok {
			handlers[index].ModelsTotal++
		}
	}

	if index, ok := handlerIndex[utils.HandlerCodex]; ok {
		handlers[index].CredentialsTotal = int(codexTotal)
		handlers[index].CredentialsEnabled = enabledCreds
	}

	return overviewResponse{
		Summary: overviewSummary{
			CredentialsEnabled: enabledCreds,
			CredentialsTotal:   int(codexTotal),
			ModelsTotal:        len(modelRows),
			LogsTotal:          logCount,
			AuthKeysTotal:      int64(len(authKeys)),
		},
		Handlers:   handlers,
		RecentLogs: recentLogs,
	}, nil
}

func defaultHandlerOverview() []handlerOverview {
	return []handlerOverview{
		{
			Key:                 utils.HandlerCodex,
			Label:               "Codex CLI",
			Summary:             "Codex池",
			Status:              "available",
			SupportedAPI:        []utils.APIType{utils.APIResponses, utils.APIResponsesCompact},
			SupportsCredentials: true,
			CredentialEndpoint:  "/admin/api/codex",
			CredentialFields: []credentialField{
				{
					Key:         "tokens",
					Label:       "Tokens",
					Kind:        "textarea",
					Placeholder: "一行一个 Refresh Token 或 Access Token",
					HelpText:    "每行一个 token，系统自动识别 RT 或 ATRT 会先刷新获取 AT",
					Preferred:   true,
				},
			},
			CredentialStatusOptions: []string{"enabled", "disabled"},
		},
	}
}
