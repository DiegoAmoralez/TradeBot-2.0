package analysis

import (
	"bot_trading/config"
	"bot_trading/internal/exchange"
	"context"
	"fmt"
)

// SpotStrategy holds configuration for spot trading strategy
type SpotStrategy struct {
	Config *config.Config
}

// NewSpotStrategy creates a new spot strategy instance
func NewSpotStrategy(cfg *config.Config) *SpotStrategy {
	return &SpotStrategy{Config: cfg}
}

// L0Filter - Universe selection (Liquidity, Spread, Histroy check implied by having data)
func (s *SpotStrategy) L0Filter(ticker exchange.TickerStats) (bool, string) {
	// Quote Volume check
	if ticker.QuoteVolume < s.Config.SpotMinQuoteVolume {
		return false, fmt.Sprintf("Low Volume (%.2fM < %.2fM)", ticker.QuoteVolume/1000000, s.Config.SpotMinQuoteVolume/1000000)
	}

	// Spread check (if available)
	// Calculate spread percentage: (Ask - Bid) / Ask * 100
	if ticker.AskPrice > 0 && ticker.BidPrice > 0 {
		spread := (ticker.AskPrice - ticker.BidPrice) / ticker.AskPrice * 100
		if spread > s.Config.SpotMaxSpread {
			return false, fmt.Sprintf("High Spread (%.3f%% > %.3f%%)", spread, s.Config.SpotMaxSpread)
		}
	}

	return true, ""
}

// L1Filter - Trend Filter (D1 Analysis: EMA200, Structure)
func (s *SpotStrategy) L1Filter(ind *Indicators, strictness int) (bool, string) {
	// Trend Check
	// If Close < EMA200, we generally avoid unless in Reversal Mode (L1 Reversal logic).
	// strictness affects how much we tolerate.

	// L1 Pass conditions:
	// 1. Uptrend: Close > EMA200 && EMA50 > EMA200
	// 2. Reversal: Close < EMA200 but Reversal Detected (with high strictness/AI later, but L1 allows it as candidate)

	// Implementation:
	// We need current Close price vs EMA200. Indicators struct has EMA20, EMA50.
	// We need EMA200. Calculating EMA200 requires more history (200+ candles).
	// Indicators struct in `indicators.go` currently calculates EMA20 and EMA50.
	// We should add EMA200 to Indicators or calculate it here if we pass klines, but L1Filter takes *Indicators.
	// Assumption: Indicators should include EMA200. I need to update Indicators struct?
	// Or maybe use EMA50 > EMA200 from `ind`?
	// `indicators.go` calculates EMA20, EMA50.
	// We need D1 EMA200.
	// The `ind` passed here is derived from... which klines?
	// The pipeline usually scans 5m or 15m. L1 Trend Filter requires D1.
	// So we need to fetch D1 klines and calculate D1 Indicators separately.

	// This function handles just the logic check, assuming `ind` is for D1 timeframe?
	// Or we pass D1 indicators specifically.

	// Getting D1 context is part of the pipeline loop (fetching D1 klines).

	return true, "" // Placeholder, actual logic will be in RunPipeline loop or explicitly passed D1 indicators
}

// AnalyzeTrendD1 performs L1 checks on Daily Klines
func (s *SpotStrategy) AnalyzeTrendD1(klines []exchange.Kline, strictness int) (bool, string, *Indicators) {
	if len(klines) < 200 {
		return false, "Insufficient history (need 200+ D1 candles)", nil
	}

	// Calculate D1 Indicators
	ind := CalculateIndicators(klines)

	prices := make([]float64, len(klines))
	for i, k := range klines {
		prices[i] = k.Close
	}
	ema200 := calculateEMA(prices, 200)
	ema50 := ind.EMA50
	closePrice := klines[len(klines)-1].Close

	ind.EMA200 = ema200 // Need to add this field to Indicators struct or just return it/use it.

	// Logic
	// Standard: Uptrend if Close > EMA200 && EMA50 > EMA200
	isUptrend := closePrice > ema200 && ema50 > ema200

	// Low strictness relaxation (< 50)
	// If strictness is LOW, we allow shorter-term momentum even if long term is messy
	if strictness < 50 {
		// Just Close > EMA50 is sufficient for "Low" strictness
		if closePrice > ema50 {
			isUptrend = true
		}
	}

	if isUptrend {
		return true, "Uptrend (Close > EMA200 or Low Strictness)", ind
	}

	// Reversal check
	if ind.IsReversal {
		return true, "Reversal Candidate", ind
	}

	return false, "Downtrend (Close < EMA200)", ind
}

// L2Filter - Setup Filter (M15/H1/H4)
// Checks for Pullback, Breakout, Squeeze
func (s *SpotStrategy) L2Filter(ctx context.Context, klines []exchange.Kline, ind *Indicators, strictness int) (bool, string, string) {
	// Reuse existing Phase2Filter logic but adapted/enhanced
	// Phase2Filter in indicators.go checks RSI/MACD.
	// We can use it as base.

	passed, side, reason := Phase2Filter(ind, strictness)
	if !passed {
		return false, "", reason
	}

	// Additional checks for Spot specific setups (Pullback etc)
	// ... (For MVP Phase2Filter logic covers RSI/MACD setup)

	return true, side, "Spot Setup found"
}

// L3Filter - Risk/Fees
func (s *SpotStrategy) L3Filter(currentPrice float64, ind *Indicators) (bool, float64, float64, string) {
	// Calculate TP/SL
	tp, sl := CalculateTPSL(currentPrice, "LONG", ind)

	// Check Min TP
	minTpPercent := 1.2 // 1.2%
	tpPercent := (tp - currentPrice) / currentPrice * 100

	if tpPercent < minTpPercent {
		return false, 0, 0, fmt.Sprintf("TP too small (%.2f%% < %.2f%%)", tpPercent, minTpPercent)
	}

	// Check R:R
	risk := currentPrice - sl
	reward := tp - currentPrice
	if risk <= 0 {
		return false, 0, 0, "Invalid SL (above price for LONG)"
	}
	rr := reward / risk
	if rr < 1.4 {
		return false, 0, 0, fmt.Sprintf("Low R:R (%.2f < 1.4)", rr)
	}

	return true, tp, sl, ""
}

// Helper need to expose calculateEMA from analysis package?
// It is unexported `calculateEMA`. I should export it or copy it.
// Or I can add `EMA200` to `Indicators` struct in `indicators.go` and `CalculateIndicators`
// so I don't have to re-implement.
