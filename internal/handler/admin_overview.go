package handler

import (
	"context"
	"net/http"

	corecodex "github.com/nekohy/MeowCLI/core/codex"
	requestplugin "github.com/nekohy/MeowCLI/plugin"
	pluginloader "github.com/nekohy/MeowCLI/plugin/loader"
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
	Key                             utils.HandlerType                `json:"key"`
	Label                           string                           `json:"label"`
	Status                          string                           `json:"status"`
	SupportedAPI                    []utils.APIType                  `json:"supported_api_types"`
	PlanList                        []string                         `json:"plan_list,omitempty"`
	SupportsCredentials             bool                             `json:"supports_credentials"`
	CredentialEndpoint              string                           `json:"credential_endpoint,omitempty"`
	CredentialFields                []credentialField                `json:"credential_fields,omitempty"`
	CredentialStatusOptions         []string                         `json:"credential_status_options,omitempty"`
	CredentialThrottleStatusOptions []credentialThrottleStatusOption `json:"credential_throttle_status_options,omitempty"`
	Plugins                         []requestplugin.Manifest         `json:"plugins,omitempty"`
	ModelsTotal                     int                              `json:"models_total"`
	CredentialsTotal                int                              `json:"credentials_total"`
	CredentialsEnabled              int64                            `json:"credentials_enabled"`
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

type credentialThrottleStatusOption struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Metric string `json:"metric"`
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

	logs, err := a.queryLogs(ctx, ListLogsParams{
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
	antigravityTotal, antigravityEnabled, err := a.antigravityCounts(ctx)
	if err != nil {
		return overviewResponse{}, err
	}
	openCodeGoTotal, err := a.store.CountOpenCodeGo(ctx)
	if err != nil {
		return overviewResponse{}, err
	}
	openCodeGoEnabled, err := a.store.CountEnabledOpenCodeGo(ctx)
	if err != nil {
		return overviewResponse{}, err
	}

	for i := range handlers {
		handlers[i].Plugins = a.availablePluginsForHandler(handlers[i].Key, handlers[i].SupportedAPI)
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
		case utils.HandlerAntigravity:
			handlers[i].CredentialsTotal = int(antigravityTotal)
			handlers[i].CredentialsEnabled = antigravityEnabled
		case utils.HandlerOpenCodeGo:
			handlers[i].CredentialsTotal = int(openCodeGoTotal)
			handlers[i].CredentialsEnabled = openCodeGoEnabled
		}
	}

	return overviewResponse{
		Summary: overviewSummary{
			CredentialsEnabled: codexEnabled + geminiEnabled + antigravityEnabled + openCodeGoEnabled,
			CredentialsTotal:   int(codexTotal + geminiTotal + antigravityTotal + openCodeGoTotal),
			ModelsTotal:        int(modelsTotal),
			LogsTotal:          logs.TotalStats.Total,
			AuthKeysTotal:      authKeysTotal,
		},
		Handlers:   handlers,
		RecentLogs: logs.Rows,
	}, nil
}

func (a *AdminHandler) availablePluginsForHandler(handler utils.HandlerType, apiTypes []utils.APIType) []requestplugin.Manifest {
	registry := a.pluginRegistry
	if registry == nil {
		registry = pluginloader.DefaultRegistry()
	}
	var plugins []requestplugin.Manifest
	seen := map[string]struct{}{}
	for _, apiType := range apiTypes {
		for _, plugin := range registry.Available(handler, apiType) {
			if _, ok := seen[plugin.Name]; ok {
				continue
			}
			seen[plugin.Name] = struct{}{}
			plugins = append(plugins, plugin)
		}
	}
	return plugins
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
			SupportedAPI:        []utils.APIType{utils.APIResponses, utils.APIResponsesCompact, utils.APICompletion},
			PlanList:            corecodex.PlanList(),
			SupportsCredentials: true,
			CredentialEndpoint:  credentialsEndpointForHandler(utils.HandlerCodex),
			CredentialFields: []credentialField{
				{
					Key:         "tokens",
					Label:       "Tokens",
					Kind:        "textarea",
					Placeholder: "一行一个 Refresh Token(@custom_client_id) 或 Access Token",
					Preferred:   true,
				},
			},
			CredentialStatusOptions: []string{"enabled", "disabled"},
			CredentialThrottleStatusOptions: []credentialThrottleStatusOption{
				{Value: "throttled:default", Label: "Default退避", Metric: "default"},
				{Value: "throttled:spark", Label: "Spark退避", Metric: "spark"},
			},
		},
		{
			Key:                 utils.HandlerGemini,
			Label:               "Gemini CLI",
			Status:              "available",
			SupportedAPI:        []utils.APIType{utils.APIGemini},
			PlanList:            utils.CodeAssistPlanList(),
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
			CredentialThrottleStatusOptions: []credentialThrottleStatusOption{
				{Value: "throttled:pro", Label: "Pro退避", Metric: "pro"},
				{Value: "throttled:flash", Label: "Flash退避", Metric: "flash"},
				{Value: "throttled:flashlite", Label: "Lite退避", Metric: "flashlite"},
			},
		},
		{
			Key:                 utils.HandlerAntigravity,
			Label:               "Antigravity",
			Status:              "available",
			SupportedAPI:        []utils.APIType{utils.APIGemini},
			PlanList:            utils.CodeAssistPlanList(),
			SupportsCredentials: true,
			CredentialEndpoint:  credentialsEndpointForHandler(utils.HandlerAntigravity),
			CredentialFields: []credentialField{
				{
					Key:         "tokens",
					Label:       "Refresh Tokens",
					Kind:        "textarea",
					Placeholder: "一行一个 Antigravity Refresh Token",
					Preferred:   true,
				},
			},
			CredentialStatusOptions: []string{"enabled", "disabled"},
			CredentialThrottleStatusOptions: []credentialThrottleStatusOption{
				{Value: "throttled:claude", Label: "Claude退避", Metric: "claude"},
				{Value: "throttled:pro", Label: "Pro退避", Metric: "pro"},
				{Value: "throttled:flash", Label: "Flash退避", Metric: "flash"},
				{Value: "throttled:flashlite", Label: "Lite退避", Metric: "flashlite"},
				{Value: "throttled:tab", Label: "Tab退避", Metric: "tab"},
				{Value: "throttled:image", Label: "Image退避", Metric: "image"},
			},
		},
		{
			Key:                 utils.HandlerOpenCodeGo,
			Label:               "OpenCode Go",
			Status:              "available",
			SupportedAPI:        []utils.APIType{utils.APICompletion},
			SupportsCredentials: true,
			CredentialEndpoint:  credentialsEndpointForHandler(utils.HandlerOpenCodeGo),
			CredentialFields: []credentialField{
				{
					Key:         "tokens",
					Label:       "Cookie",
					Kind:        "textarea",
					Placeholder: "Fe26开头的Auth值",
					HelpText:    "",
					Preferred:   true,
				},
			},
			CredentialStatusOptions: []string{"enabled", "disabled"},
			CredentialThrottleStatusOptions: []credentialThrottleStatusOption{
				{Value: "throttled:default", Label: "Default退避", Metric: "default"},
			},
		},
	}
}
