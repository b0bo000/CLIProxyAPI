package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadAccessTokenNestedAndSnakeCase(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "nested", body: `{"claudeAiOauth":{"accessToken":"sk-ant-oat01-nested"}}`, want: "sk-ant-oat01-nested"},
		{name: "flat snake", body: `{"access_token":"sk-ant-oat01-flat"}`, want: "sk-ant-oat01-flat"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "oauth.json")
			if err := os.WriteFile(path, []byte(test.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := readAccessToken(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("token = %q, want %q", got, test.want)
			}
		})
	}
}

func TestErrorStatus(t *testing.T) {
	if got := errorStatus(assertError("fetch profile failed with status 401")); got != 401 {
		t.Fatalf("status = %d, want 401", got)
	}
	if got := errorStatus(assertError("network unavailable")); got != 0 {
		t.Fatalf("status = %d, want 0", got)
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }
