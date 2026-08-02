package commands

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/application/usecases"
	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/domain/config"
	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/infrastructure/api"
)

func withApiKeyRetry(ctx context.Context, loadConfig *usecases.LoadConfigUseCase, out io.Writer, action func(config.Config, *api.Client) error) (bool, error) {
	cfg, err := loadConfig.Execute(ctx)
	if err != nil {
		return false, err
	}
	client := api.NewClientFromConfig(cfg)

	actionErr := action(cfg, client)
	if actionErr == nil {
		return false, nil
	} else if !errors.Is(actionErr, api.ErrApiKeyMissing) && !errors.Is(actionErr, api.ErrApiKeyInvalid) {
		return false, actionErr
	}

	// Recovers the common case: MINEOS_API_KEY went missing or stale while the
	// key material it is derived from is still in .env. If the key there is
	// simply wrong, this rewrites the same wrong value and the retry below
	// fails with that said plainly -- there is no database to fall back to,
	// because keys are stored hashed.
	key, source, refreshErr := refreshApiKeyFromEnv(cfg)
	if refreshErr != nil {
		return false, fmt.Errorf("%w (auto-refresh failed: %v)", actionErr, refreshErr)
	}

	if out != nil {
		fmt.Fprintf(out, "Rewrote MINEOS_API_KEY from %s.\n", source)
	}

	cfg, err = loadConfig.Execute(ctx)
	if err != nil {
		return true, err
	}
	client = api.NewClientFromConfig(cfg)
	if err := action(cfg, client); err != nil {
		if key != "" {
			return true, fmt.Errorf("%w (refreshed key did not resolve the issue)", err)
		}
		return true, err
	}
	return true, nil
}
