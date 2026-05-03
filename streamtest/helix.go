package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const helixBase = "https://api.twitch.tv/helix"

type helixClient struct {
	clientID    string
	accessToken string
}

func (h *helixClient) get(path string, query url.Values, dst any) (int, error) {
	req, err := http.NewRequest(http.MethodGet, helixBase+path+"?"+query.Encode(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Client-Id", h.clientID)
	req.Header.Set("Authorization", "Bearer "+h.accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return resp.StatusCode, nil
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("helix %s returned %d: %s", path, resp.StatusCode, string(body))
	}
	if dst != nil {
		if err := json.Unmarshal(body, dst); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

func (h *helixClient) UserID(login string) (string, error) {
	var resp struct {
		Data []struct {
			ID    string `json:"id"`
			Login string `json:"login"`
		} `json:"data"`
	}
	q := url.Values{"login": {login}}
	if _, err := h.get("/users", q, &resp); err != nil {
		return "", err
	}
	if len(resp.Data) == 0 {
		return "", fmt.Errorf("no twitch user named %q", login)
	}
	return resp.Data[0].ID, nil
}

func (h *helixClient) IsFollower(userID, broadcasterID string) (bool, error) {
	var resp struct {
		Data []json.RawMessage `json:"data"`
	}
	q := url.Values{
		"user_id":        {userID},
		"broadcaster_id": {broadcasterID},
	}
	if _, err := h.get("/channels/followed", q, &resp); err != nil {
		return false, err
	}
	return len(resp.Data) > 0, nil
}

func (h *helixClient) IsSubscriber(userID, broadcasterID string) (bool, error) {
	var resp struct {
		Data []json.RawMessage `json:"data"`
	}
	q := url.Values{
		"user_id":        {userID},
		"broadcaster_id": {broadcasterID},
	}
	code, err := h.get("/subscriptions/user", q, &resp)
	if err != nil {
		return false, err
	}
	if code == http.StatusNotFound {
		return false, nil
	}
	return len(resp.Data) > 0, nil
}
