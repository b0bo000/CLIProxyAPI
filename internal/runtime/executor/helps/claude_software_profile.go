package helps

import (
	"context"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// ClaudeSoftwareProfileProvenance identifies the authority used to resolve a
// request's software identity. It is deliberately separate from Confirmed:
// configured policy is not evidence that the caller is a native client.
type ClaudeSoftwareProfileProvenance string

const (
	ClaudeSoftwareProfileDetected      ClaudeSoftwareProfileProvenance = "detected"
	ClaudeSoftwareProfileConfiguredCLI  ClaudeSoftwareProfileProvenance = "configured-cli"
	ClaudeSoftwareProfileUnknown        ClaudeSoftwareProfileProvenance = "unknown"
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
		Device:     defaultClaudeDeviceProfile(cfg),
		Confirmed:  detection.Confirmed,
		Provenance: ClaudeSoftwareProfileUnknown,
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
		return resolved, nil
	}

	if configuredCLI {
		resolved.Entrypoint = "cli"
		resolved.Subclient = claudeCodeSubclientByEntrypoint[resolved.Entrypoint]
		resolved.Provenance = ClaudeSoftwareProfileConfiguredCLI
	}
	return resolved, nil
}
