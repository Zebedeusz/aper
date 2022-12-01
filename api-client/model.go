package apiclient

type Chain string

var (
	ETH   Chain = "1"
	MATIC Chain = ""
)

type RequestEntity string

var (
	HOLDERS  RequestEntity = "token_holders"
	BALANCES RequestEntity = "balances_v2"
)

type Holder struct {
}

type Balance struct {
}

var Chains = map[string]string{"ETH": "1", "MATIC": ""}
