package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	antigravityapi "github.com/nekohy/MeowCLI/api/antigravity"
	geminiapi "github.com/nekohy/MeowCLI/api/gemini"
	oauthcore "github.com/nekohy/MeowCLI/core"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
)

type oauthCallbackRequest struct {
	State string `json:"state" binding:"required"`
	Code  string `json:"code" binding:"required"`
}

func (a *AdminHandler) SetOAuthFlows(flows map[utils.HandlerType]oauthcore.OAuthFlow) {
	a.oauthFlows = flows
}

func (a *AdminHandler) StartOAuth(c *gin.Context) {
	provider, flow, ok := a.oauthFlow(c)
	if !ok {
		return
	}

	authFlow, err := flow.CreateAuthorizationFlow()
	if err != nil {
		writeInternalError(c, err)
		return
	}
	if authFlow == nil || strings.TrimSpace(authFlow.State) == "" || strings.TrimSpace(authFlow.AuthorizeURL) == "" {
		writeInternalError(c, errors.New("oauth authorization flow is invalid"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"provider":      string(provider),
		"state":         authFlow.State,
		"authorize_url": authFlow.AuthorizeURL,
	})
}

func (a *AdminHandler) OAuthCallback(c *gin.Context) {
	provider, flow, ok := a.oauthFlow(c)
	if !ok {
		return
	}

	var req oauthCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.State = strings.TrimSpace(req.State)
	req.Code = strings.TrimSpace(req.Code)
	if req.State == "" || req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "state and code are required"})
		return
	}

	token, err := flow.ExchangeAuthorizationCode(c.Request.Context(), req.State, req.Code)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if token == nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "oauth token response is empty"})
		return
	}

	id, err := a.upsertOAuthCredential(c.Request.Context(), provider, token)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	a.invalidateCredentials(provider, []string{id})
	a.syncCredentialQuotas(context.Background(), provider, []string{id})

	c.JSON(http.StatusOK, gin.H{
		"provider": string(provider),
		"id":       id,
	})
}

func (a *AdminHandler) oauthFlow(c *gin.Context) (utils.HandlerType, oauthcore.OAuthFlow, bool) {
	provider, ok := utils.ParseHandlerType(strings.ToLower(strings.TrimSpace(c.Param("provider"))))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported oauth provider"})
		return "", nil, false
	}

	var flow oauthcore.OAuthFlow
	if a != nil && a.oauthFlows != nil {
		flow = a.oauthFlows[provider]
	}
	switch provider {
	case utils.HandlerCodex, utils.HandlerGemini, utils.HandlerAntigravity:
		if flow == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": string(provider) + " oauth backend is unavailable"})
			return provider, nil, false
		}
		return provider, flow, true
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported oauth provider"})
		return "", nil, false
	}
}

func (a *AdminHandler) upsertOAuthCredential(ctx context.Context, provider utils.HandlerType, token *oauthcore.OAuthToken) (string, error) {
	switch provider {
	case utils.HandlerCodex:
		return a.upsertCodexFromTokenData(ctx, token.AccessToken, token.RefreshToken, token.IDToken)
	case utils.HandlerGemini:
		credential, err := a.upsertGeminiCredentialFromTokenData(ctx, &geminiapi.TokenData{
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			Expiry:       token.Expiry,
		})
		if err != nil {
			return "", err
		}
		return credential.ID, nil
	case utils.HandlerAntigravity:
		credential, err := a.upsertAntigravityCredentialFromTokenData(ctx, &antigravityapi.TokenData{
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			Expiry:       token.Expiry,
		})
		if err != nil {
			return "", err
		}
		return credential.ID, nil
	default:
		return "", errors.New("unsupported oauth provider")
	}
}
