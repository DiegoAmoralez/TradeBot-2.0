package analysis

import (
	"bot_trading/internal/exchange"
	"fmt"
	"math"
)

// Indicators holds calculated technical indicators
type Indicators struct {
	RSI             float64
	MACD            float64
	Signal          float64
	VolumeChange    float64
	PriceChange1h   float64
	PriceChange4h   float64
	Trend           string // "BULLISH", "BEARISH", "NEUTRAL"
	Volatility      float64
	SupportLevel    float64
	ResistanceLevel float64
	EMA20           float64
	EMA50           float64
	EMA200          float64
	RVol            float64 // Relative Volume
	Trend4h         string  // "BULLISH", "BEARISH", "NEUTRAL"
	TrendDaily      string  // "BULLISH", "BEARISH", "NEUTRAL"
	Structure       string  // "BULLISH", "BEARISH", "SIDEWAYS"
	IsReversal      bool
}

// CalculateIndicators computes technical indicators from klines
func CalculateIndicators(klines []exchange.Kline) *Indicators {
	if len(klines) < 50 {
		return nil
	}

	ind := &Indicators{}

	// RSI (14 period)
	ind.RSI = calculateRSI(klines, 14)

	// MACD (12, 26, 9)
	ind.MACD, ind.Signal = calculateMACD(klines)

	// Volume change
	if len(klines) >= 2 {
		currentVol := klines[len(klines)-1].Volume
		prevVol := klines[len(klines)-2].Volume
		if prevVol > 0 {
			ind.VolumeChange = ((currentVol - prevVol) / prevVol) * 100
		}
	}

	// Price changes
	currentPrice := klines[len(klines)-1].Close
	if len(klines) >= 12 {
		price1hAgo := klines[len(klines)-12].Close
		ind.PriceChange1h = ((currentPrice - price1hAgo) / price1hAgo) * 100
	}
	if len(klines) >= 48 {
		price4hAgo := klines[len(klines)-48].Close
		ind.PriceChange4h = ((currentPrice - price4hAgo) / price4hAgo) * 100
	}

	// Trend determination
	ind.Trend = determineTrend(klines)

	// Volatility (ATR-based)
	ind.Volatility = calculateATR(klines, 14)

	// Support/Resistance
	ind.SupportLevel, ind.ResistanceLevel = findSupportResistance(klines)

	// EMA for crossovers
	prices := make([]float64, len(klines))
	for i, k := range klines {
		prices[i] = k.Close
	}
	ind.EMA20 = calculateEMA(prices, 20)
	ind.EMA50 = calculateEMA(prices, 50)

	// RVol (last candle volume vs 20-period average)
	if len(klines) >= 20 {
		var volSum float64
		for i := len(klines) - 20; i < len(klines)-1; i++ {
			volSum += klines[i].Volume
		}
		avgVol := volSum / 19.0
		if avgVol > 0 {
			ind.RVol = klines[len(klines)-1].Volume / avgVol
		}
	}

	// Structure (HH/HL vs LH/LL)
	ind.Structure = detectStructure(klines)

	// Reversal candidate
	ind.IsReversal = detectReversal(klines, ind)

	return ind
}

// detectStructure determines if the market structure is BULLISH (HH/HL) or BEARISH (LH/LL) based on swing points
func detectStructure(klines []exchange.Kline) string {
	if len(klines) < 20 {
		return "NEUTRAL"
	}
	// Simplified structure detection: compare recent swing highs/lows
	// This is a naive implementation; a real one requires zig-zag or pivot point logic.
	// We'll use a simple comparison of 20-period halves for MVP.
	mid := len(klines) / 2
	firstHalfHigh := maxBy(klines[:mid], func(k exchange.Kline) float64 { return k.High })
	secondHalfHigh := maxBy(klines[mid:], func(k exchange.Kline) float64 { return k.High })

	firstHalfLow := minBy(klines[:mid], func(k exchange.Kline) float64 { return k.Low })
	secondHalfLow := minBy(klines[mid:], func(k exchange.Kline) float64 { return k.Low })

	if secondHalfHigh > firstHalfHigh && secondHalfLow > firstHalfLow {
		return "BULLISH"
	} else if secondHalfHigh < firstHalfHigh && secondHalfLow < firstHalfLow {
		return "BEARISH"
	}
	return "SIDEWAYS"
}

