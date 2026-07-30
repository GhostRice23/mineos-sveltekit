package commands

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/application/usecases"
	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/infrastructure/env"
)

func NewReconfigureCommand(loadConfig *usecases.LoadConfigUseCase) *cobra.Command {
	return &cobra.Command{
		Use:   "reconfigure",
		Short: "Update MineOS configuration in .env",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReconfigure(cmd, loadConfig)
		},
	}
}

func runReconfigure(cmd *cobra.Command, loadConfig *usecases.LoadConfigUseCase) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	var reader *bufio.Reader // kept for function signature compatibility

	cfg, err := loadConfig.Execute(ctx)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("no .env found; run `mineos install` first")
		}
		return err
	}
	envPath := cfg.EnvPath
	if envPath == "" {
		envPath = ".env"
	}

	values, err := loadEnvValues(envPath)
	if err != nil {
		return err
	}

	currentAdmin := fallback(values["Auth__SeedUsername"], "admin")
	currentPass := values["Auth__SeedPassword"]
	currentHostDir := fallback(values["HOST_BASE_DIRECTORY"], defaultHostBaseDir)
	currentDataDir := fallback(values["Data__Directory"], defaultDataDir)
	currentApiPort := parseEnvInt(values["API_PORT"], defaultApiPort)
	currentWebPort := parseEnvInt(values["WEB_PORT"], defaultWebPort)

	currentOrigin := values["WEB_ORIGIN_PROD"]
	if currentOrigin == "" {
		currentOrigin = values["ORIGIN"]
	}
	if currentOrigin == "" {
		currentOrigin = fmt.Sprintf("http://localhost:%d", currentWebPort)
	}

	currentMinecraftHost := fallback(values["PUBLIC_MINECRAFT_HOST"], "localhost")
	currentBodySize := fallback(values["BODY_SIZE_LIMIT"], defaultBodySizeLimit)
	currentShutdownTimeout := parseEnvInt(values["MINEOS_SHUTDOWN_TIMEOUT"], defaultShutdownTimeout)
	currentCurseforge := values["CurseForge__ApiKey"]
	currentDiscord := values["Discord__WebhookUrl"]
	currentNetworkMode := fallback(values["MINEOS_NETWORK_MODE"], defaultNetworkMode)
	currentBuildFromSource := parseEnvBool(values["MINEOS_BUILD_FROM_SOURCE"])
	currentImageTag := fallback(values["MINEOS_IMAGE_TAG"], "latest")
	currentManagementKey := values["MINEOS_API_KEY"]
	currentTelemetry := values["MINEOS_TELEMETRY_ENABLED"] != "false" // default true
	currentPrerelease := parseEnvBool(values["MINEOS_CLI_PRERELEASE_UPDATES"])

	fmt.Fprintln(out, "MineOS reconfigure")
	fmt.Fprintln(out, "Press Enter to keep the current value.")

	adminUser, err := promptString(reader, out, "Admin username", currentAdmin)
	if err != nil {
		return err
	}

	adminPass, err := promptOptionalPassword(out, currentPass)
	if err != nil {
		return err
	}

	managementKey, err := promptOptionalValue(reader, out, "Management API key for this CLI", currentManagementKey)
	if err != nil {
		return err
	}

	hostDir, err := promptRelativePathWithCurrent(reader, out, "Local storage directory for Minecraft servers (relative)", currentHostDir)
	if err != nil {
		return err
	}

	dataDir, err := promptRelativePathWithCurrent(reader, out, "Database directory (relative)", currentDataDir)
	if err != nil {
		return err
	}

	apiPort, err := promptInt(reader, out, "API port", currentApiPort)
	if err != nil {
		return err
	}

	webPort, err := promptInt(reader, out, "Web UI port", currentWebPort)
	if err != nil {
		return err
	}

	webOrigin, err := promptString(reader, out, "Web UI origin", currentOrigin)
	if err != nil {
		return err
	}
	caddySite := deriveCaddySite(webOrigin)

	minecraftHost, err := promptString(reader, out, "Public Minecraft host", currentMinecraftHost)
	if err != nil {
		return err
	}

	bodySizeLimit, err := promptString(reader, out, "Web UI upload body size limit", currentBodySize)
	if err != nil {
		return err
	}

	shutdownTimeout, err := promptInt(reader, out, "Server shutdown timeout (seconds)", currentShutdownTimeout)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "LAN discovery requires host networking on Linux.")
	enableHostNetworking, err := promptYesNo(reader, out, "Enable host networking for LAN discovery", currentNetworkMode == "host")
	if err != nil {
		return err
	}
	networkMode := defaultNetworkMode
	if enableHostNetworking {
		if runtime.GOOS != "linux" {
			fmt.Fprintln(out, "Host networking is only supported on Linux. Using bridge mode.")
		} else {
			networkMode = "host"
		}
	}

	buildFromSource, err := promptYesNo(reader, out, "Build images from source instead of pulling", currentBuildFromSource)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Version to pull:")
	fmt.Fprintln(out, "- 'latest': Most recent stable version (recommended)")
	fmt.Fprintln(out, "- 'preview': Latest preview/pre-release version")
	fmt.Fprintln(out, "- Or specify a version tag like 'v1.0.0' to pin to a specific release")
	imageTag, err := promptString(reader, out, "Image tag to pull", currentImageTag)
	if err != nil {
		return err
	}

	curseforgeKey, err := promptOptionalValue(reader, out, "CurseForge API key (optional)", currentCurseforge)
	if err != nil {
		return err
	}

	discordWebhook, err := promptOptionalValue(reader, out, "Discord webhook URL (optional)", currentDiscord)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "")
	telemetryEnabled, err := promptYesNo(reader, out, "Enable anonymous telemetry", currentTelemetry)
	if err != nil {
		return err
	}

	prereleaseEnabled, err := promptYesNo(reader, out, "Enable pre-release CLI updates", currentPrerelease)
	if err != nil {
		return err
	}

	// Collected and written in a single atomic pass: reconfigure used to rewrite
	// the whole .env once per key, so an error or a crash partway through left
	// the install half-configured.
	updates := []env.KeyValue{{Key: "Auth__SeedUsername", Value: adminUser}}

	passwordChanged := false
	if strings.TrimSpace(adminPass) != "" {
		updates = append(updates, env.KeyValue{Key: "Auth__SeedPassword", Value: adminPass})
		if adminPass != currentPass {
			passwordChanged = true
			updates = append(updates, env.KeyValue{Key: "Auth__ForcePasswordReset", Value: "true"})
		}
	}
	if strings.TrimSpace(managementKey) != "" {
		updates = append(updates, env.KeyValue{Key: "MINEOS_API_KEY", Value: managementKey})
	}
	updates = append(updates,
		env.KeyValue{Key: "HOST_BASE_DIRECTORY", Value: hostDir},
		env.KeyValue{Key: "Data__Directory", Value: dataDir},
		env.KeyValue{Key: "API_PORT", Value: strconv.Itoa(apiPort)},
		env.KeyValue{Key: "WEB_PORT", Value: strconv.Itoa(webPort)},
		env.KeyValue{Key: "WEB_ORIGIN_PROD", Value: webOrigin},
		env.KeyValue{Key: "PUBLIC_API_BASE_URL", Value: webOrigin},
		env.KeyValue{Key: "ORIGIN", Value: webOrigin},
		env.KeyValue{Key: "CADDY_SITE", Value: caddySite},
		env.KeyValue{Key: "PUBLIC_MINECRAFT_HOST", Value: minecraftHost},
		env.KeyValue{Key: "BODY_SIZE_LIMIT", Value: bodySizeLimit},
		env.KeyValue{Key: "MINEOS_SHUTDOWN_TIMEOUT", Value: strconv.Itoa(shutdownTimeout)},
		env.KeyValue{Key: "MINEOS_NETWORK_MODE", Value: networkMode},
		env.KeyValue{Key: "MINEOS_BUILD_FROM_SOURCE", Value: strconv.FormatBool(buildFromSource)},
		env.KeyValue{Key: "MINEOS_IMAGE_TAG", Value: imageTag},
	)
	if strings.TrimSpace(curseforgeKey) != "" {
		updates = append(updates, env.KeyValue{Key: "CurseForge__ApiKey", Value: curseforgeKey})
	}
	if strings.TrimSpace(discordWebhook) != "" {
		updates = append(updates, env.KeyValue{Key: "Discord__WebhookUrl", Value: discordWebhook})
	}
	updates = append(updates,
		env.KeyValue{Key: "MINEOS_TELEMETRY_ENABLED", Value: strconv.FormatBool(telemetryEnabled)},
		env.KeyValue{Key: "MINEOS_CLI_PRERELEASE_UPDATES", Value: strconv.FormatBool(prereleaseEnabled)},
	)

	if err := envRepo(envPath).SetAll(updates); err != nil {
		return err
	}

	// Ensure any new env vars from newer versions are present with defaults
	if _, err := ensureEnvDefaults(envPath, out); err != nil {
		return err
	}

	fmt.Println("Configuration updated.")

	// Ask if user wants to restart services
	restartServices, err := promptYesNo(nil, nil, "Restart services now to apply changes", true)
	if err != nil {
		return err
	}

	if restartServices {
		fmt.Println("Recreating services to apply new configuration...")
		compose, composeErr := detectCompose()
		if composeErr != nil {
			fmt.Println("Warning: Could not detect docker compose. Please restart services manually:")
			fmt.Println("  docker compose up -d")
			return nil
		}

		// Run docker compose up -d to recreate containers with new env vars
		// (restart doesn't reload environment variables)
		args := append([]string{}, compose.baseArgs...)
		args = append(args, "up", "-d")
		restartCmd := exec.Command(compose.exe, args...)
		restartCmd.Stdout = os.Stdout
		restartCmd.Stderr = os.Stderr
		if err := restartCmd.Run(); err != nil {
			return fmt.Errorf("failed to recreate services: %w", err)
		}
		fmt.Println("Services recreated successfully with new configuration.")

		// Clear the force password reset flag after services have started
		if passwordChanged {
			_ = setEnvFileValue(envPath, "Auth__ForcePasswordReset", "false")
			fmt.Println("Admin password has been reset.")
		}
	} else if passwordChanged {
		fmt.Println("Note: Password will be reset when services are restarted.")
	}

	return nil
}

