package aper

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:          "ApeR",
	SilenceUsage: true,
}

func main() {
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))

	rootCmd.AddCommand(balancesOfTokensHolders)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
