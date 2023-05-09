package apiclient

import (
	"github.com/shopspring/decimal"
)

type CoingeckoCoinInfo struct {
	ID          string                  `json:"id"`
	Symbol      string                  `json:"symbol"`
	GenesisDate string                  `json:"genesis_date"`
	MarketData  CoingeckoCoinMarketData `json:"market_data"`
}

type CoingeckoCoinMarketData struct {
	MarketCap CoingeckoCoinMarketCap `json:"market_cap"`
}

type CoingeckoCoinMarketCap struct {
	USD decimal.Decimal `json:"usd"`
}