func parseEnvInt(raw string, fallbackValue int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallbackValue
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallbackValue
	}
	return parsed
}

func parseEnvBool(raw string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
	return err == nil && parsed
}

func promptOptionalPassword(out io.Writer, current string) (string, error) {
	value, err := promptPassword(out, "Admin password (leave blank to keep current): ")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return current, nil
	}
	return value, nil
}

func promptOptionalValue(_ *bufio.Reader, _ io.Writer, label, current string) (string, error) {
	if current != "" {
		fmt.Printf("%s (leave blank to keep current): ", label)
	} else {
		fmt.Printf("%s (optional): ", label)
	}

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return current, nil
	}
	line := strings.TrimSpace(scanner.Text())
	if line == "" {
		return current, nil
	}
	return line, nil
}

func promptRelativePathWithCurrent(reader *bufio.Reader, out io.Writer, label, current string) (string, error) {
	for {
		value, err := promptString(reader, out, label, current)
		if err != nil {
			return "", err
		}
		value = strings.TrimSpace(value)
		if value == "" || value == current {
			return current, nil
		}
		if isValidRelativePath(value) {
			if !strings.HasPrefix(value, "./") && !strings.HasPrefix(value, ".\\") {
				value = "./" + value
			}
			return value, nil
		}
		fmt.Println("Path must be relative to the current directory (no leading /, ~, or ..).")
	}
}
