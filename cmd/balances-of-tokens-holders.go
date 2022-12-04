package cmd

import (
	apiclient "aper/api-client"
	"aper/config"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cshields143/govalent/class_a"
	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	balancesOfTokensHolders.PersistentFlags().StringVar(&tokenAddress, "tokenAddress", "", "")
	_ = balancesOfTokensHolders.MarkPersistentFlagRequired("tokenAddress")

	balancesOfTokensHolders.PersistentFlags().StringVar(&tokenChain, "tokenChain", "", "")
	_ = balancesOfTokensHolders.MarkPersistentFlagRequired("tokenChain")

	balancesOfTokensHolders.PersistentFlags().IntVar(&minTokenQnt, "minTokenQnt", 100, "")
	_ = balancesOfTokensHolders.MarkPersistentFlagRequired("minTokenQnt")

	balancesOfTokensHolders.PersistentFlags().StringVar(&minHoldingUSDValueStr, "minHoldingUSDValue", "100", "")
	_ = balancesOfTokensHolders.MarkPersistentFlagRequired("minHoldingUSDValue")

	balancesOfTokensHolders.PersistentFlags().StringVar(&whaleThresholdStr, "whaleThreshold", "", "")
	_ = balancesOfTokensHolders.MarkPersistentFlagRequired("whaleThreshold")
}

const (
	configPath            = "/Users/michal.gil/go/src/aper/config/config.yaml"
	coingeckoURL          = "https://www.coingecko.com/en/coins/%s"
	coingeckoCoinsListURL = "https://api.coingecko.com/api/v3/coins/list?include_platform=true"
	resultsPathTokens     = "./results/tokens"
	resultsPathWhales     = "./results/whales"
)

var (
	cfg                                      config.Config
	tokenAddress                             string
	tokenSymbol                              string
	tokenChain                               string
	minTokenQnt                              int
	minHoldingUSDValueStr, whaleThresholdStr string
	minHoldingUSDValue, whaleThreshold       decimal.Decimal
	coingeckoTokensMap                       = make(map[apiclient.Chain]map[string]string) // chain to token symbol to coingecko token ID
	apiClient                                apiclient.APIClienter
)

// what do I need to know about a token:
// - website
// - total supply / max supply
// - all time price chart

type whales struct {
	lock *sync.RWMutex
	list map[string]string // address to portfolio value
}

type holdings struct {
	lock *sync.RWMutex
	list map[string]struct{} // token symbol
}

var balancesOfTokensHolders = &cobra.Command{
	Use:   "balancesOfTokensHolders",
	Short: "Retrieve current holders of a token",
	RunE: func(cmd *cobra.Command, args []string) error {
		initConfig(configPath)

		var err error
		minHoldingUSDValue, err = decimal.NewFromString(minHoldingUSDValueStr)
		if err != nil {
			log.Fatalf("error parsing minimal holding value %v: %v", minHoldingUSDValueStr, err)
		}
		whaleThreshold, err = decimal.NewFromString(whaleThresholdStr)
		if err != nil {
			log.Fatalf("error parsing whale threshold %v: %v", whaleThresholdStr, err)
		}

		apiClient = apiclient.NewAPIClient(&cfg)

		tokenChainC := apiclient.Chain(tokenChain)

		go initCoingeckoTokensMap()

		fmt.Printf("Retrieving holders...\n")
		holders, err := apiClient.GetTokenHolders(apiclient.Chain(tokenChainC), tokenAddress)
		if err != nil {
			log.Fatalf("error retrieving token holders for address %v: %v", tokenAddress, err)
		}
		fmt.Printf("Found %d holders\n", len(holders))

		tokenSymbol = holders[0].ContractTickerSymbol

		whales := whales{
			lock: &sync.RWMutex{},
			list: make(map[string]string, 0),
		}

		var wg sync.WaitGroup
		for _, chain := range cfg.Chains {
			fmt.Printf("Processing %s chain...\n", chain)

			holdings := holdings{
				lock: &sync.RWMutex{},
				list: make(map[string]struct{}, 0),
			}

			for _, holder := range holders {
				if shouldSkipHolder(&holder) {
					continue
				}

				wg.Add(1)
				holderAddress := holder.Address

				go func() {
					processHolder(holderAddress, chain, holdings, whales)
					wg.Done()
				}()
			}
			wg.Wait()
			saveFoundTokensInAFile(chain, holdings.list)
		}
		saveFoundWhalesInAFile(whales.list)
		return nil
	},
}

