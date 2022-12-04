package apiclient

import (
	"aper/config"
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cshields143/govalent"
	"github.com/cshields143/govalent/class_a"
	"golang.org/x/time/rate"
)

type APIClienter interface {
	GetTokenHolders(chain Chain, token string) ([]class_a.Portfolio, error)
	GetAddressBalances(req GetAddressBalancesReq) ([]class_a.Portfolio, error)
	GetAddressBalancesRateLimited(ctx context.Context, req GetAddressBalancesReq) ([]class_a.Portfolio, error)
}

type GetAddressBalancesReq struct {
	Chain   Chain
	Address string
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
		balancesReqsLimiter: rate.NewLimiter(rate.Every(time.Millisecond*20), 1),
	}
}

func (c *ApiClient) GetTokenHolders(chain Chain, tokenAddress string) ([]class_a.Portfolio, error) {
	chainID, ok := Chains[chain]
	if !ok {
		return nil, errors.New("not supported chain")
	}

retry:
	portfolios, err := govalent.ClassA().TokenHolders(chainID, tokenAddress, class_a.PaginateParams{
		PageSize: 300,
	})
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
