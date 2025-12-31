package command

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Common coin aliases
var coinAliases = map[string]string{
	"btc":     "bitcoin",
	"eth":     "ethereum",
	"uni":     "uniswap",
	"sol":     "solana",
	"ada":     "cardano",
	"dot":     "polkadot",
	"matic":   "matic-network",
	"link":    "chainlink",
	"avax":    "avalanche-2",
	"atom":    "cosmos",
	"xrp":     "ripple",
	"doge":    "dogecoin",
	"shib":    "shiba-inu",
	"ltc":     "litecoin",
	"bch":     "bitcoin-cash",
	"xlm":     "stellar",
	"algo":    "algorand",
	"vet":     "vechain",
	"fil":     "filecoin",
	"trx":     "tron",
	"etc":     "ethereum-classic",
	"xmr":     "monero",
	"aave":    "aave",
	"mkr":     "maker",
	"comp":    "compound-governance-token",
	"snx":     "havven",
	"crv":     "curve-dao-token",
	"sushi":   "sushi",
	"yfi":     "yearn-finance",
	"1inch":   "1inch",
	"grt":     "the-graph",
	"ens":     "ethereum-name-service",
	"ldo":     "lido-dao",
	"arb":     "arbitrum",
	"op":      "optimism",
	"pepe":    "pepe",
	"wbtc":    "wrapped-bitcoin",
	"steth":   "staked-ether",
	"usdt":    "tether",
	"usdc":    "usd-coin",
	"dai":     "dai",
	"busd":    "binance-usd",
}

func init() {
	Register(&Command{
		Name:        "price",
		Description: "Get cryptocurrency price",
		Usage:       "/price <coin>",
		Handler:     priceHandler,
	})
}

func priceHandler(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /price <coin> (e.g. /price btc, /price eth)", nil
	}

	coin := strings.ToLower(args[0])
	
	// Check alias
	if alias, ok := coinAliases[coin]; ok {
		coin = alias
	}

	// Call CoinGecko
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd&include_24hr_change=true", coin)
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "Error fetching price", err
	}
	defer resp.Body.Close()

	var result map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "Error parsing response", err
	}

	data, ok := result[coin]
	if !ok {
		return fmt.Sprintf("Coin '%s' not found", args[0]), nil
	}

	price := data["usd"]
	change := data["usd_24h_change"]

	// Format based on price magnitude
	var priceStr string
	if price < 0.01 {
		priceStr = fmt.Sprintf("$%.6f", price)
	} else if price < 1 {
		priceStr = fmt.Sprintf("$%.4f", price)
	} else if price < 100 {
		priceStr = fmt.Sprintf("$%.2f", price)
	} else {
		priceStr = fmt.Sprintf("$%.0f", price)
	}

	// Format change
	changeStr := ""
	if change != 0 {
		if change > 0 {
			changeStr = fmt.Sprintf(" (+%.1f%%)", change)
		} else {
			changeStr = fmt.Sprintf(" (%.1f%%)", change)
		}
	}

	return fmt.Sprintf("%s: %s%s", strings.ToUpper(args[0]), priceStr, changeStr), nil
}
