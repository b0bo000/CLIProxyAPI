package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	claudeauth "github.com/router-for-me/CLIProxyAPI/v7/internal/auth/claude"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

var statusPattern = regexp.MustCompile(`status\s+(\d{3})`)

type credentialEnvelope struct {
	ClaudeOAuth *credentialTokens `json:"claudeAiOauth"`
	credentialTokens
}

type credentialTokens struct {
	AccessToken      string `json:"accessToken"`
	AccessTokenSnake string `json:"access_token"`
}

type gateResult struct {
	OK                bool   `json:"ok"`
	HTTPStatus        int    `json:"http_status,omitempty"`
	ErrorClass        string `json:"error_class,omitempty"`
	ElapsedMillis     int64  `json:"elapsed_millis"`
	AccountUUIDSHA256 string `json:"account_uuid_sha256,omitempty"`
	RefreshAttempted  bool   `json:"refresh_attempted"`
}

func main() {
	if len(os.Args) != 2 {
		writeResult(gateResult{ErrorClass: "usage", RefreshAttempted: false})
		os.Exit(2)
	}
	accessToken, err := readAccessToken(os.Args[1])
	if err != nil {
		writeResult(gateResult{ErrorClass: "credential_input", RefreshAttempted: false})
		os.Exit(3)
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	profile, err := claudeauth.NewClaudeAuthWithProxyURL(&config.Config{}, "direct").FetchOAuthProfile(ctx, accessToken)
	result := gateResult{ElapsedMillis: time.Since(started).Milliseconds(), RefreshAttempted: false}
	if err != nil {
		result.HTTPStatus = errorStatus(err)
		if result.HTTPStatus != 0 {
			result.ErrorClass = fmt.Sprintf("http_%d", result.HTTPStatus)
		} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.ErrorClass = "timeout"
		} else {
			result.ErrorClass = "transport_or_parse"
		}
		writeResult(result)
		return
	}
	result.OK = true
	result.HTTPStatus = 200
	result.AccountUUIDSHA256 = hashString(profile.Account.UUID)
	writeResult(result)
}

func readAccessToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var envelope credentialEnvelope
	if err = json.Unmarshal(data, &envelope); err != nil {
		return "", err
	}
	tokens := envelope.credentialTokens
	if envelope.ClaudeOAuth != nil {
		tokens = *envelope.ClaudeOAuth
	}
	accessToken := strings.TrimSpace(tokens.AccessToken)
	if accessToken == "" {
		accessToken = strings.TrimSpace(tokens.AccessTokenSnake)
	}
	if !strings.HasPrefix(accessToken, "sk-ant-oat") {
		return "", errors.New("invalid access token")
	}
	return accessToken, nil
}

func errorStatus(err error) int {
	match := statusPattern.FindStringSubmatch(err.Error())
	if len(match) != 2 {
		return 0
	}
	var status int
	_, _ = fmt.Sscanf(match[1], "%d", &status)
	return status
}

func hashString(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func writeResult(result gateResult) {
	_ = json.NewEncoder(os.Stdout).Encode(result)
}
