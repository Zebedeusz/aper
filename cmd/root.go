package cmd

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:          "ApeR",
	SilenceUsage: true,
}

func Execute() {
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))

	rootCmd.AddCommand(balancesOfTokensHolders)

	// TODO
	// whales watching:
	// - read addresses from the CSV file - input: CSV file
	// - for each address: get transactions since date D - input: lookup date
	// - list sold and bought tokens per address, if sold: percentage of holding of these tokens that were sold

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
