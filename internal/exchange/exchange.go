package exchange

import (
	"bot_trading/internal/models"
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/adshao/go-binance/v2/futures"
)

// ExchangeClient interface for both real and emulated trading
type ExchangeClient interface {
	GetPairs(ctx context.Context) ([]string, error)
	GetPrice(ctx context.Context, symbol string) (float64, error)
	Get24hStats(ctx context.Context) ([]TickerStats, error)
	GetKlines(ctx context.Context, symbol string, interval string, limit int) ([]Kline, error)
	OpenPosition(ctx context.Context, symbol, side string, size float64, tp, sl float64) (*models.Position, error)
	AddToPosition(ctx context.Context, symbol, side string, size float64) (*models.Position, error)
	ClosePosition(ctx context.Context, position *models.Position, reason string) (*models.Trade, error)
	UpdatePosition(ctx context.Context, position *models.Position) error
	GetBalance(ctx context.Context) (float64, error)
}

type Kline struct {
	OpenTime  time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	CloseTime time.Time
}

type TickerStats struct {
	Symbol      string
	LastPrice   float64
	QuoteVolume float64
	PriceChange float64 // Percent
	BidPrice    float64
	AskPrice    float64
}

// FuturesClient - real Binance Futures client
type FuturesClient struct {
	client *futures.Client
}

func NewFuturesClient(apiKey, secretKey string, testnet bool) *FuturesClient {
	client := futures.NewClient(apiKey, secretKey)
	if testnet {
		futures.UseTestnet = true
	}
	return &FuturesClient{client: client}
}

func (b *FuturesClient) GetPairs(ctx context.Context) ([]string, error) {
	info, err := b.client.NewExchangeInfoService().Do(ctx)
	if err != nil {
		return nil, err
	}

	var symbols []string
	for _, s := range info.Symbols {
		if s.QuoteAsset == "USDT" && s.Status == "TRADING" {
			symbols = append(symbols, s.Symbol)
		}
	}
	return symbols, nil
}

func (b *FuturesClient) Get24hStats(ctx context.Context) ([]TickerStats, error) {
	stats, err := b.client.NewListPriceChangeStatsService().Do(ctx)
	if err != nil {
		return nil, err
	}

	var result []TickerStats
	for _, s := range stats {
		result = append(result, TickerStats{
			Symbol:      s.Symbol,
			LastPrice:   parseFloat(s.LastPrice),
			QuoteVolume: parseFloat(s.QuoteVolume),
			PriceChange: parseFloat(s.PriceChangePercent),
		})
	}
	return result, nil
}

func (b *FuturesClient) GetPrice(ctx context.Context, symbol string) (float64, error) {
	prices, err := b.client.NewListPricesService().Symbol(symbol).Do(ctx)
	if err != nil {
		return 0, err
	}
	if len(prices) == 0 {
		return 0, fmt.Errorf("no price data for %s", symbol)
	}
	return parseFloat(prices[0].Price), nil
}

func (b *FuturesClient) GetKlines(ctx context.Context, symbol string, interval string, limit int) ([]Kline, error) {
	klines, err := b.client.NewKlinesService().
		Symbol(symbol).
		Interval(interval).
		Limit(limit).
		Do(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]Kline, len(klines))
	for i, k := range klines {
		result[i] = Kline{
			OpenTime:  time.Unix(k.OpenTime/1000, 0),
			Open:      parseFloat(k.Open),
			High:      parseFloat(k.High),
			Low:       parseFloat(k.Low),
			Close:     parseFloat(k.Close),
			Volume:    parseFloat(k.Volume),
			CloseTime: time.Unix(k.CloseTime/1000, 0),
		}
	}
	return result, nil
}

func (b *FuturesClient) OpenPosition(ctx context.Context, symbol, side string, size float64, tp, sl float64) (*models.Position, error) {
	// This is a simplified implementation - real implementation would need proper order placement
	price, err := b.GetPrice(ctx, symbol)
	if err != nil {
		return nil, err
	}

	return &models.Position{
		ID:           fmt.Sprintf("%s_%d", symbol, time.Now().Unix()),
		Symbol:       symbol,
		Side:         side,
		EntryPrice:   price,
		CurrentPrice: price,
		PositionSize: size,
		TakeProfit:   tp,
		StopLoss:     sl,
		TrailStop:    tp * 0.01, // 1% of TP
		OpenTime:     time.Now(),
	}, nil
}

func (b *FuturesClient) AddToPosition(ctx context.Context, symbol, side string, size float64) (*models.Position, error) {
	// Re-use OpenPosition logic for stub, as adding is just another buy/sell
	// For real futures, we would place a new order.
	return b.OpenPosition(ctx, symbol, side, size, 0, 0) // TP/SL 0 as they merge
}