// detectReversal checks if conditions for a reversal buy are met
func detectReversal(klines []exchange.Kline, ind *Indicators) bool {
	if len(klines) < 3 {
		return false
	}
	// 1. RSI < 32 (oversold) but rising recently could be passed in, but here checking just low RSI
	if ind.RSI > 32 {
		return false
	}

	// 2. Rising RSI last 2 bars handling?
	// The indicator only holds current RSI. We'd need history of RSI or just check price action.
	// For MVP: RSI < 32 is the base trigger.
	return true
}

// Helper generic max/min functions
func maxBy[T any](slice []T, valueFunc func(T) float64) float64 {
	if len(slice) == 0 {
		return 0
	}
	m := -math.MaxFloat64
	for _, item := range slice {
		v := valueFunc(item)
		if v > m {
			m = v
		}
	}
	return m
}

func minBy[T any](slice []T, valueFunc func(T) float64) float64 {
	if len(slice) == 0 {
		return 0
	}
	m := math.MaxFloat64
	for _, item := range slice {
		v := valueFunc(item)
		if v < m {
			m = v
		}
	}
	return m
}

func calculateRSI(klines []exchange.Kline, period int) float64 {
	if len(klines) < period+1 {
		return 50
	}

	gains := 0.0
	losses := 0.0

	for i := len(klines) - period; i < len(klines); i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			gains += change
		} else {
			losses -= change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))
	return rsi
}

func calculateEMA(prices []float64, period int) float64 {
	if len(prices) == 0 {
		return 0
	}
	if len(prices) < period {
		// Not enough data for full EMA, return SMA as starting point
		sum := 0.0
		for _, p := range prices {
			sum += p
		}
		return sum / float64(len(prices))
	}

	multiplier := 2.0 / float64(period+1)
	// Start with SMA of the first 'period' elements
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += prices[i]
	}
	ema := sum / float64(period)

	for i := period; i < len(prices); i++ {
		ema = (prices[i] * multiplier) + (ema * (1 - multiplier))
	}

	return ema
}

// calculateEMAForMACD is a specialized EMA for MACD signal line which might have fewer than 9 points initially
func calculateEMAForMACD(values []float64, period int) float64 {
	if len(values) == 0 {
		return 0
	}
	multiplier := 2.0 / float64(period+1)
	ema := values[0]
	for i := 1; i < len(values); i++ {
		ema = (values[i] * multiplier) + (ema * (1 - multiplier))
	}
	return ema
}

func calculateMACD(klines []exchange.Kline) (float64, float64) {
	if len(klines) < 35 { // 26 + 9 for proper signal line development
		return 0, 0
	}

	prices := make([]float64, len(klines))
	for i, k := range klines {
		prices[i] = k.Close
	}

	// We need to calculate a series of MACD values to get an EMA of them
	// Let's calculate the last 15 MACD points
	macdValues := []float64{}
	for i := len(prices) - 15; i < len(prices); i++ {
		ema12 := calculateEMA(prices[:i+1], 12)
		ema26 := calculateEMA(prices[:i+1], 26)
		macdValues = append(macdValues, ema12-ema26)
	}

	currentMacd := macdValues[len(macdValues)-1]
	signal := calculateEMAForMACD(macdValues, 9)

	return currentMacd, signal
}

// CalculateTrend determines the trend for a given set of klines using EMA crossover
func CalculateTrend(klines []exchange.Kline) string {
	if len(klines) < 50 {
		return "NEUTRAL"
	}

	prices := make([]float64, len(klines))
	for i, k := range klines {
		prices[i] = k.Close
	}

	ema20 := calculateEMA(prices, 20)
	ema50 := calculateEMA(prices, 50)

	// Bullish: EMA20 > EMA50 * 1.002
	// Bearish: EMA20 < EMA50 * 0.998
	if ema20 > ema50*1.002 {
		return "BULLISH"
	} else if ema20 < ema50*0.998 {
		return "BEARISH"
	}
	return "NEUTRAL"
}

func determineTrend(klines []exchange.Kline) string {
	return CalculateTrend(klines)
}

func calculateATR(klines []exchange.Kline, period int) float64 {
	if len(klines) < period+1 {
		return 0
	}

	sum := 0.0
	for i := len(klines) - period; i < len(klines); i++ {
		tr := math.Max(klines[i].High-klines[i].Low,
			math.Max(math.Abs(klines[i].High-klines[i-1].Close),
				math.Abs(klines[i].Low-klines[i-1].Close)))
		sum += tr
	}

	return sum / float64(period)
}

