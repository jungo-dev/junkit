// Package recaptcha verifies Google reCAPTCHA v3 tokens.
package recaptcha

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Verifier defines the interface for verifying reCAPTCHA v3 tokens.
type Verifier interface {
	Verify(ctx context.Context, token string, remoteIP string) error
}

// Options configures reCAPTCHA token verification.
type Options struct {
	// SecretKey is the reCAPTCHA secret key issued by Google for your site.
	SecretKey string
	// MinScore is the minimum acceptable reCAPTCHA v3 score (0.0-1.0, defaults to 0.5 when <= 0).
	MinScore float64
	// Mock, when true, enables MockVerifier for local development and tests.
	Mock bool
}

// Client verifies reCAPTCHA tokens against Google's API.
type Client struct {
	secretKey  string
	httpClient *http.Client
	minScore   float64
	apiURL     string
}

// siteVerifyResponse is Google's siteverify JSON response shape.
type siteVerifyResponse struct {
	Success     bool      `json:"success"`
	Score       float64   `json:"score"`
	Action      string    `json:"action"`
	ChallengeTS time.Time `json:"challenge_ts"`
	Hostname    string    `json:"hostname"`
	ErrorCodes  []string  `json:"error-codes"`
}

// NewClient creates a new reCAPTCHA Client instance.
//
// Usage:
//
//	c := recaptcha.NewClient(recaptcha.Options{SecretKey: secret, MinScore: 0.5}, nil)
func NewClient(opts Options, httpClient *http.Client) *Client {
	minScore := opts.MinScore
	if minScore <= 0 {
		minScore = 0.5
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		secretKey:  opts.SecretKey,
		httpClient: httpClient,
		minScore:   minScore,
		apiURL:     "https://www.google.com/recaptcha/api/siteverify",
	}
}

// Verify implements Verifier by calling Google's siteverify endpoint.
func (c *Client) Verify(ctx context.Context, token string, remoteIP string) error {
	if c.secretKey == "" {
		return fmt.Errorf("recaptcha secret key not configured")
	}

	params := url.Values{}
	params.Set("secret", c.secretKey)
	params.Set("response", token)
	if remoteIP != "" {
		params.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.URL.RawQuery = params.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to recaptcha: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("recaptcha api returned non-200 status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var verifyResp siteVerifyResponse
	if err := json.Unmarshal(body, &verifyResp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !verifyResp.Success {
		return fmt.Errorf("recaptcha verification failed: %v", verifyResp.ErrorCodes)
	}
	if verifyResp.Score < c.minScore {
		return fmt.Errorf("recaptcha score too low: %f < %f", verifyResp.Score, c.minScore)
	}

	return nil
}