func processHolder(holderAddress, chain string, holdings holdings, whales whales) {
	balances, err := apiClient.GetAddressBalances(apiclient.GetAddressBalancesReq{
		Chain:   apiclient.Chain(chain),
		Address: holderAddress,
	})
	if err != nil {
		fmt.Printf("error retrieving balances for chain %v, address: %v; %v\n", chain, holderAddress, err)
		return
	}

	portfolioValue := decimal.NewFromInt(0)
	for _, balance := range balances {
		quote := decimal.NewFromFloat(balance.Quote)
		portfolioValue = portfolioValue.Add(quote)

		if shouldSkipBalance(&balance) {
			continue
		}
		if minHoldingUSDValue.LessThanOrEqual(quote) {
			holdings.lock.Lock()
			holdings.list[balance.ContractTickerSymbol] = struct{}{}
			holdings.lock.Unlock()
		}
	}
	if !portfolioValue.LessThan(whaleThreshold) {
		whales.lock.Lock()
		whales.list[holderAddress] = shortValue(portfolioValue)
		whales.lock.Unlock()
	}
}

type coingeckoCoin struct {
	ID        string            `json:"id"`
	Symbol    string            `json:"symbol"`
	Platforms map[string]string `json:"platforms"`
}

func initCoingeckoTokensMap() {
	fmt.Printf("Initializing coingecko tokens map...\n")

	r, err := http.Get(coingeckoCoinsListURL)
	if err != nil {
		log.Fatalf("failure retrieving coingecko coins list: %s", err)
	}
	defer r.Body.Close()

	var coinsList []coingeckoCoin
	jsonDataFromHttp, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Fatalf("failure reading response body %s", err)
	}
	if err := json.Unmarshal([]byte(jsonDataFromHttp), &coinsList); err != nil {
		log.Fatalf("failure unmarshalling response body %s", err)
	}

	if len(coinsList) == 0 {
		log.Fatalf("empty coingecko list")
	}

	for _, chain := range cfg.Chains {
		coingeckoTokensMap[apiclient.Chain(chain)] = make(map[string]string)
	}

	for _, coin := range coinsList {
		if strings.Contains(coin.ID, "wormhole") {
			continue
		}
		if _, ok := coingeckoTokensMap[apiclient.ETH]; ok && len(coin.Platforms) == 0 {
			coingeckoTokensMap[apiclient.ETH][coin.Symbol] = coin.ID
			continue
		}
		for _, chain := range cfg.Chains {
			if _, ok := coin.Platforms[apiclient.CoingeckoPlatforms[apiclient.Chain(chain)]]; ok {
				coingeckoTokensMap[apiclient.Chain(chain)][coin.Symbol] = coin.ID
			}
		}
	}
	if len(coingeckoTokensMap) == 0 {
		log.Fatalf("empty coingecko tokens map")
	}

	fmt.Printf("tokens per chain in coingecko: ")
	for k, v := range coingeckoTokensMap {
		fmt.Printf("%s : %d, ", k, len(v))
	}
	fmt.Println()
}

func saveFoundWhalesInAFile(whales map[string]string) {
	if len(whales) == 0 {
		return
	}
	fmt.Printf("Found %d whales. Saving results...\n", len(whales))

	filename := fmt.Sprintf("whales_%s.csv", time.Now().Format("2006-01-02"))

	f, err := os.Create(fmt.Sprintf("%s/%s", resultsPathWhales, filename))
	if err != nil {
		log.Fatalln("failed to create file:", err)
	}

	w := csv.NewWriter(f)

	err = w.Write([]string{"address", "portfolio value"})
	if err != nil {
		log.Fatalln("error writing headers to csv file:", err)
	}

	for k, v := range whales {
		if err := w.Write([]string{k, v}); err != nil {
			log.Fatalln("error writing whales list to csv file:", err)
		}
	}
	w.Flush()

	err = f.Close()
	if err != nil {
		log.Fatalln("error closing csv file:", err)
	}
}

