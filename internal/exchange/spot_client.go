package exchange

import (
	"bot_trading/internal/models"
	"context"
	"fmt"
	"time"

	"github.com/adshao/go-binance/v2"
)

// SpotClient - real Binance Spot client
type SpotClient struct {
	client *binance.Client
}

func NewSpotClient(apiKey, secretKey string, testnet bool) *SpotClient {
	client := binance.NewClient(apiKey, secretKey)
	if testnet {
		binance.UseTestnet = true
	}
	return &SpotClient{client: client}
}

func (s *SpotClient) GetPairs(ctx context.Context) ([]string, error) {
	info, err := s.client.NewExchangeInfoService().Do(ctx)
	if err != nil {
		return nil, err
	}

	var symbols []string
	for _, sym := range info.Symbols {
		if sym.QuoteAsset == "USDT" && sym.Status == "TRADING" && sym.IsSpotTradingAllowed {
			symbols = append(symbols, sym.Symbol)
		}
	}
	return symbols, nil
}

func (s *SpotClient) Get24hStats(ctx context.Context) ([]TickerStats, error) {
	stats, err := s.client.NewListPriceChangeStatsService().Do(ctx)
	if err != nil {
		return nil, err
	}

	var result []TickerStats
	for _, st := range stats {
		result = append(result, TickerStats{
			Symbol:      st.Symbol,
			LastPrice:   parseFloat(st.LastPrice),
			QuoteVolume: parseFloat(st.QuoteVolume),
			PriceChange: parseFloat(st.PriceChangePercent),
			BidPrice:    parseFloat(st.BidPrice),
			AskPrice:    parseFloat(st.AskPrice),
		})
	}
	return result, nil
}

func (s *SpotClient) GetPrice(ctx context.Context, symbol string) (float64, error) {
	prices, err := s.client.NewListPricesService().Symbol(symbol).Do(ctx)
	if err != nil {
		return 0, err
	}
	if len(prices) == 0 {
		return 0, fmt.Errorf("no price data for %s", symbol)
	}
	return parseFloat(prices[0].Price), nil
}

func (s *SpotClient) GetKlines(ctx context.Context, symbol string, interval string, limit int) ([]Kline, error) {
	klines, err := s.client.NewKlinesService().
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

func (s *SpotClient) OpenPosition(ctx context.Context, symbol, side string, size float64, tp, sl float64) (*models.Position, error) {
	// Spot only supports LONG for basic implementation
	if side != "LONG" {
		return nil, fmt.Errorf("spot currenty only allows LONG positions")
	}

	price, err := s.GetPrice(ctx, symbol)
	if err != nil {
		return nil, err
	}

	// Real buy order logic would go here
	// For now we just return position object to simulate successful entry in this "Client" wrapper
	// In production this would be client.NewCreateOrderService()...

	return &models.Position{
		ID:           fmt.Sprintf("SPOT_%s_%d", symbol, time.Now().Unix()),
		Symbol:       symbol,
		Side:         side,
		EntryPrice:   price,
		CurrentPrice: price,
		PositionSize: size,
		TakeProfit:   tp,
		StopLoss:     sl,
		TrailStop:    0, // Spot trailing might be different, keeping simplistic
		OpenTime:     time.Now(),
	}, nil
}

func (c *SpotClient) AddToPosition(ctx context.Context, symbol, side string, size float64) (*models.Position, error) {
	// For Spot, adding is just opening a new order.
	// We reuse OpenPosition but without TP/SL (since they are managed by the engine/first order usually)
	// We iterate tp/sl as 0 to signal "Just Buy".
	return c.OpenPosition(ctx, symbol, side, size, 0, 0)
}

func (s *SpotClient) ClosePosition(ctx context.Context, position *models.Position, reason string) (*models.Trade, error) {
	// Real sell order logic would go here
	price, err := s.GetPrice(ctx, position.Symbol)
	if err != nil {
		return nil, err
	}

	pl := (price - position.EntryPrice) / position.EntryPrice * position.PositionSize
	// Fee ~0.1%
	pl -= position.PositionSize * 0.001

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

func (s *SpotClient) UpdatePosition(ctx context.Context, position *models.Position) error {
	price, err := s.GetPrice(ctx, position.Symbol)
	if err != nil {
		return err
	}

	position.CurrentPrice = price
	position.UnrealizedPL = (price - position.EntryPrice) / position.EntryPrice * position.PositionSize
	position.PLPercent = position.UnrealizedPL / position.PositionSize * 100

	return nil
}

func (s *SpotClient) GetBalance(ctx context.Context) (float64, error) {
	account, err := s.client.NewGetAccountService().Do(ctx)
	if err != nil {
		return 0, err
	}

	for _, balance := range account.Balances {
		if balance.Asset == "USDT" {
			return parseFloat(balance.Free), nil
		}
	}
	return 0, nil
}
