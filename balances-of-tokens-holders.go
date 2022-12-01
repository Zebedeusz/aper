package aper

import (
	apiclient "aper/api-client"
	"aper/config"
	"fmt"
	"log"

	"github.com/mitchellh/mapstructure"
	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	balancesOfTokensHolders.PersistentFlags().StringVar(&tokenAddress, "tokenAddress", "", "")
	_ = balancesOfTokensHolders.MarkPersistentFlagRequired("tokenAddress")

	balancesOfTokensHolders.PersistentFlags().IntVar(&minTokenQnt, "minTokenQnt", 100, "")
	_ = balancesOfTokensHolders.MarkPersistentFlagRequired("minTokenQnt")

	balancesOfTokensHolders.PersistentFlags().StringVar(&minHoldingUSDValueStr, "minHoldingUSDValue", "100", "")
	_ = balancesOfTokensHolders.MarkPersistentFlagRequired("minHoldingUSDValue")

	balancesOfTokensHolders.PersistentFlags().StringVar(&whaleThresholdStr, "whaleThreshold", "", "")
	_ = balancesOfTokensHolders.MarkPersistentFlagRequired("whaleThreshold")
}

const (
	configPath = "./config.yaml"
)

var (
	cfg                   config.Config
	tokenAddress          string
	minTokenQnt           int
	minHoldingUSDValueStr string
	whaleThresholdStr     string
)

type statistics struct {
	holdersAnalyzed int
}

// TODO
// support 5 requests per second limit

// TODO
// handle pagination

var balancesOfTokensHolders = &cobra.Command{
	Use:   "balancesOfTokensHolders",
	Short: "Retrieve current holders of a token",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.SetConfigFile(configPath)
		if err := viper.ReadInConfig(); err != nil {
			log.Fatalf("error using config file %v: %v", viper.ConfigFileUsed(), err)
		}
		loadConfig(cfg)

		minHoldingUSDValue, err := decimal.NewFromString(minHoldingUSDValueStr)
		if err != nil {
			log.Fatalf("error parsing minimal holding value %v: %v", minHoldingUSDValueStr, err)
		}
		whaleThreshold, err := decimal.NewFromString(whaleThresholdStr)
		if err != nil {
			log.Fatalf("error parsing whale threshold %v: %v", whaleThresholdStr, err)
		}

		apiClient := apiclient.NewAPIClient(&cfg)

		// algorithm:
		// INPUT: token name, minimum token quantity of a holder, minumum holding USD value, whale threshold
		// OUTPUT: list of the names of their holdings
		// OUTPUT 2: whales list
		// for all of the chains:
		// 		GET all holders of a token from the API: https://api.covalenthq.com/v1/1/tokens/XXX/token_holders
		// 		for each holder that has more than MIN_TOKEN_QNT:
		// 			GET his balances from the API: https://api.covalenthq.com/v1/1/address/demo.eth/balances_v2
		//			for each of his holdings:
		//				if holding.name == token name -> skip
		//				if holding is dust -> skip
		//				if holding.ValueInUSD >= MIN_HOLDING_USD_VALUE -> add to the output list
		//				if value in USD of all holdings >= WHALE_THRESHOLD -> add to whales list

		holdings := make(map[string]struct{}, 0)
		whales := make(map[string]struct{}, 0)

		for _, chain := range cfg.Chains {
			holders, err := apiClient.GetTokenHolders(apiclient.Chain(chain), tokenAddress)
			if err != nil {
				log.Fatalf("error retrieving token holders for chain %v: %v", chain, err)
			}

			for _, holder := range holders {
				balances, err := apiClient.GetAddressBalances(apiclient.Chain(chain), holder.Address)
				if err != nil {
					log.Fatalf("error retrieving balances for chain %v, address: %v; %v", chain, holder.Address, err)
				}
			}
		}

		log.Println()
		fmt.Println()
		fmt.Println("******************************************************")
		fmt.Println("  ")
		fmt.Printf("  ")
		fmt.Println("******************************************************")
		fmt.Println()
		fmt.Println()
		return nil
	},
}

func loadConfig(cfg interface{}) {
	if err := viper.GetViper().Unmarshal(&cfg, func(c *mapstructure.DecoderConfig) { c.TagName = "yaml" }); err != nil {
		log.Fatalf("error loading config: %s", err)
	}
}
