package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/maypok86/otter/v2"
	"golang.org/x/oauth2"
)

const (
	defaultOAuthHTTPTimeout = 30 * time.Second
	defaultOAuthSessionTTL  = 10 * time.Minute
)

type OAuthFlow interface {
	CreateAuthorizationFlow() (*OAuthAuthorizationFlow, error)
	ExchangeAuthorizationCode(ctx context.Context, state string, code string) (*OAuthToken, error)
	RefreshAccessToken(ctx context.Context, refreshToken string) (*OAuthToken, error)
}

type OAuthConfig struct {
	Provider     string
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	RedirectURI  string
	Scopes       []string
	AuthParams   map[string]string
	TokenParams  map[string]string
	UsePKCE      bool

	HTTPClient   *http.Client
	Now          func() time.Time
	SessionTTL   time.Duration
	SessionCache *otter.Cache[string, OAuthAuthorizationFlow]
}

type OAuthClient struct {
	oauth2Config *oauth2.Config
	provider     string
	authParams   map[string]string
	tokenParams  map[string]string
	usePKCE      bool
	httpClient   *http.Client
	now          func() time.Time
	sessions     *otter.Cache[string, OAuthAuthorizationFlow]
	sessionTTL   time.Duration
}

type OAuthAuthorizationFlow struct {
	State         string
	CodeVerifier  string
	CodeChallenge string
	AuthorizeURL  string
}

type OAuthToken struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	TokenType    string
	Expiry       time.Time
}

func NewOAuthClient(cfg OAuthConfig) (*OAuthClient, error) {
	if strings.TrimSpace(cfg.ClientID) == "" {
		return nil, errors.New("oauth client_id is required")
	}
	if strings.TrimSpace(cfg.AuthURL) == "" {
		return nil, errors.New("oauth auth url is required")
	}
	if strings.TrimSpace(cfg.TokenURL) == "" {
		return nil, errors.New("oauth token url is required")
	}
	if strings.TrimSpace(cfg.RedirectURI) == "" {
		return nil, errors.New("oauth redirect uri is required")
	}
	if _, err := url.ParseRequestURI(cfg.AuthURL); err != nil {
		return nil, fmt.Errorf("parse oauth auth url: %w", err)
	}
	if _, err := url.ParseRequestURI(cfg.TokenURL); err != nil {
		return nil, fmt.Errorf("parse oauth token url: %w", err)
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultOAuthHTTPTimeout}
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	sessionTTL := cfg.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = defaultOAuthSessionTTL
	}
	sessions := cfg.SessionCache
	if sessions == nil {
		var err error
		sessions, err = otter.New[string, OAuthAuthorizationFlow](&otter.Options[string, OAuthAuthorizationFlow]{
			ExpiryCalculator: otter.ExpiryWriting[string, OAuthAuthorizationFlow](sessionTTL),
		})
		if err != nil {
			return nil, fmt.Errorf("create oauth session cache: %w", err)
		}
	}

	return &OAuthClient{
		oauth2Config: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURI,
			Scopes:       cfg.Scopes,
			Endpoint: oauth2.Endpoint{
				AuthURL:   cfg.AuthURL,
				TokenURL:  cfg.TokenURL,
				AuthStyle: oauth2.AuthStyleInParams,
			},
		},
		provider:    cfg.Provider,
		authParams:  cfg.AuthParams,
		tokenParams: cfg.TokenParams,
		usePKCE:     cfg.UsePKCE,
		httpClient:  client,
		now:         now,
		sessions:    sessions,
		sessionTTL:  sessionTTL,
	}, nil
}

func (c *OAuthClient) CreateAuthorizationFlow() (*OAuthAuthorizationFlow, error) {
	if c == nil {
		return nil, errors.New("oauth client is nil")
	}
	state, err := randomHex(16)
	if err != nil {
		return nil, err
	}

	var verifier string
	var challenge string
	opts := make([]oauth2.AuthCodeOption, 0, len(c.authParams)+1)
	for key, value := range c.authParams {
		opts = append(opts, oauth2.SetAuthURLParam(key, value))
	}
	if c.usePKCE {
		verifier = oauth2.GenerateVerifier()
		challenge = oauth2.S256ChallengeFromVerifier(verifier)
		opts = append(opts, oauth2.S256ChallengeOption(verifier))
	}
	authURL := c.oauth2Config.AuthCodeURL(state, opts...)

	flow := OAuthAuthorizationFlow{
		State:         state,
		CodeVerifier:  verifier,
		CodeChallenge: challenge,
		AuthorizeURL:  authURL,
	}
	if c.sessions != nil {
		c.sessions.Set(flow.State, flow)
		if c.sessionTTL > 0 {
			c.sessions.SetExpiresAfter(flow.State, c.sessionTTL)
		}
	}
	return &flow, nil
}

