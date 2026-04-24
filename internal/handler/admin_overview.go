package handler

import (
	"context"
	"net/http"

	corecodex "github.com/nekohy/MeowCLI/core/codex"
	coregemini "github.com/nekohy/MeowCLI/core/gemini"
	"github.com/nekohy/MeowCLI/utils"

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
	Status                  string            `json:"status"`
	SupportedAPI            []utils.APIType   `json:"supported_api_types"`
	PlanList                []string          `json:"plan_list,omitempty"`
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

	codexEnabled, err := a.store.CountEnabledCodex(ctx)
	if err != nil {
		return overviewResponse{}, err
	}

	codexTotal, err := a.store.CountCodex(ctx)
	if err != nil {
		return overviewResponse{}, err
	}

	modelsTotal, err := a.store.CountModels(ctx)
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

	authKeysTotal, err := a.store.CountAuthKeys(ctx)
	if err != nil {
		return overviewResponse{}, err
	}

	geminiTotal, geminiEnabled, geminiAvailable, err := a.geminiCounts(ctx)
	if err != nil {
		return overviewResponse{}, err
	}

	for i := range handlers {
		count, countErr := a.store.CountModelsByHandler(ctx, string(handlers[i].Key))
		if countErr != nil {
			return overviewResponse{}, countErr
		}
		handlers[i].ModelsTotal = int(count)
	}

	for i := range handlers {
		switch handlers[i].Key {
		case utils.HandlerCodex:
			handlers[i].CredentialsTotal = int(codexTotal)
			handlers[i].CredentialsEnabled = codexEnabled
		case utils.HandlerGemini:
			if geminiAvailable {
				handlers[i].CredentialsTotal = int(geminiTotal)
				handlers[i].CredentialsEnabled = geminiEnabled
			}
		}
	}

	return overviewResponse{
		Summary: overviewSummary{
			CredentialsEnabled: codexEnabled + geminiEnabled,
			CredentialsTotal:   int(codexTotal + geminiTotal),
			ModelsTotal:        int(modelsTotal),
			LogsTotal:          logCount,
			AuthKeysTotal:      authKeysTotal,
		},
		Handlers:   handlers,
		RecentLogs: recentLogs,
	}, nil
}

func credentialsEndpointForHandler(handler utils.HandlerType) string {
	return "/admin/api/" + string(handler)
}

func defaultHandlerOverview() []handlerOverview {
	return []handlerOverview{
		{
			Key:                 utils.HandlerCodex,
			Label:               "Codex CLI",
			Status:              "available",
			SupportedAPI:        []utils.APIType{utils.APIResponses, utils.APIResponsesCompact},
			PlanList:            corecodex.PlanList(),
			SupportsCredentials: true,
			CredentialEndpoint:  credentialsEndpointForHandler(utils.HandlerCodex),
			CredentialFields: []credentialField{
				{
					Key:         "tokens",
					Label:       "Tokens",
					Kind:        "textarea",
					Placeholder: "一行一个 Refresh Token 或 Access Token",
					Preferred:   true,
				},
			},
			CredentialStatusOptions: []string{"enabled", "disabled"},
		},
		{
			Key:                 utils.HandlerGemini,
			Label:               "Gemini CLI",
			Status:              "available",
			SupportedAPI:        []utils.APIType{utils.APIGemini},
			PlanList:            coregemini.PlanList(),
			SupportsCredentials: true,
			CredentialEndpoint:  credentialsEndpointForHandler(utils.HandlerGemini),
			CredentialFields: []credentialField{
				{
					Key:         "tokens",
					Label:       "Refresh Tokens",
					Kind:        "textarea",
					Placeholder: "一行一个 Refresh Token",
					Preferred:   true,
				},
			},
			CredentialStatusOptions: []string{"enabled", "disabled"},
		},
	}
}