func (b *FuturesClient) ClosePosition(ctx context.Context, position *models.Position, reason string) (*models.Trade, error) {
	price, err := b.GetPrice(ctx, position.Symbol)
	if err != nil {
		return nil, err
	}

	var pl float64
	if position.Side == "LONG" {
		pl = (price - position.EntryPrice) / position.EntryPrice * position.PositionSize
	} else {
		pl = (position.EntryPrice - price) / position.EntryPrice * position.PositionSize
	}

	return &models.Trade{
		Symbol:       position.Symbol,
		Side:         position.Side,
		EntryPrice:   position.EntryPrice,
		ExitPrice:    price,
		PositionSize: position.PositionSize,
		RealizedPL:   pl,
		PLPercent:    pl / position.PositionSize * 100,
		OpenTime:     position.OpenTime,
		CloseTime:    time.Now(),
		Duration:     time.Since(position.OpenTime),
		CloseReason:  reason,
	}, nil
}

func (b *FuturesClient) UpdatePosition(ctx context.Context, position *models.Position) error {
	price, err := b.GetPrice(ctx, position.Symbol)
	if err != nil {
		return err
	}

	position.CurrentPrice = price
	if position.Side == "LONG" {
		position.UnrealizedPL = (price - position.EntryPrice) / position.EntryPrice * position.PositionSize
	} else {
		position.UnrealizedPL = (position.EntryPrice - price) / position.EntryPrice * position.PositionSize
	}
	position.PLPercent = position.UnrealizedPL / position.PositionSize * 100

	return nil
}

func (b *FuturesClient) GetBalance(ctx context.Context) (float64, error) {
	account, err := b.client.NewGetAccountService().Do(ctx)
	if err != nil {
		return 0, err
	}

	for _, asset := range account.Assets {
		if asset.Asset == "USDT" {
			return parseFloat(asset.WalletBalance), nil
		}
	}
	return 0, nil
}

// EmulatorClient - paper trading client
type EmulatorClient struct {
	balance   float64
	positions map[string]*models.Position
	mu        sync.RWMutex
	baseAPI   ExchangeClient // Changed: can wrap any ExchangeClient (Futures or Spot)
}

func NewEmulatorClient(initialBalance float64, api ExchangeClient) *EmulatorClient {
	return &EmulatorClient{
		balance:   initialBalance,
		positions: make(map[string]*models.Position),
		baseAPI:   api,
	}
}

func (e *EmulatorClient) GetPairs(ctx context.Context) ([]string, error) {
	return e.baseAPI.GetPairs(ctx)
}

func (e *EmulatorClient) Get24hStats(ctx context.Context) ([]TickerStats, error) {
	return e.baseAPI.Get24hStats(ctx)
}

func (e *EmulatorClient) GetPrice(ctx context.Context, symbol string) (float64, error) {
	return e.baseAPI.GetPrice(ctx, symbol)
}

func (e *EmulatorClient) GetKlines(ctx context.Context, symbol string, interval string, limit int) ([]Kline, error) {
	return e.baseAPI.GetKlines(ctx, symbol, interval, limit)
}

func (e *EmulatorClient) OpenPosition(ctx context.Context, symbol, side string, size float64, tp, sl float64) (*models.Position, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.balance < size {
		return nil, fmt.Errorf("insufficient balance: %.2f USDT", e.balance)
	}

	price, err := e.GetPrice(ctx, symbol)
	if err != nil {
		return nil, err
	}

	e.balance -= size

	position := &models.Position{
		ID:           fmt.Sprintf("%s_%d", symbol, time.Now().Unix()),
		Symbol:       symbol,
		Side:         side,
		EntryPrice:   price,
		CurrentPrice: price,
		PositionSize: size,
		TakeProfit:   tp,
		StopLoss:     sl,
		TrailStop:    math.Abs(tp-price) * 0.01, // 1% of TP distance
		OpenTime:     time.Now(),
	}

	e.positions[position.ID] = position
	log.Printf("✅ Emulator: Opened %s %s at %.4f | Size: %.2f USDT | TP: %.4f | SL: %.4f",
		side, symbol, price, size, tp, sl)

	return position, nil
}

