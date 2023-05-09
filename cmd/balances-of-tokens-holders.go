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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cshields143/govalent/class_a"
	"github.com/pkg/errors"
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

	balancesOfTokensHolders.PersistentFlags().StringVar(&date, "date", "", "")
}

const (
	configPath            = "/Users/michal.gil/go/src/aper/config/config.yaml"
	coingeckoURL          = "https://www.coingecko.com/en/coins/%s"
	coingeckoCoinsListURL = "https://api.coingecko.com/api/v3/coins/list?include_platform=true"
	coingeckoCoinURL      = "https://api.coingecko.com/api/v3/coins/%s?localization=false&tickers=false&community_data=false&developer_data=false&sparkline=false"
	resultsPathTokens     = "./results/tokens"
	resultsPathWhales     = "./results/whales"
	dateFormat            = "2006-01-02"
)

var (
	cfg                                      config.Config
	tokenAddress                             string
	tokenSymbol                              string
	tokenChain                               string
	minTokenQnt                              int
	minHoldingUSDValueStr, whaleThresholdStr string
	minHoldingUSDValue, whaleThreshold       decimal.Decimal
	apiClient                                apiclient.APIClienter
	date                                     string
)

type coins struct {
	lock               *sync.RWMutex
	coingeckoTokensMap map[apiclient.Chain]map[string]*tokenInfo // chain to token symbol to token info
}

type whales struct {
	lock *sync.RWMutex
	list map[string]string // address to portfolio value
}

type holdings struct {
	lock *sync.RWMutex
	list map[string]decimal.Decimal // token symbol to quote to be able to sort
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

		var block *int
		if date != "" {
			dateTime, err := time.Parse(dateFormat, date)
			if err != nil {
				log.Fatalf("error parsing date %v: %v", date, err)
			}
			block, err = apiClient.GetBlockByDate(apiclient.GetBlockByDateReq{
				Chain: tokenChainC,
				Date:  dateTime,
			})
			log.Printf("block: %d", *block)
			if err != nil {
				log.Fatalf("error retrieving block by date: %v", err)
			}
			if block == nil {
				log.Fatal("no block found for given date")
			}
		}

		coins := coins{
			lock:               &sync.RWMutex{},
			coingeckoTokensMap: make(map[apiclient.Chain]map[string]*tokenInfo),
		}

		// saveCoingeckoTokensList(coins)
		// os.Exit(0)

		initCoingeckoTokensMap(coins)

		fmt.Printf("Retrieving holders...\n")
		holders, err := apiClient.GetTokenHolders(apiclient.Chain(tokenChainC), tokenAddress, block)
		if err != nil {
			log.Fatalf("error retrieving token holders for address %v: %v", tokenAddress, err)
		}
		fmt.Printf("Found %d holders\n", len(holders))
		if len(holders) == 0 {
			log.Fatal("Exiting...")
		}

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
				list: make(map[string]decimal.Decimal, 0),
			}

			for _, holder := range holders {
				if shouldSkipHolder(&holder) {
					continue
				}

				wg.Add(1)
				holderAddress := holder.Address

				go func() {
					processHolder(holderAddress, chain, holdings, whales, coins)
					wg.Done()
				}()
			}
			wg.Wait()
			saveFoundTokensInAFile(chain, holdings.list, coins)
		}
		saveFoundWhalesInAFile(whales.list)
		return nil
	},
}

