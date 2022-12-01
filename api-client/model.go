package apiclient

type Chain string

var (
	ETH   Chain = "1"
	MATIC Chain = ""
)

var Chains = map[Chain]string{"ETH": "1", "MATIC": ""}
