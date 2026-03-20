package store

func netPnL(realizedPnL, fee float64) float64 {
	return realizedPnL - fee
}
