package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	twitchDeviceURL   = "https://id.twitch.tv/oauth2/device"
	twitchTokenURL    = "https://id.twitch.tv/oauth2/token"
	twitchValidateURL = "https://id.twitch.tv/oauth2/validate"
)

var twitchScopes = []string{
	"chat:read",
	"chat:edit",
	"user:read:follows",
	"user:read:subscriptions",
}

type Auth struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token,omitempty"`
	Login        string   `json:"login"`
	UserID       string   `json:"user_id"`
	Scopes       []string `json:"scopes"`
	ClientID     string   `json:"client_id"`
}

func (a *Auth) IRCToken() string {
	return "oauth:" + a.AccessToken
}

func Authorize(clientID, cachePath string) (*Auth, error) {
	if a, err := loadAuth(cachePath); err == nil && a != nil && a.ClientID == clientID {
		if err := validateAuth(a); err == nil && hasAllScopes(a.Scopes, twitchScopes) {
			return a, nil
		}
	}
	a, err := deviceFlow(clientID)
	if err != nil {
		return nil, err
	}
	if err := saveAuth(cachePath, a); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not cache token to %s: %v\n", cachePath, err)
	}
	return a, nil
}

func hasAllScopes(have, need []string) bool {
	set := map[string]struct{}{}
	for _, s := range have {
		set[s] = struct{}{}
	}
	for _, n := range need {
		if _, ok := set[n]; !ok {
			return false
		}
	}
	return true
}

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type tokenResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	Scope        []string `json:"scope"`
	TokenType    string   `json:"token_type"`
	Message      string   `json:"message"`
	Status       int      `json:"status"`
}

type validateResponse struct {
	ClientID string   `json:"client_id"`
	Login    string   `json:"login"`
	UserID   string   `json:"user_id"`
	Scopes   []string `json:"scopes"`
}

func deviceFlow(clientID string) (*Auth, error) {
	dc, err := requestDeviceCode(clientID)
	if err != nil {
		return nil, fmt.Errorf("request device code: %w", err)
	}

	fmt.Printf("\n  Open %s\n", dc.VerificationURI)
	fmt.Printf("  Enter code: %s\n\n", dc.UserCode)
	fmt.Printf("  Waiting for authorization (expires in %ds)...\n", dc.ExpiresIn)

	interval := time.Duration(dc.Interval) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(interval)
		tok, err := pollToken(clientID, dc.DeviceCode)
		if err != nil {
			return nil, err
		}
		if tok != nil {
			v, err := validateToken(tok.AccessToken)
			if err != nil {
				return nil, fmt.Errorf("validate token: %w", err)
			}
			return &Auth{
				AccessToken:  tok.AccessToken,
				RefreshToken: tok.RefreshToken,
				Login:        v.Login,
				UserID:       v.UserID,
				Scopes:       v.Scopes,
				ClientID:     v.ClientID,
			}, nil
		}
	}
	return nil, errors.New("device code expired before authorization completed")
}

func requestDeviceCode(clientID string) (*deviceCodeResponse, error) {
	form := url.Values{
		"client_id": {clientID},
		"scopes":    {strings.Join(twitchScopes, " ")},
	}
	resp, err := http.PostForm(twitchDeviceURL, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var dc deviceCodeResponse
	if err := json.Unmarshal(body, &dc); err != nil {
		return nil, err
	}
	return &dc, nil
}

func pollToken(clientID, deviceCode string) (*tokenResponse, error) {
	form := url.Values{
		"client_id":   {clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	resp, err := http.PostForm(twitchTokenURL, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var tok tokenResponse
	_ = json.Unmarshal(body, &tok)
	if resp.StatusCode == http.StatusOK && tok.AccessToken != "" {
		return &tok, nil
	}
	msg := strings.ToLower(tok.Message)
	if strings.Contains(msg, "authorization_pending") || strings.Contains(msg, "missing device code") || strings.Contains(msg, "slow_down") {
		return nil, nil
	}
	if msg != "" {
		return nil, fmt.Errorf("twitch token endpoint: %s", tok.Message)
	}
	return nil, fmt.Errorf("twitch token endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func validateToken(token string) (*validateResponse, error) {
	req, err := http.NewRequest(http.MethodGet, twitchValidateURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "OAuth "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("validate returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var v validateResponse
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func validateAuth(a *Auth) error {
	v, err := validateToken(a.AccessToken)
	if err != nil {
		return err
	}
	a.Login = v.Login
	a.UserID = v.UserID
	a.Scopes = v.Scopes
	return nil
}

func loadAuth(path string) (*Auth, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var a Auth
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func saveAuth(path string, a *Auth) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
