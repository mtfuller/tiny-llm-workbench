package cmd

import (
	"os"

	"github.com/mtfuller/tiny-llm-workbench/internal/color"
	"github.com/mtfuller/tiny-llm-workbench/internal/logger"
	"github.com/mtfuller/tiny-llm-workbench/internal/version"
	"github.com/spf13/cobra"
)

var (
	verbose  bool
	logLevel string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tlw",
	Short: "Tiny LLM Workbench — train, run, and evaluate agents backed by tiny LLMs",
	Long: color.Bold("tlw") + ` is the CLI for Tiny LLM Workbench (TLW), a local tool for training,
running, and evaluating agents backed by tiny LLMs, via a local webserver and browser UI.

Run ` + color.Bold("tlw serve") + ` to start the webserver and open the browser UI. See README.md
for the full feature list.`,
	Version: version.Version,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Configure logger based on flags
		if verbose {
			logger.SetLevel(logger.DEBUG)
		} else {
			logger.SetLevel(logger.ParseLogLevel(logLevel))
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		color.Error("Error: %v", err)
		os.Exit(1)
	}
}

func init() {
	// `tlw --version` prints just the version; `tlw version` still gives the
	// fuller commit/build-date breakdown.
	rootCmd.SetVersionTemplate("tlw {{.Version}}\n")

	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output (debug level)")
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info", "set log level (debug, info, warn, error)")
}
