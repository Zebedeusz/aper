package apiclient

type Chain string

var (
	ETH       Chain = "ETHEREUM"
	MATIC     Chain = "MATIC"
	ARBITRUM  Chain = "ARBITRUM"
	AVALANCHE Chain = "AVALANCHE"
	FANTOM    Chain = "FANTOM"
)

var Chains = map[Chain]string{ETH: "1", MATIC: "137", ARBITRUM: "42161", AVALANCHE: "43114", FANTOM: "250"}

var CoingeckoPlatforms = map[Chain]string{ETH: "ethereum", MATIC: "polygon-pos", ARBITRUM: "arbitrum-one", AVALANCHE: "avalanche", FANTOM: "fantom"}
