package nofxos

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// OIPosition represents open interest data for a single coin
type OIPosition struct {
	Symbol            string  `json:"symbol"`
	Rank              int     `json:"rank"`
	Price             float64 `json:"price"`
	CurrentOI         float64 `json:"current_oi"`
	OIDelta           float64 `json:"oi_delta"`
	OIDeltaPercent    float64 `json:"oi_delta_percent"`    // Already x100 (5.0 = 5%)
	OIDeltaValue      float64 `json:"oi_delta_value"`      // USDT value
	PriceDeltaPercent float64 `json:"price_delta_percent"` // Already x100 (5.0 = 5%)
	NetLong           float64 `json:"net_long"`
	NetShort          float64 `json:"net_short"`
}

// OIRankingResponse is the API response structure for OI ranking
type OIRankingResponse struct {
	Success bool `json:"success"`
	Code    int  `json:"code"`
	Data    struct {
		Positions      []OIPosition `json:"positions"`
		Count          int          `json:"count"`
		Exchange       string       `json:"exchange"`
		TimeRange      string       `json:"time_range"`
		TimeRangeParam string       `json:"time_range_param"`
		RankType       string       `json:"rank_type"`
		Limit          int          `json:"limit"`
	} `json:"data"`
}

// OIRankingData contains both top and low OI rankings
type OIRankingData struct {
	TimeRange    string       `json:"time_range"`
	Duration     string       `json:"duration"`
	TopPositions []OIPosition `json:"top_positions"`
	LowPositions []OIPosition `json:"low_positions"`
	FetchedAt    time.Time    `json:"fetched_at"`
}

// GetOIRanking retrieves OI ranking data (both top increase and low decrease)
func (c *Client) GetOIRanking(duration string, limit int) (*OIRankingData, error) {
	if duration == "" {
		duration = "1h"
	}
	if limit <= 0 {
		limit = 20
	}

	result := &OIRankingData{
		Duration:  duration,
		FetchedAt: time.Now(),
	}

	// Fetch top ranking (OI increase)
	topPositions, timeRange, err := c.fetchOIRanking("top", duration, limit)
	if err != nil {
		log.Printf("⚠️  Failed to fetch OI top ranking: %v", err)
	} else {
		result.TopPositions = topPositions
		result.TimeRange = timeRange
	}

	// Fetch low ranking (OI decrease)
	lowPositions, _, err := c.fetchOIRanking("low", duration, limit)
	if err != nil {
		log.Printf("⚠️  Failed to fetch OI low ranking: %v", err)
	} else {
		result.LowPositions = lowPositions
	}

	log.Printf("✓ Fetched OI ranking data: %d top, %d low (duration: %s)",
		len(result.TopPositions), len(result.LowPositions), duration)

	return result, nil
}

func (c *Client) fetchOIRanking(rankType, duration string, limit int) ([]OIPosition, string, error) {
	endpoint := fmt.Sprintf("/api/oi/%s-ranking?limit=%d&duration=%s", rankType, limit, duration)

	body, err := c.doRequest(endpoint)
	if err != nil {
		return nil, "", fmt.Errorf("request failed: %w", err)
	}

	var response OIRankingResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, "", fmt.Errorf("JSON parsing failed: %w", err)
	}

	// Check for success (support both success field and code field)
	if !response.Success && response.Code != 0 {
		return nil, "", fmt.Errorf("API returned error code: %d", response.Code)
	}

	return response.Data.Positions, response.Data.TimeRange, nil
}

// GetOITopPositionsWithDuration retrieves top OI increase positions with the specified duration.
func (c *Client) GetOITopPositionsWithDuration(duration string, limit int) ([]OIPosition, error) {
	positions, _, err := c.fetchOIRanking("top", duration, limit)
	if err != nil {
		return nil, err
	}
	return positions, nil
}

// GetOITopPositions retrieves top OI increase positions (legacy compatibility)
func (c *Client) GetOITopPositions() ([]OIPosition, error) {
	return c.GetOITopPositionsWithDuration("1h", 20)
}

// GetOITopSymbols retrieves OI top coin symbol list
func (c *Client) GetOITopSymbols() ([]string, error) {
	positions, err := c.GetOITopPositions()
	if err != nil {
		return nil, err
	}

	var symbols []string
	for _, pos := range positions {
		symbol := NormalizeSymbol(pos.Symbol)
		symbols = append(symbols, symbol)
	}

	return symbols, nil
}

// GetOILowPositionsWithDuration retrieves OI decrease positions with the specified duration.
func (c *Client) GetOILowPositionsWithDuration(duration string, limit int) ([]OIPosition, error) {
	positions, _, err := c.fetchOIRanking("low", duration, limit)
	if err != nil {
		return nil, err
	}
	return positions, nil
}