func (e *EmulatorClient) AddToPosition(ctx context.Context, symbol, side string, size float64) (*models.Position, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Find existing position
	var existing *models.Position
	for _, p := range e.positions {
		if p.Symbol == symbol {
			existing = p
			break
		}
	}

	if existing == nil {
		return nil, fmt.Errorf("position not found for %s", symbol)
	}

	if e.balance < size {
		return nil, fmt.Errorf("insufficient balance: %.2f USDT", e.balance)
	}

	price, err := e.GetPrice(ctx, symbol)
	if err != nil {
		return nil, err
	}

	e.balance -= size

	// Calculate DCA
	// NewAvg = (OldQty*OldPrice + NewQty*NewPrice) / TotalQty
	// Since Size = Qty * Price, => (OldSize + NewSize)
	oldSize := existing.PositionSize
	oldEntry := existing.EntryPrice // Weighted avg stored here

	// Qty approximation (assuming USDT size)
	oldQty := oldSize / oldEntry
	newQty := size / price

	totalQty := oldQty + newQty
	newAvgPrice := (oldSize + size) / totalQty

	existing.EntryPrice = newAvgPrice
	existing.PositionSize += size

	// Update Current Price & PL immediately
	existing.CurrentPrice = price
	// TP/SL adjustment?
	// Option: Keep original TP price? Or maintain % distance?
	// User said "recalculate average", implying TP should likely shift to maintain profitability logic or stay fixed.
	// Common DCA strategy: Average down entry, usually keep TP same (so TP is reached sooner %-wise) or lower TP to break even sooner.
	// Let's keep TP/SL levels fixed absolute values for now unless requested,
	// OR re-apply the fixed % from the strategy?
	// For simplicity, we DO NOT move TP/SL automatically here, assuming strategy logic might adjust it later if needed.
	// But resetting TrailStop distance might be good.

	log.Printf("✅ Emulator: Added to %s %s at %.4f | Added: %.2f USDT | New Avg: %.4f | New Size: %.2f",
		side, symbol, price, size, newAvgPrice, existing.PositionSize)

	// Return a "Chunk" position to represent what just happened (for Engine logs/logic if needed)
	// OR return the Updated position?
	// Based on plan: Return the CHUNK.
	chunk := &models.Position{
		Symbol:       symbol,
		Side:         side,
		EntryPrice:   price,
		PositionSize: size,
		OpenTime:     time.Now(),
	}
	return chunk, nil
}

func (e *EmulatorClient) ClosePosition(ctx context.Context, position *models.Position, reason string) (*models.Trade, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	price, err := e.GetPrice(ctx, position.Symbol)
	if err != nil {
		return nil, err
	}

	var pl float64
	if position.Side == "LONG" {
		pl = (price - position.EntryPrice) / position.EntryPrice * position.PositionSize
	} else {
		pl = (position.EntryPrice - price) / position.EntryPrice * position.PositionSize
	}

	// Apply 0.04% fee (0.02% open + 0.02% close) - typical for Futures, Spot might be differnet (0.1%)
	// For MVP simplicity we keep it or can adjust if we passed fee config.
	fee := position.PositionSize * 0.001 // 0.1% for spot is safer as default or conservative
	pl -= fee

	e.balance += position.PositionSize + pl

	delete(e.positions, position.ID)

	trade := &models.Trade{
		Symbol:       position.Symbol,
		Side:         position.Side,
		EntryPrice:   position.EntryPrice,
		ExitPrice:    price,
		PositionSize: position.PositionSize,
		RealizedPL:   pl,
		PLPercent:    pl / position.PositionSize * 100,
		OpenTime:     position.OpenTime,
		CloseTime:    time.Now(),
		Duration:     time.Since(position.OpenTime),
		CloseReason:  reason,
	}

	log.Printf("🎯 Emulator: Closed %s %s | %.4f → %.4f | P&L: %.2f USDT (%.2f%%) | Reason: %s",
		position.Side, position.Symbol, position.EntryPrice, price, pl, trade.PLPercent, reason)

	return trade, nil
}

func (e *EmulatorClient) UpdatePosition(ctx context.Context, position *models.Position) error {
	price, err := e.GetPrice(ctx, position.Symbol)
	if err != nil {
		return err
	}

	position.CurrentPrice = price

	// Calculate Gross P&L
	var grossPL float64
	if position.Side == "LONG" {
		grossPL = (price - position.EntryPrice) / position.EntryPrice * position.PositionSize
	} else {
		grossPL = (position.EntryPrice - price) / position.EntryPrice * position.PositionSize
	}

	// Apply estimated close fee (0.1%) to match ClosePosition logic and show NET P&L
	fee := position.PositionSize * 0.001
	position.UnrealizedPL = grossPL - fee

	position.PLPercent = position.UnrealizedPL / position.PositionSize * 100

	return nil
}

func (e *EmulatorClient) GetBalance(ctx context.Context) (float64, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.balance, nil
}

func (e *EmulatorClient) GetPositions() []*models.Position {
	e.mu.RLock()
	defer e.mu.RUnlock()

	positions := make([]*models.Position, 0, len(e.positions))
	for _, p := range e.positions {
		positions = append(positions, p)
	}
	return positions
}

// Helper function
func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
