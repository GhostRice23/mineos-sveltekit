package config

import "testing"

func TestEffectiveApiKeyPrefersTheManagementKey(t *testing.T) {
	cfg := Config{
		ManagementApiKey: "management",
		ApiKeyStatic:     "static",
		ApiKeySeed:       "seed",
	}

	if got := cfg.EffectiveApiKey(); got != "management" {
		t.Errorf("got %q, want the management key", got)
	}
}

func TestEffectiveApiKeyFallsBackInOrder(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"static when no management key", Config{ApiKeyStatic: "static", ApiKeySeed: "seed"}, "static"},
		{"seed when neither is set", Config{ApiKeySeed: "seed"}, "seed"},
		{"empty when nothing is configured", Config{}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.EffectiveApiKey(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsPreReleaseEnabledOnlyForAnExplicitTrue(t *testing.T) {
	// Anything but "true" keeps the user on stable — an unset or malformed
	// value must not silently opt them into preview builds.
	for _, value := range []string{"", "false", "TRUE", "1", "yes", "maybe"} {
		if (Config{PreReleaseUpdates: value}).IsPreReleaseEnabled() {
			t.Errorf("PreReleaseUpdates=%q enabled pre-releases", value)
		}
	}
	if !(Config{PreReleaseUpdates: "true"}).IsPreReleaseEnabled() {
		t.Error(`PreReleaseUpdates="true" should enable pre-releases`)
	}
}

func TestIsTelemetryEnabledDefaultsOnAndOptsOutOnlyForFalse(t *testing.T) {
	// Telemetry is opt-out, so an unset value counts as enabled.
	for _, value := range []string{"", "true", "anything"} {
		if !(Config{TelemetryEnabled: value}).IsTelemetryEnabled() {
			t.Errorf("TelemetryEnabled=%q disabled telemetry", value)
		}
	}
	if (Config{TelemetryEnabled: "false"}).IsTelemetryEnabled() {
		t.Error(`TelemetryEnabled="false" should disable telemetry`)
	}
}

func TestEffectiveTelemetryEndpointFallsBackToProduction(t *testing.T) {
	if got := (Config{}).EffectiveTelemetryEndpoint(); got != "https://mineos.net" {
		t.Errorf("got %q, want the production endpoint", got)
	}
	if got := (Config{TelemetryEndpoint: "http://localhost:9000"}).EffectiveTelemetryEndpoint(); got != "http://localhost:9000" {
		t.Errorf("got %q, want the configured endpoint", got)
	}
}
