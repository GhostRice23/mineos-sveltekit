package commands

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/application/usecases"
	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/infrastructure/api"
)

func NewHealthCommand(loadConfig *usecases.LoadConfigUseCase) *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check MineOS API health",
		Long: "Check MineOS API health.\n\n" +
			"With --all, report the consolidated roll-up: watchdog state, crash counts\n" +
			"and the most recent crash per server, restart attempts and cooldowns, plus\n" +
			"unread alerts (including the API's automatic low-TPS warnings).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := context.Background()
			cfg, err := loadConfig.Execute(ctx)
			if err != nil {
				return err
			}
			client := api.NewClientFromConfig(cfg)

			if !all {
				if err := usecases.NewHealthCheckUseCase(client).Execute(ctx); err != nil {
					return err
				}
				cmd.Println("OK")
				return nil
			}

			rollup := usecases.NewHealthRollupUseCase(client).Execute(ctx)
			RenderHealthRollup(cmd.OutOrStdout(), rollup, time.Now())

			// An unreachable API is a failed check, not an empty report — exit
			// non-zero so scripts and monitors notice.
			if !rollup.ApiReachable {
				return rollup.ApiError
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false,
		"report the full health roll-up (watchdog, crashes, restarts, alerts)")

	return cmd
}
