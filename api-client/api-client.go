package apiclient

import (
	"aper/config"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/cshields143/govalent"
	"github.com/cshields143/govalent/class_a"
	"golang.org/x/time/rate"
)

type APIClienter interface {
	GetTokenHolders(chain Chain, token string, block *int) ([]class_a.Portfolio, error)
	GetAddressBalances(req GetAddressBalancesReq) ([]class_a.Portfolio, error)
	GetAddressBalancesRateLimited(ctx context.Context, req GetAddressBalancesReq) ([]class_a.Portfolio, error)
	GetBlockByDate(req GetBlockByDateReq) (*int, error)
}

type GetAddressBalancesReq struct {
	Chain   Chain
	Address string
}

type GetBlockByDateReq struct {
	Chain Chain
	Date  time.Time
}

func (c *ApiClient) GetAddressBalancesRateLimited(ctx context.Context, req GetAddressBalancesReq) ([]class_a.Portfolio, error) {
	if err := c.balancesReqsLimiter.Wait(ctx); err != nil {
		return nil, err
	}
	return c.GetAddressBalances(req)
}

type ApiClient struct {
	cfg                 config.Config
	balancesReqsLimiter *rate.Limiter
}

func NewAPIClient(cfg *config.Config) APIClienter {
	govalent.APIKey = cfg.ApiKey
	return &ApiClient{
		cfg:                 *cfg,
		balancesReqsLimiter: rate.NewLimiter(rate.Every(time.Millisecond*50), 1),
	}
}

func (c *ApiClient) GetTokenHolders(chain Chain, tokenAddress string, block *int) ([]class_a.Portfolio, error) {
	chainID, ok := Chains[chain]
	if !ok {
		return nil, errors.New("not supported chain")
	}

	params := class_a.TokenHoldersWithHeightParams{
		PageSize: 500,
	}
	if block != nil {
		params.BlockHeight = fmt.Sprint(*block)
	}

retry:
	portfolios, err := govalent.ClassA().TokenHolders(chainID, tokenAddress, params)
	if err != nil {
		if isAPITempError(err) {
			goto retry
		}
		return nil, err
	}

	return portfolios.Items, nil
}

func (c *ApiClient) GetAddressBalances(req GetAddressBalancesReq) ([]class_a.Portfolio, error) {
	chainID, ok := Chains[req.Chain]
	if !ok {
		return nil, errors.New("not supported chain")
	}

retry:
	if err := c.balancesReqsLimiter.Wait(context.Background()); err != nil {
		return nil, err
	}
	portfolios, err := govalent.ClassA().TokenBalances(chainID, req.Address, class_a.BalanceParams{
		Nft:        false,
		NoNftFetch: false,
	})
	if err != nil {
		if isAPITempError(err) || isRateLimitExceededError(err) {
			fmt.Printf("error retrieving balances: %s, retrying...\n", err)
			time.Sleep(time.Second / 2)
			goto retry
		}
		// if isRateLimitExceededError(err) {
		// 	if c.balancesReqsLimiter.Limit() < rate.Limit(time.Millisecond*200) {
		// 		c.balancesReqsLimiter.SetLimit(c.balancesReqsLimiter.Limit() * 2)
		// 		fmt.Printf("Rate limit modified to %v\n", c.balancesReqsLimiter.Limit())
		// 	}
		// 	time.Sleep(time.Second / 2)
		// 	goto retry
		// }
		return nil, err
	}

	return portfolios.Items, nil
}

func isRateLimitExceededError(err error) bool {
	return err.Error() == "Rate limit exceeded"
}

func isAPITempError(err error) bool {
	if err.Error() == "backend queue is full and cannot accept request" ||
		err.Error() == "invalid character '<' looking for beginning of value" ||
		err.Error() == "Database statement timeout exceeded" ||
		strings.Contains(err.Error(), "timeout") {
		return true
	}
	return false
}

func (c *ApiClient) GetBlockByDate(req GetBlockByDateReq) (*int, error) {
	date := req.Date.Format("2006-01-02")
	url := fmt.Sprintf("https://deep-index.moralis.io/api/v2/dateToBlock?chain=%s&date=%s", MoralisChain[req.Chain], date)

	r, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	r.Header.Add("Accept", "application/json")
	r.Header.Add("X-API-Key", c.cfg.MoralisApiKey)

	res, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	bodyMap := make(map[string]interface{})
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		return nil, err
	}

	block := int(bodyMap["block"].(float64))

	return &block, nil
}
