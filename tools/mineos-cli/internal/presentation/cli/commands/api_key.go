package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/application/usecases"
)

func NewApiKeyCommand(loadConfig *usecases.LoadConfigUseCase) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api-key",
		Short: "Manage MineOS API keys",
	}

	cmd.AddCommand(NewApiKeyRefreshCommand(loadConfig))
	return cmd
}

func NewApiKeyRefreshCommand(loadConfig *usecases.LoadConfigUseCase) *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Rewrite MINEOS_API_KEY from the key material in .env",
		Long: "Rebuilds MINEOS_API_KEY from ApiKey__StaticKey or ApiKey__SeedKey in the\n" +
			"same .env file.\n\n" +
			"This used to read the key out of the sqlite database. The API stores only a\n" +
			"SHA-256 of each key now, so there is nothing there to read back -- a key that\n" +
			"is not in .env has to be reissued rather than recovered.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig.Execute(cmd.Context())
			if err != nil {
				return err
			}
			key, source, err := refreshApiKeyFromEnv(cfg)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "MINEOS_API_KEY rewritten from %s.\n", source)
			fmt.Fprintf(cmd.OutOrStdout(), "MINEOS_API_KEY: %s\n", mask(key))
			return nil
		},
	}
}
