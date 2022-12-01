package apiclient

type GetHoldersResponse struct {
	Holders []*Holder `json:"items"`
}

type Holder struct {
	Decimals string `json:"contract_decimals"`
	Address  string `json:"address"`
	Balance  string `json:"balance"`
}

type GetBalancesResponse struct {
	Balances []*Balance `json:"items"`
}

type Balance struct {
	Symbol  string `json:"contract_ticker_symbol"`
	Type    string `json:"type"`
	Balance string `json:"quote"`
}
