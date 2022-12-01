package apiclient

import "aper/config"

type APIClienter interface {
	GetTokenHolders(chain Chain, token string) ([]*Holder, error)
	GetAddressBalances(chain Chain, address string) ([]*Balance, error)
}

// func composeApiUrl(cfg config.Config, chain Chain, reqEntity RequestEntity, entity string) string {
// 	return fmt.Sprintf("%s/%s/")
// }

type apiClient struct {
	cfg config.Config
}

func NewAPIClient(cfg *config.Config) APIClienter {
	return &apiClient{
		cfg: *cfg,
	}
}

func (c *apiClient) GetTokenHolders(chain Chain, token string) ([]*Holder, error) {
	return nil, nil
}

func (c *apiClient) GetAddressBalances(chain Chain, address string) ([]*Balance, error) {
	return nil, nil
}
