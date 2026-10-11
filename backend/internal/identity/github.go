package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHub implements Provider using GitHub's official OAuth web flow
// (https://docs.github.com/apps/oauth-apps). Only least-privilege scopes are
// requested and the access token is discarded after fetching the profile.
type GitHub struct {
	ClientID     string
	ClientSecret string
	HTTP         *http.Client
}

// NewGitHub constructs a GitHub provider.
func NewGitHub(clientID, clientSecret string) *GitHub {
	return &GitHub{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		HTTP:         &http.Client{Timeout: 15 * time.Second},
	}
}

// Name returns the provider slug.
func (g *GitHub) Name() string { return "github" }

// AuthURL builds the GitHub authorize URL with least-privilege scope.
func (g *GitHub) AuthURL(state, redirectURI string) string {
	q := url.Values{}
	q.Set("client_id", g.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", "read:user")
	q.Set("state", state)
	q.Set("allow_signup", "true")
	return "https://github.com/login/oauth/authorize?" + q.Encode()
}

// Exchange swaps the code for a token and fetches the authenticated user.
func (g *GitHub) Exchange(ctx context.Context, code, redirectURI string) (*Profile, error) {
	form := url.Values{}
	form.Set("client_id", g.ClientID)
	form.Set("client_secret", g.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://github.com/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := g.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if tokenResp.Error != "" || tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("github token exchange failed: %s", tokenResp.Error)
	}

	// Fetch the authenticated user. The token is used only here and never stored.
	ureq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	ureq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	ureq.Header.Set("Accept", "application/vnd.github+json")
	uresp, err := g.HTTP.Do(ureq)
	if err != nil {
		return nil, err
	}
	defer uresp.Body.Close()
	ubody, _ := io.ReadAll(io.LimitReader(uresp.Body, 1<<20))
	if uresp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github user fetch: http %d", uresp.StatusCode)
	}
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err := json.Unmarshal(ubody, &user); err != nil {
		return nil, err
	}
	if user.ID == 0 || user.Login == "" {
		return nil, fmt.Errorf("github user response missing id/login")
	}
	return &Profile{Subject: fmt.Sprintf("%d", user.ID), Username: user.Login}, nil
}
