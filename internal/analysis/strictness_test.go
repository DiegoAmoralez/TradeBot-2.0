package analysis

import (
	"bot_trading/internal/exchange"
	"testing"
)

func TestStrictnessLogic(t *testing.T) {
	// Mock klines for an oversold condition
	// RSI will be low
	klines := make([]exchange.Kline, 100)
	for i := range klines {
		klines[i] = exchange.Kline{Close: 100.0 - float64(i)*0.1}
	}

	ind := CalculateIndicators(klines)
	if ind == nil {
		t.Fatal("Failed to calculate indicators")
	}

	// Test Phase 2 Filter at different strictness
	// Default strictness (100) -> threshold 45
	passed100, _, _ := Phase2Filter(ind, 100)

	// Higher strictness (150) -> threshold 42.5
	passed150, _, _ := Phase2Filter(ind, 150)

	// Lower strictness (50) -> threshold 47.5
	passed50, _, _ := Phase2Filter(ind, 50)

	t.Logf("RSI: %.2f", ind.RSI)
	t.Logf("Passed 100: %v, Passed 150: %v, Passed 50: %v", passed100, passed150, passed50)

	// In this specific mock (price goes down linearly), RSI will be very low.
	// But the logic is: high strictness = harder to pass.
}