func findSupportResistance(klines []exchange.Kline) (float64, float64) {
	if len(klines) < 20 {
		current := klines[len(klines)-1].Close
		return current * 0.98, current * 1.02
	}

	// Find recent highs and lows
	var highs, lows []float64
	for i := len(klines) - 20; i < len(klines); i++ {
		highs = append(highs, klines[i].High)
		lows = append(lows, klines[i].Low)
	}

	support := min(lows)
	resistance := max(highs)

	return support, resistance
}

func min(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values {
		if v < m {
			m = v
		}
	}
	return m
}

func max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values {
		if v > m {
			m = v
		}
	}
	return m
}

// Phase1Filter - Basic market scan filter
func Phase1Filter(ind *Indicators, strictness int) bool {
	k := float64(strictness) / 100.0

	// Filter criteria for initial scan
	// 1. Volatility check (not too low, not too high)
	// Base range: 0.3 - 15. High strictness scales the floor up and ceiling down.
	volFloor := 0.3 * k
	volCeiling := 15.0 / k
	if ind.Volatility < volFloor || ind.Volatility > volCeiling {
		return false
	}

	// 2. Volume check (some activity)
	// Base: 3. High strictness requires more volume.
	if math.Abs(ind.VolumeChange) < 3*k {
		return false
	}

	// 3. Price movement check
	// Base: 0.3. High strictness requires more movement.
	if math.Abs(ind.PriceChange1h) < 0.3*k {
		return false
	}

	return true
}

// Phase2Filter - Detailed strategy filter with strictness adjustment
func Phase2Filter(ind *Indicators, strictness int) (bool, string, string) {
	k := float64(strictness) / 100.0

	// Scaling RSI: higher strictness (k > 1) pushes thresholds further from 50 (neutral).
	// k=1.0 (strictness 100) -> 45 / 55
	// k=0.5 (strictness 50)  -> 47.5 / 52.5 (more relaxed)
	// k=1.5 (strictness 150) -> 42.5 / 57.5 (stricter)
	rsiLongThreshold := 50.0 - (5.0 * k)
	rsiShortThreshold := 50.0 + (5.0 * k)

	// Long signal conditions
	longSignal := ind.RSI < rsiLongThreshold &&
		ind.MACD > ind.Signal && // MACD crossover
		ind.Trend != "BEARISH"

	// Short signal conditions
	shortSignal := ind.RSI > rsiShortThreshold &&
		ind.MACD < ind.Signal && // MACD crossunder
		ind.Trend != "BULLISH"

	if longSignal {
		return true, "LONG", ""
	}
	if shortSignal {
		return true, "SHORT", ""
	}

	// Determine rejection reason
	reason := ""
	if ind.RSI >= rsiLongThreshold && ind.RSI <= rsiShortThreshold {
		reason = fmt.Sprintf("RSI in neutral/strict zone (%.1f, threshold: %.1f-%.1f)", ind.RSI, rsiLongThreshold, rsiShortThreshold)
	} else if ind.MACD <= ind.Signal && ind.RSI < rsiLongThreshold {
		reason = fmt.Sprintf("MACD bearish despite low RSI (MACD: %.4f < Signal: %.4f)", ind.MACD, ind.Signal)
	} else if ind.MACD >= ind.Signal && ind.RSI > rsiShortThreshold {
		reason = fmt.Sprintf("MACD bullish despite high RSI (MACD: %.4f > Signal: %.4f)", ind.MACD, ind.Signal)
	} else if ind.Trend == "BEARISH" && ind.RSI < rsiLongThreshold {
		reason = "Bearish trend conflicts with oversold RSI"
	} else if ind.Trend == "BULLISH" && ind.RSI > rsiShortThreshold {
		reason = "Bullish trend conflicts with overbought RSI"
	} else {
		reason = fmt.Sprintf("No clear signal (RSI: %.1f, MACD: %.4f, Trend: %s)", ind.RSI, ind.MACD, ind.Trend)
	}

	return false, "", reason
}

// CalculateTPSL calculates take profit and stop loss levels
func CalculateTPSL(currentPrice float64, side string, ind *Indicators) (tp, sl float64) {
	riskRewardRatio := 2.0

	if side == "LONG" {
		// SL below support or 2% below entry
		sl = math.Min(ind.SupportLevel, currentPrice*0.98)
		risk := currentPrice - sl
		tp = currentPrice + (risk * riskRewardRatio)
	} else {
		// SL above resistance or 2% above entry
		sl = math.Max(ind.ResistanceLevel, currentPrice*1.02)
		risk := sl - currentPrice
		tp = currentPrice - (risk * riskRewardRatio)
	}

	return tp, sl
}