func saveFoundTokensInAFile(chain string, tokens map[string]struct{}) {
	if len(tokens) == 0 {
		fmt.Printf("No tokens found for this chain\n")
		return
	}
	filename := fmt.Sprintf("tokens_%s_%s_%s.csv", tokenSymbol, chain, time.Now().Format("2006-01-02"))

	fmt.Printf("Found %d tokens. Saving results...\n", len(tokens))

	f, err := os.Create(fmt.Sprintf("%s/%s", resultsPathTokens, filename))
	if err != nil {
		log.Fatalln("failed to create file:", err)
	}

	w := csv.NewWriter(f)

	err = w.Write([]string{"symbol", "info"})
	if err != nil {
		log.Fatalln("error writing headers to csv file:", err)
	}

	for k := range tokens {
		if err := w.Write([]string{k,
			fmt.Sprintf(coingeckoURL, coingeckoTokensMap[apiclient.Chain(chain)][strings.ToLower(k)])}); err != nil {
			log.Fatalln("error writing tokens list to csv file:", err)
		}
	}
	w.Flush()

	err = f.Close()
	if err != nil {
		log.Fatalln("error closing csv file:", err)
	}
}

func shouldSkipBalance(balance *class_a.Portfolio) bool {
	return balance.ContractAddress == tokenAddress || balance.Type == "dust"
}

func shouldSkipHolder(holder *class_a.Portfolio) bool {
	if holder.Address == "0x000000000000000000000000000000000000dead" {
		return true
	}

	holderBalance, err := decimal.NewFromString(holder.Balance)
	if err != nil {
		fmt.Println("got incorrect balance: " + holder.Balance)
		return true
	}
	holderBalance = holderBalance.Div(
		decimal.NewFromInt(int64(holder.ContractDecimals)).
			Mul(decimal.NewFromInt(10)))

	return holderBalance.LessThan(decimal.NewFromInt(int64(minTokenQnt)))
}

func shortValue(value decimal.Decimal) string {
	million := decimal.NewFromInt(1000000)
	thousand := decimal.NewFromInt(1000)
	one := decimal.NewFromInt(1)

	valueDivdByMillion := value.Div(million)
	if !valueDivdByMillion.LessThan(one) {
		return valueDivdByMillion.RoundCash(100).String() + "M"
	}
	return value.Div(thousand).RoundCash(100).String() + "K"
}

// func printOutput(chain string, holdings map[string]struct{}, whales map[string]string) {
// 	var holdingsStr strings.Builder
// 	holdingsStr.Grow(len(holdings) * 8)
// 	for k := range holdings {
// 		holdingsStr.WriteString(fmt.Sprintf("%s, ", k))
// 	}
// 	var whalesStr strings.Builder
// 	whalesStr.Grow(len(whales) * 8)
// 	for k, v := range whales {
// 		whalesStr.WriteString(fmt.Sprintf("%s:%s, ", k, v))
// 	}

// 	fmt.Println()
// 	fmt.Println()
// 	fmt.Println("******************************************************")
// 	fmt.Printf("Chain: %s", chain)
// 	fmt.Println()
// 	fmt.Printf("Tokens: %s", holdingsStr.String())
// 	fmt.Println()
// 	fmt.Printf("Whales: %s", whalesStr.String())
// 	fmt.Println()
// 	fmt.Println("******************************************************")
// 	fmt.Println()
// 	fmt.Println()
// }

func initConfig(cfgFilepath string) {
	viper.SetConfigFile(cfgFilepath)

	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	} else {
		log.Fatalf("error using config file %v: %v", viper.ConfigFileUsed(), err)
	}

	if err := viper.GetViper().Unmarshal(&cfg); err != nil {
		log.Fatalf("error unmarshalling config: %v", err)
	}
}
