package apiclient

import (
	"aper/config"
	"errors"

	"github.com/AlchemistsLab/govalent"
	"github.com/AlchemistsLab/govalent/class_a"
)

type APIClienter interface {
	GetTokenHolders(chain Chain, token string) ([]class_a.Portfolio, error)
	GetAddressBalances(chain Chain, address string) ([]class_a.Portfolio, error)
}

// func composeApiUrl(cfg config.Config, chain Chain, reqEntity RequestEntity, entity string) string {
// 	return fmt.Sprintf("%s/%s/")
// }

type ApiClient struct {
	cfg config.Config
}

func NewAPIClient(cfg *config.Config) APIClienter {
	govalent.APIKey = cfg.ApiKey
	return &ApiClient{
		cfg: *cfg,
	}
}

func (c *ApiClient) GetTokenHolders(chain Chain, tokenAddress string) ([]class_a.Portfolio, error) {
	chainID, ok := Chains[chain]
	if !ok {
		return nil, errors.New("not supported chain")
	}

	portfolios, err := govalent.ClassA().TokenHolders(chainID, tokenAddress)
	if err != nil {
		return nil, err
	}

	return portfolios.Items, nil
}

func (c *ApiClient) GetAddressBalances(chain Chain, address string) ([]class_a.Portfolio, error) {
	chainID, ok := Chains[chain]
	if !ok {
		return nil, errors.New("not supported chain")
	}

	portfolios, err := govalent.ClassA().TokenBalances(chainID, address, class_a.BalanceParams{
		Nft:        false,
		NoNftFetch: false,
	})
	if err != nil {
		return nil, err
	}

	return portfolios.Items, nil
}