// GetOILowPositions retrieves OI decrease positions (for short opportunities)
func (c *Client) GetOILowPositions() ([]OIPosition, error) {
	return c.GetOILowPositionsWithDuration("1h", 20)
}

// GetOILowSymbols retrieves OI low coin symbol list
func (c *Client) GetOILowSymbols() ([]string, error) {
	positions, err := c.GetOILowPositions()
	if err != nil {
		return nil, err
	}

	var symbols []string
	for _, pos := range positions {
		symbol := NormalizeSymbol(pos.Symbol)
		symbols = append(symbols, symbol)
	}

	return symbols, nil
}

// FormatOIRankingForAI formats OI ranking data for AI consumption
func FormatOIRankingForAI(data *OIRankingData, lang Language) string {
	if data == nil {
		return ""
	}

	if lang == LangChinese {
		return formatOIRankingZH(data)
	}
	return formatOIRankingEN(data)
}

func formatOIRankingZH(data *OIRankingData) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## 持仓量变化排行 (%s)\n\n", data.Duration))

	if len(data.TopPositions) > 0 {
		sb.WriteString("### 持仓增加榜\n")
		sb.WriteString("资金流入趋势，可能表示趋势延续或新仓建立:\n\n")
		sb.WriteString("| 排名 | 币种 | 持仓变化(USDT) | OI变化% | 价格变化% |\n")
		sb.WriteString("|------|------|----------------|---------|----------|\n")
		for _, pos := range data.TopPositions {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %+.2f%% | %+.2f%% |\n",
				pos.Rank, pos.Symbol, formatValue(pos.OIDeltaValue),
				pos.OIDeltaPercent, pos.PriceDeltaPercent))
		}
		sb.WriteString("\n")
	}

	if len(data.LowPositions) > 0 {
		sb.WriteString("### 持仓减少榜\n")
		sb.WriteString("资金流出趋势，可能表示趋势反转或存量仓位平仓:\n\n")
		sb.WriteString("| 排名 | 币种 | 持仓变化(USDT) | OI变化% | 价格变化% |\n")
		sb.WriteString("|------|------|----------------|---------|----------|\n")
		for _, pos := range data.LowPositions {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %+.2f%% | %+.2f%% |\n",
				pos.Rank, pos.Symbol, formatValue(pos.OIDeltaValue),
				pos.OIDeltaPercent, pos.PriceDeltaPercent))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("**解读**:\n")
	sb.WriteString("- OI增 + 价涨 = 多头主导，通常代表新多开仓，趋势延续概率更高\n")
	sb.WriteString("- OI增 + 价跌 = 空头主导，通常代表新空开仓，下跌压力可能延续\n")
	sb.WriteString("- OI减 + 价涨 = 空头平仓，价格反弹可能更多来自回补而非新增买盘\n")
	sb.WriteString("- OI减 + 价跌 = 多头平仓，抛压集中释放后需结合后续承接判断是否接近阶段性底部\n\n")
	return sb.String()
}

func formatOIRankingEN(data *OIRankingData) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Open Interest Changes (%s)\n\n", data.Duration))

	if len(data.TopPositions) > 0 {
		sb.WriteString("### OI Increase Ranking\n")
		sb.WriteString("Capital inflow trend, which may indicate trend continuation or new positioning:\n\n")
		sb.WriteString("| Rank | Symbol | OI Change (USDT) | OI Change % | Price Change % |\n")
		sb.WriteString("|------|--------|------------------|-------------|----------------|\n")
		for _, pos := range data.TopPositions {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %+.2f%% | %+.2f%% |\n",
				pos.Rank, pos.Symbol, formatValue(pos.OIDeltaValue),
				pos.OIDeltaPercent, pos.PriceDeltaPercent))
		}
		sb.WriteString("\n")
	}

	if len(data.LowPositions) > 0 {
		sb.WriteString("### OI Decrease Ranking\n")
		sb.WriteString("Capital outflow trend, which may indicate trend reversal or position closing:\n\n")
		sb.WriteString("| Rank | Symbol | OI Change (USDT) | OI Change % | Price Change % |\n")
		sb.WriteString("|------|--------|------------------|-------------|----------------|\n")
		for _, pos := range data.LowPositions {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %+.2f%% | %+.2f%% |\n",
				pos.Rank, pos.Symbol, formatValue(pos.OIDeltaValue),
				pos.OIDeltaPercent, pos.PriceDeltaPercent))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("**Key**:\n")
	sb.WriteString("- OI up + Price up = Bulls dominant, often showing fresh long positioning and a higher chance of trend continuation\n")
	sb.WriteString("- OI up + Price down = Bears dominant, often showing fresh short positioning and sustained downside pressure\n")
	sb.WriteString("- OI down + Price up = Short covering, where the rebound may be driven more by covering than by new spot demand\n")
	sb.WriteString("- OI down + Price down = Long liquidation, where forced exits may signal washout but still require follow-through confirmation\n\n")
	return sb.String()
}