func processHolder(holderAddress, chain string, holdings holdings, whales whales, coins coins) {
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
		if !minHoldingUSDValue.LessThanOrEqual(quote) {
			continue
		}
		skip, err := shouldSkipToken(chain, balance.ContractTickerSymbol, coins)
		if err != nil {
			fmt.Printf("error checking for token skip: %s\n", err)
			return
		}
		if skip {
			continue
		}

		holdings.lock.Lock()
		if v, ok := holdings.list[balance.ContractTickerSymbol]; ok {
			holdings.list[balance.ContractTickerSymbol] = v.Add(quote)
		} else {
			holdings.list[balance.ContractTickerSymbol] = quote
		}
		holdings.lock.Unlock()
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

type tokenInfo struct {
	ID          string
	Symbol      string
	MarketCap   decimal.Decimal
	GenesisDate string
}

func (t *tokenInfo) toCsvRow() []string {
	return []string{t.ID, t.Symbol, t.MarketCap.String(), t.GenesisDate}
}

func httpGetCoingeckoTokensList() ([]coingeckoCoin, error) {
	r, err := http.Get(coingeckoCoinsListURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failure retrieving coingecko coins list")
	}
	defer r.Body.Close()

	var coinsList []coingeckoCoin
	jsonDataFromHttp, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "failure reading response body")
	}
	if r.StatusCode != 200 {
		return nil, errors.Errorf("response status: %d; body: %s", r.StatusCode, string(jsonDataFromHttp))
	}
	if err := json.Unmarshal([]byte(jsonDataFromHttp), &coinsList); err != nil {
		return nil, errors.Wrapf(err, "failure unmarshalling response body")
	}

	if len(coinsList) == 0 {
		return nil, errors.New("empty coingecko list")
	}

	return coinsList, nil
}

func httpGetCoingeckoTokenInfo(tokenID string) (*tokenInfo, error) {
retry:
	r, err := http.Get(fmt.Sprintf(coingeckoCoinURL, tokenID))
	if err != nil {
		return nil, errors.Wrapf(err, "failure retrieving coingecko coin info")
	}
	defer r.Body.Close()

	if r.StatusCode == 429 {
		waitTime, err := time.ParseDuration(r.Header.Get("Retry-After") + "s")
		if err != nil {
			return nil, err
		}
		fmt.Println("coingecko rate limit reached. waiting " + r.Header.Get("Retry-After") + "s...")
		time.Sleep(waitTime)
		goto retry
	}

	var coinApiNativeInfo apiclient.CoingeckoCoinInfo
	jsonDataFromHttp, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "failure reading response body")
	}
	if err := json.Unmarshal([]byte(jsonDataFromHttp), &coinApiNativeInfo); err != nil {
		return nil, errors.Wrapf(err, "failure unmarshalling response body")
	}

	return &tokenInfo{
		ID:          coinApiNativeInfo.ID,
		Symbol:      coinApiNativeInfo.Symbol,
		GenesisDate: coinApiNativeInfo.GenesisDate,
		MarketCap:   coinApiNativeInfo.MarketData.MarketCap.USD,
	}, nil
}

