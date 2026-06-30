package diagnostics

import (
	"os"
	"strings"
)

// envDisabled interprets DISABLE_DIAGNOSTICS by value. Disabled iff the
// normalized value is not one of "", "false", "0", "no", "off".
func envDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("DISABLE_DIAGNOSTICS")))
	switch v {
	case "", "false", "0", "no", "off":
		return false
	default:
		return true
	}
}

// resolveEndpoint returns the OTLP logs URL: DIAGNOSTICS_ENDPOINT or the default,
// with a single /v1/logs suffix.
func resolveEndpoint() string {
	base := DefaultDiagnosticsEndpoint
	if v := os.Getenv("DIAGNOSTICS_ENDPOINT"); v != "" {
		base = v
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1/logs") {
		return base
	}
	return base + "/v1/logs"
}

// resolveToken returns DIAGNOSTICS_TOKEN or the default shared token.
func resolveToken() string {
	if v := os.Getenv("DIAGNOSTICS_TOKEN"); v != "" {
		return v
	}
	return DefaultDiagnosticsToken
}