func (c *OAuthClient) ExchangeAuthorizationCode(ctx context.Context, state string, code string) (*OAuthToken, error) {
	if c == nil {
		return nil, errors.New("oauth client is nil")
	}
	flow, ok := c.consumeAuthorizationFlow(state)
	if !ok {
		return nil, providerError(c.provider, "authorization state not found or expired")
	}
	return c.exchangeAuthorizationCode(ctx, code, flow.CodeVerifier)
}

func (c *OAuthClient) consumeAuthorizationFlow(state string) (*OAuthAuthorizationFlow, bool) {
	if c == nil || c.sessions == nil {
		return nil, false
	}
	state = strings.TrimSpace(state)
	if state == "" {
		return nil, false
	}
	flow, ok := c.sessions.GetIfPresent(state)
	if !ok {
		return nil, false
	}
	c.sessions.Invalidate(state)
	return &OAuthAuthorizationFlow{
		State:         flow.State,
		CodeVerifier:  flow.CodeVerifier,
		CodeChallenge: flow.CodeChallenge,
		AuthorizeURL:  flow.AuthorizeURL,
	}, true
}

func (c *OAuthClient) exchangeAuthorizationCode(ctx context.Context, code string, verifier string) (*OAuthToken, error) {
	if c == nil {
		return nil, errors.New("oauth client is nil")
	}
	code = strings.TrimSpace(code)
	verifier = strings.TrimSpace(verifier)
	if code == "" {
		return nil, errors.New("authorization code is required")
	}
	if c.usePKCE && verifier == "" {
		return nil, errors.New("code verifier is required")
	}

	opts := make([]oauth2.AuthCodeOption, 0, len(c.tokenParams)+1)
	for key, value := range c.tokenParams {
		opts = append(opts, oauth2.SetAuthURLParam(key, value))
	}
	if c.usePKCE {
		opts = append(opts, oauth2.VerifierOption(verifier))
	}
	rawToken, err := c.oauth2Config.Exchange(c.withHTTPClient(ctx), code, opts...)
	if err != nil {
		return nil, providerError(c.provider, err.Error())
	}
	token := c.convertOAuth2Token(rawToken)
	if token.AccessToken == "" {
		return nil, providerError(c.provider, "token response returned empty access_token")
	}
	if strings.TrimSpace(token.RefreshToken) == "" {
		return nil, providerError(c.provider, "authorization code exchange returned empty refresh_token")
	}
	return token, nil
}

func (c *OAuthClient) RefreshAccessToken(ctx context.Context, refreshToken string) (*OAuthToken, error) {
	if c == nil {
		return nil, errors.New("oauth client is nil")
	}
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, errors.New("refresh token is required")
	}

	source := c.oauth2Config.TokenSource(c.withHTTPClient(ctx), &oauth2.Token{
		RefreshToken: refreshToken,
		Expiry:       c.now().Add(-time.Second),
	})
	rawToken, err := source.Token()
	if err != nil {
		return nil, providerError(c.provider, err.Error())
	}
	token := c.convertOAuth2Token(rawToken)
	if token.AccessToken == "" {
		return nil, providerError(c.provider, "token response returned empty access_token")
	}
	if strings.TrimSpace(token.RefreshToken) == "" {
		token.RefreshToken = refreshToken
	}
	return token, nil
}

func (c *OAuthClient) convertOAuth2Token(raw *oauth2.Token) *OAuthToken {
	token := &OAuthToken{
		AccessToken:  strings.TrimSpace(raw.AccessToken),
		RefreshToken: strings.TrimSpace(raw.RefreshToken),
		TokenType:    strings.TrimSpace(raw.TokenType),
		Expiry:       raw.Expiry,
	}
	if idToken, ok := raw.Extra("id_token").(string); ok {
		token.IDToken = strings.TrimSpace(idToken)
	}
	if token.Expiry.IsZero() {
		token.Expiry = c.now().Add(time.Hour)
	}
	return token
}

func (c *OAuthClient) withHTTPClient(ctx context.Context) context.Context {
	if c == nil || c.httpClient == nil {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, c.httpClient)
}

func providerError(provider string, msg string) error {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return errors.New(msg)
	}
	return fmt.Errorf("%s oauth: %s", provider, msg)
}

func randomHex(nBytes int) (string, error) {
	if nBytes <= 0 {
		return "", errors.New("invalid random byte length")
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
