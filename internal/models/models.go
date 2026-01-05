package models

import "time"

// Position represents a trading position
type Position struct {
	ID           string
	Symbol       string
	Side         string // "LONG" or "SHORT"
	EntryPrice   float64
	CurrentPrice float64
	Quantity     float64
	PositionSize float64 // in USDT
	TakeProfit   float64
	StopLoss     float64
	TrailStop    float64
	OpenTime     time.Time
	CreatedAt    time.Time
	UnrealizedPL float64
	PLPercent    float64
	Reasoning    string
	Log          []PositionLog
}

// PositionLog tracks events for a position
type PositionLog struct {
	Time   time.Time
	Type   string // "OPEN", "DCA", "TP_UPDATE", etc.
	Price  float64
	Size   float64
	Reason string
}

// Trade represents a closed trade
type Trade struct {
	Symbol       string
	Side         string
	EntryPrice   float64
	ExitPrice    float64
	PositionSize float64
	RealizedPL   float64
	PLPercent    float64
	OpenTime     time.Time
	CloseTime    time.Time
	Duration     time.Duration
	CloseReason  string // "TP", "SL", "MANUAL"
}

// Stats represents trading statistics
type Stats struct {
	TotalTrades      int
	ProfitableTrades int
	LosingTrades     int
	TotalPL          float64
	RealizedPL       float64
	UnrealizedPL     float64
	WinRate          float64
	AvgProfit        float64
	AvgLoss          float64
	MaxProfit        float64
	MaxLoss          float64
	AvgHoldTime      time.Duration
}

// AISignal represents AI analysis result
type AISignal struct {
	Symbol     string
	Action     string  // "BUY", "SELL", "WAIT"
	Side       string  // "LONG", "SHORT"
	Confidence float64 // 0-100
	TakeProfit float64
	StopLoss   float64
	Reasoning  string
	FirstSeen  time.Time
}
