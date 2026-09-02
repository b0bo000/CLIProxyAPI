package helps

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/tidwall/gjson"
)

// ClaudeSoftwareProfileProvenance identifies the authority used to resolve a
// request's software identity. It is deliberately separate from Confirmed:
// configured policy is not evidence that the caller is a native client.
type ClaudeSoftwareProfileProvenance string

const (
	ClaudeSoftwareProfileDetected      ClaudeSoftwareProfileProvenance = "detected"
	ClaudeSoftwareProfileConfiguredCLI ClaudeSoftwareProfileProvenance = "configured-cli"
	ClaudeSoftwareProfileUnknown       ClaudeSoftwareProfileProvenance = "unknown"
)

// ResolvedClaudeSoftwareProfile joins the persistent device tuple with the
// request-scoped native-client detection result. Dynamic request/body fields
// remain outside this value.
type ResolvedClaudeSoftwareProfile struct {
	Device          ClaudeDeviceProfile
	Entrypoint      string
	Subclient       string
	AgentSDKVersion string
	Confirmed       bool
	Provenance      ClaudeSoftwareProfileProvenance
	helperProfile   bool
}

// IsHelperProfile reports whether the detector matched one of the measured
// native Haiku helper request shapes. It stays private to the resolver's
// authority: callers cannot assert helper status by setting an HTTP field.
func (p ResolvedClaudeSoftwareProfile) IsHelperProfile() bool {
	return p.helperProfile
}

// ResolveClaudeSoftwareProfile resolves one request's software identity. The
// configuredCLI argument is an internal policy result, never a value derived
// from an HTTP header, session ID, credential or request body.
//
// A confirmed native request keeps the detector's entrypoint provenance. An
// explicit CLI fingerprint policy gets the configured device tuple and the
// configured CLI entrypoint, but remains unconfirmed. Unknown callers retain
// only the configured baseline device tuple and an unknown entrypoint.
func ResolveClaudeSoftwareProfile(
	ctx context.Context,
	auth *cliproxyauth.Auth,
	apiKey string,
	headers http.Header,
	payload []byte,
	countTokens bool,
	cfg *config.Config,
	configuredCLI bool,
) (ResolvedClaudeSoftwareProfile, error) {
	detection := DetectClaudeCodeRequest(headers, payload, countTokens, cfg)
	resolved := ResolvedClaudeSoftwareProfile{
		Device:        defaultClaudeDeviceProfile(cfg),
		Confirmed:     detection.Confirmed,
		Provenance:    ClaudeSoftwareProfileUnknown,
		helperProfile: detection.HelperProfile,
	}

	if detection.Confirmed {
		resolved.Entrypoint = strings.TrimSpace(detection.Entrypoint)
		resolved.Subclient = strings.TrimSpace(detection.Subclient)
		resolved.AgentSDKVersion = strings.TrimSpace(detection.AgentSDKVersion)
		resolved.Provenance = ClaudeSoftwareProfileDetected
		if candidate, ok := extractClaudeDeviceProfile(headers, cfg); ok {
			resolved.Device = candidate
		}
		if ClaudeDeviceProfileStabilizationEnabled(cfg) {
			profile, errProfile := ResolveClaudeDeviceProfileRequired(ctx, auth, apiKey, headers, cfg)
			if errProfile != nil {
				return ResolvedClaudeSoftwareProfile{}, errProfile
			}
			resolved.Device = profile
		}
		if deviceEntrypoint, _ := parseClaudeCodeUserAgentDetails(resolved.Device.UserAgent); deviceEntrypoint != "" && deviceEntrypoint != resolved.Entrypoint {
			return ResolvedClaudeSoftwareProfile{}, fmt.Errorf("resolved device entrypoint %q conflicts with detected entrypoint %q", deviceEntrypoint, resolved.Entrypoint)
		}
		return resolved, nil
	}

	if configuredCLI {
		resolved.Entrypoint = "cli"
		if entrypoint, _ := parseClaudeCodeUserAgentDetails(resolved.Device.UserAgent); nativeClaudeEntrypoints[entrypoint] {
			resolved.Entrypoint = entrypoint
		}
		resolved.Subclient = claudeCodeSubclientByEntrypoint[resolved.Entrypoint]
		resolved.Provenance = ClaudeSoftwareProfileConfiguredCLI
	}
	return resolved, nil
}

func validateClaudeBillingSoftwareIdentity(payload []byte, profile ResolvedClaudeSoftwareProfile, cfg *config.Config) error {
	billing := gjson.GetBytes(payload, "system.0.text")
	if billing.Type != gjson.String || !strings.HasPrefix(billing.String(), "x-anthropic-billing-header:") {
		return nil
	}
	version, entrypoint, errParse := parseClaudeBillingSoftwareIdentity(billing.String())
	if errParse != nil {
		return errParse
	}
	expectedVersion := ClaudeDeviceProfileVersion(profile.Device, cfg)
	if version != expectedVersion && !strings.HasPrefix(version, expectedVersion+".") {
		return fmt.Errorf("Claude billing cc_version %q conflicts with resolved software version %q", version, expectedVersion)
	}
	if entrypoint != profile.Entrypoint {
		return fmt.Errorf("Claude billing cc_entrypoint %q conflicts with resolved entrypoint %q", entrypoint, profile.Entrypoint)
	}
	return nil
}

// ValidateClaudeBillingSoftwareIdentity checks the finished upstream body
// against the resolved software authority. Callers should run it after all
// translation and payload rules, immediately before CCH finalization.
func ValidateClaudeBillingSoftwareIdentity(payload []byte, profile ResolvedClaudeSoftwareProfile, cfg *config.Config) error {
	if profile.Provenance == ClaudeSoftwareProfileUnknown {
		return nil
	}
	return validateClaudeBillingSoftwareIdentity(payload, profile, cfg)
}

func parseClaudeBillingSoftwareIdentity(billing string) (version, entrypoint string, err error) {
	for _, segment := range strings.Split(strings.TrimPrefix(billing, "x-anthropic-billing-header:"), ";") {
		key, value, found := strings.Cut(strings.TrimSpace(segment), "=")
		if !found {
			continue
		}
		switch key {
		case "cc_version":
			if version != "" {
				return "", "", fmt.Errorf("Claude billing header contains duplicate cc_version")
			}
			version = strings.TrimSpace(value)
		case "cc_entrypoint":
			if entrypoint != "" {
				return "", "", fmt.Errorf("Claude billing header contains duplicate cc_entrypoint")
			}
			entrypoint = strings.TrimSpace(value)
		}
	}
	if version == "" || entrypoint == "" {
		return "", "", fmt.Errorf("Claude billing header is missing software identity fields")
	}
	return version, entrypoint, nil
}