func initCoingeckoTokensMap(coins coins) {
	fmt.Printf("Initializing coingecko tokens map...\n")

	coinsList, err := httpGetCoingeckoTokensList()
	if err != nil {
		log.Fatalf("failure getting coins list: %s", err.Error())
	}

	coins.lock.Lock()
	defer coins.lock.Unlock()

	for _, chain := range cfg.Chains {
		coins.coingeckoTokensMap[apiclient.Chain(chain)] = make(map[string]*tokenInfo)
	}

	for _, coin := range coinsList {
		if coin.ID == "" || coin.Symbol == "" {
			continue
		}

		if strings.Contains(coin.ID, "wormhole") {
			continue
		}

		tokenInfo := &tokenInfo{
			ID:     coin.ID,
			Symbol: coin.Symbol,
		}

		if _, ok := coins.coingeckoTokensMap[apiclient.ETH]; ok && len(coin.Platforms) == 0 {
			coins.coingeckoTokensMap[apiclient.ETH][coin.Symbol] = tokenInfo
			continue
		}
		for _, chain := range cfg.Chains {
			if _, ok := coin.Platforms[apiclient.CoingeckoPlatforms[apiclient.Chain(chain)]]; ok {
				coins.coingeckoTokensMap[apiclient.Chain(chain)][coin.Symbol] = tokenInfo
			}
		}
	}
	if len(coins.coingeckoTokensMap) == 0 {
		log.Fatalf("empty coingecko tokens map")
	}

	fmt.Printf("Tokens per chain in coingecko: ")
	for k, v := range coins.coingeckoTokensMap {
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

func saveFoundTokensInAFile(chain string, tokens map[string]decimal.Decimal, coins coins) {
	if len(tokens) == 0 {
		fmt.Printf("No tokens found for this chain\n")
		return
	}
	filename := fmt.Sprintf("tokens_%s_%s_%s.csv", tokenSymbol, chain, time.Now().Format("2006-01-02"))

	fmt.Printf("Found %d tokens. Saving results...\n", len(tokens))

	// sort tokens by quote quantity in descending order
	keys := make([]string, 0, len(tokens))
	for key := range tokens {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		return tokens[keys[i]].Cmp(tokens[keys[j]]) > 0
	})

	f, err := os.Create(fmt.Sprintf("%s/%s", resultsPathTokens, filename))
	if err != nil {
		log.Fatalln("failed to create file:", err)
	}

	w := csv.NewWriter(f)

	err = w.Write([]string{"symbol", "info"})
	if err != nil {
		log.Fatalln("error writing headers to csv file:", err)
	}

	coins.lock.RLock()
	for _, k := range keys {
		var coinID string
		if coinInfo, ok := coins.coingeckoTokensMap[apiclient.Chain(chain)][strings.ToLower(k)]; ok {
			coinID = coinInfo.ID
		}

		if err := w.Write([]string{k,
			fmt.Sprintf(coingeckoURL, coinID)}); err != nil {
			log.Fatalln("error writing tokens list to csv file:", err)
		}
	}
	coins.lock.RUnlock()
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

func shouldSkipToken(chain string, tokenSymbol string, coins coins) (bool, error) {
	coins.lock.Lock()
	defer coins.lock.Unlock()

	tokenInfo, ok := coins.coingeckoTokensMap[apiclient.Chain(chain)][strings.ToLower(tokenSymbol)]
	if !ok {
		return true, nil
	}

	if tokenInfo.ID == "" {
		return true, nil
	}

	if tokenInfo.MarketCap.Equals(decimal.Decimal{}) {
		coinGeckoTokenInfo, err := httpGetCoingeckoTokenInfo(tokenInfo.ID)
		if err != nil {
			return false, errors.Wrapf(err, "failure getting coin info for coin ID: %s", tokenInfo.ID)
		}

		coins.coingeckoTokensMap[apiclient.Chain(chain)][strings.ToLower(tokenSymbol)] = coinGeckoTokenInfo
		tokenInfo = coinGeckoTokenInfo

		fmt.Printf("symbol: %s, tokenInfo: %+v\n", tokenSymbol, tokenInfo)
	}

	// TO ADD:
	// - price metrics e.g. not an inverted exponential curve, not around ATH

	if tokenInfo.MarketCap.GreaterThan(decimal.NewFromInt(50000000)) {
		return true, nil
	}

	if tokenInfo.GenesisDate != "" {
		genesisDate, err := time.Parse("2006-01-02", tokenInfo.GenesisDate)
		if err != nil {
			return false, err
		}
		limitDate, _ := time.Parse("2006-01-02", "2022-04-01")
		if genesisDate.Before(limitDate) {
			return true, nil
		}
	}

	return false, nil
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

func saveCoingeckoTokensList(coins coins) {
	initCoingeckoTokensMap(coins)

	coins.lock.Lock()
	defer coins.lock.Unlock()

	f, err := os.Create("tokenslist.csv")
	if err != nil {
		log.Fatalln("failed to create file:", err)
	}

	w := csv.NewWriter(f)

	err = w.Write([]string{"id", "symbol", "marketcap", "genesis date"})
	if err != nil {
		log.Fatalln("error writing headers to csv file:", err)
	}

	for chain := range coins.coingeckoTokensMap {
		for coinSymbol := range coins.coingeckoTokensMap[chain] {
			tokenInfo := coins.coingeckoTokensMap[chain][coinSymbol]
			coinGeckoTokenInfo, err := httpGetCoingeckoTokenInfo(tokenInfo.ID)
			if err != nil {
				log.Fatalf("failure getting coin info for coin ID: %s, err: %s", tokenInfo.ID, err)
			}

			if err := w.Write(coinGeckoTokenInfo.toCsvRow()); err != nil {
				log.Fatalln("error writing token info to csv file:", err)
			}
			w.Flush()
		}
	}

	w.Flush()

	err = f.Close()
	if err != nil {
		log.Fatalln("error closing file:", err)
	}
}
