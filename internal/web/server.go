package web

import (
	"bot_trading/internal/engine"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type Server struct {
	engine *engine.TradingEngine
	port   string
}

func NewServer(engine *engine.TradingEngine, port string) *Server {
	return &Server{
		engine: engine,
		port:   port,
	}
}

func (s *Server) Start() {
	http.HandleFunc("/", s.handleIndex)
	http.HandleFunc("/api/stats", s.handleStats)
	http.HandleFunc("/api/positions/", s.handlePositions) // Trailing slash to match /api/positions/{id}
	http.HandleFunc("/api/signals", s.handleSignals)
	http.HandleFunc("/api/health", s.handleHealth)
	http.HandleFunc("/api/type", s.handleType)
	http.HandleFunc("/api/history", s.handleHistory)
	http.HandleFunc("/api/strictness", s.handleStrictness)
	http.HandleFunc("/api/max-slots", s.handleMaxSlots)
	http.HandleFunc("/api/mode", s.handleMode)
	http.HandleFunc("/api/close-position", s.handleClosePosition)
	http.HandleFunc("/api/engine/action", s.handleEngineAction)

	log.Printf("🌐 Web server starting on http://localhost:%s", s.port)
	go func() {
		if err := http.ListenAndServe(":"+s.port, nil); err != nil {
			log.Printf("Web server error: %v", err)
		}
	}()
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	stats := s.engine.GetStats(ctx)
	balance, _ := s.engine.GetBalance(ctx)

	positions := s.engine.GetPositions()
	inPositions := 0.0
	for _, p := range positions {
		inPositions += p.PositionSize
	}

	response := map[string]interface{}{
		"balance":        balance,
		"in_positions":   inPositions,
		"open_positions": len(positions),
		"max_slots":      s.engine.GetMaxPositions(),
		"free_slots":     s.engine.GetFreeSlots(),
		"today_profit":   stats.RealizedPL, // Proxy for today
		"total_profit":   stats.TotalPL,
		"active_signals": len(s.engine.GetActiveSignals()),
		"running":        s.engine.IsRunning(),
		"strictness":     s.engine.GetStrictness(),
		"trading_mode":   s.engine.GetTradingMode(),
		"is_simulated":   s.engine.IsSimulated(),
		"timestamp":      time.Now().Unix(),

		// Pass-through stats for UI
		"total_trades":  stats.TotalTrades,
		"profitable":    stats.ProfitableTrades,
		"losing":        stats.LosingTrades,
		"win_rate":      stats.WinRate,
		"realized_pl":   stats.RealizedPL,
		"unrealized_pl": stats.UnrealizedPL,
		"total_pl":      stats.TotalPL,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handlePositions(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path if present
	path := r.URL.Path
	id := ""
	if len(path) > len("/api/positions/") {
		id = path[len("/api/positions/"):]
	}

	// Handle DELETE /api/positions/{id}
	if r.Method == http.MethodDelete {
		if id == "" {
			http.Error(w, "Position ID required", http.StatusBadRequest)
			return
		}

		log.Printf("🔄 Closing position via API: %s", id)
		ctx := context.Background()
		err := s.engine.ClosePositionByID(ctx, id)
		if err != nil {
			log.Printf("❌ Failed to close position %s: %v", id, err)
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		log.Printf("✅ Position %s closed successfully", id)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Handle GET /api/positions or /api/positions/
	positions := s.engine.GetPositions()

	type logEntry struct {
		Time   int64   `json:"time"`
		Type   string  `json:"type"`
		Price  float64 `json:"price"`
		Size   float64 `json:"size"`
		Reason string  `json:"reason"`
	}

	type positionResponse struct {
		ID           string     `json:"id"`
		Symbol       string     `json:"symbol"`
		Side         string     `json:"side"`
		EntryPrice   float64    `json:"entry_price"`
		CurrentPrice float64    `json:"current_price"`
		PositionSize float64    `json:"position_size"`
		UnrealizedPL float64    `json:"unrealized_pl"`
		PLPercent    float64    `json:"pl_percent"`
		TakeProfit   float64    `json:"take_profit"`
		StopLoss     float64    `json:"stop_loss"`
		OpenTime     int64      `json:"open_time"`
		CreatedAt    int64      `json:"created_at"`
		Reasoning    string     `json:"reasoning"`
		Log          []logEntry `json:"log"`
	}

	response := make([]positionResponse, len(positions))
	for i, p := range positions {
		var logs []logEntry
		for _, l := range p.Log {
			logs = append(logs, logEntry{
				Time:   l.Time.Unix(),
				Type:   l.Type,
				Price:  l.Price,
				Size:   l.Size,
				Reason: l.Reason,
			})
		}

		response[i] = positionResponse{
			ID:           p.ID,
			Symbol:       p.Symbol,
			Side:         p.Side,
			EntryPrice:   p.EntryPrice,
			CurrentPrice: p.CurrentPrice,
			PositionSize: p.PositionSize,
			UnrealizedPL: p.UnrealizedPL,
			PLPercent:    p.PLPercent,
			TakeProfit:   p.TakeProfit,
			StopLoss:     p.StopLoss,
			OpenTime:     p.OpenTime.Unix(),
			CreatedAt:    p.CreatedAt.Unix(),
			Reasoning:    p.Reasoning,
			Log:          logs,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleSignals(w http.ResponseWriter, r *http.Request) {
	signals := s.engine.GetActiveSignals()

	type signalResponse struct {
		Symbol     string  `json:"symbol"`
		Side       string  `json:"side"`
		Confidence float64 `json:"confidence"`
		Reasoning  string  `json:"reasoning"`
		FirstSeen  int64   `json:"first_seen"`
		Duration   int64   `json:"duration"`
	}

	response := make([]signalResponse, len(signals))
	for i, sig := range signals {
		response[i] = signalResponse{
			Symbol:     sig.Symbol,
			Side:       sig.Side,
			Confidence: sig.Confidence,
			Reasoning:  sig.Reasoning,
			FirstSeen:  sig.FirstSeen.Unix(),
			Duration:   int64(time.Since(sig.FirstSeen).Seconds()),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type serviceStatus struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Message string `json:"message"`
		Latency int64  `json:"latency_ms"`
	}

	results := make([]serviceStatus, 0)

	// Test Binance API
	binanceStart := time.Now()
	_, err := s.engine.GetBalance(ctx)
	binanceLatency := time.Since(binanceStart).Milliseconds()

	binanceStatus := serviceStatus{
		Name:    "Binance API",
		Latency: binanceLatency,
	}
	if err != nil {
		binanceStatus.Status = "error"
		binanceStatus.Message = err.Error()
	} else {
		binanceStatus.Status = "ok"
		binanceStatus.Message = "Connected successfully"
	}
	results = append(results, binanceStatus)

	// Test getting symbols (another Binance check)
	symbolsStart := time.Now()
	symbols, err := s.engine.TestGetSymbols(ctx)
	symbolsLatency := time.Since(symbolsStart).Milliseconds()

	symbolsStatus := serviceStatus{
		Name:    "Binance Market Data",
		Latency: symbolsLatency,
	}
	if err != nil {
		symbolsStatus.Status = "error"
		symbolsStatus.Message = err.Error()
	} else {
		symbolsStatus.Status = "ok"
		symbolsStatus.Message = fmt.Sprintf("Found %d trading pairs", symbols)
	}
	results = append(results, symbolsStatus)

	// Mistral AI status (can't test without actual request, so just report configured)
	mistralStatus := serviceStatus{
		Name:    "Mistral AI",
		Status:  "configured",
		Message: "API key configured (test requires market analysis)",
		Latency: 0,
	}
	results = append(results, mistralStatus)

	// Telegram status
	telegramStatus := serviceStatus{
		Name:    "Telegram Bot",
		Status:  "configured",
		Message: "Bot is running",
		Latency: 0,
	}
	results = append(results, telegramStatus)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"services":  results,
		"timestamp": time.Now().Unix(),
	})
}

func (s *Server) handleStrictness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		Strictness int `json:"strictness"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	s.engine.SetStrictness(data.Strictness)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
}

func (s *Server) handleMaxSlots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		MaxSlots int `json:"max_slots"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate range 1-30
	if data.MaxSlots < 1 || data.MaxSlots > 30 {
		http.Error(w, "Max slots must be between 1 and 30", http.StatusBadRequest)
		return
	}

	s.engine.SetMaxPositions(data.MaxSlots)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
}

func (s *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		Mode string `json:"mode"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.engine.SwitchTradingMode(data.Mode); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
}

func (s *Server) handleType(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		IsSimulated bool `json:"is_simulated"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.engine.SwitchTradingType(data.IsSimulated); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
}

func (s *Server) handleClosePosition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		ID string `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	if err := s.engine.ClosePositionByID(ctx, data.ID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
}

func (s *Server) handleEngineAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		Action string `json:"action"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	switch data.Action {
	case "start":
		log.Println("▶️ Engine Start requested from API")
		s.engine.Start()
	case "stop":
		log.Println("⏸️ Engine Stop requested from API")
		s.engine.Stop()
	case "analyze":
		s.engine.RunManualAnalysis()
	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	trades := s.engine.GetTrades()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trades)
}

const indexHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Trading Bot Dashboard</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: linear-gradient(135deg, #0f0c29, #302b63, #24243e);
            color: #fff;
            min-height: 100vh;
            padding: 20px;
        }

        .container {
            max-width: 1400px;
            margin: 0 auto;
            display: grid;
            grid-template-columns: 350px 1fr;
            gap: 20px;
        }

        .card {
            background: rgba(255, 255, 255, 0.05);
            backdrop-filter: blur(10px);
            border-radius: 16px;
            padding: 24px;
            border: 1px solid rgba(255, 255, 255, 0.1);
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3);
        }

        h1 {
            font-size: 28px;
            margin-bottom: 24px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }

        h2 {
            font-size: 20px;
            margin-bottom: 16px;
            color: #a0aec0;
        }

        .stat-row {
            display: flex;
            justify-content: space-between;
            padding: 12px 0;
            border-bottom: 1px solid rgba(255, 255, 255, 0.05);
        }

        .stat-label {
            color: #a0aec0;
        }

        .stat-value {
            font-weight: 600;
            color: #fff;
        }

        .positive {
            color: #48bb78;
        }

        .negative {
            color: #f56565;
        }

        .status-badge {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 12px;
            font-size: 12px;
            font-weight: 600;
        }

        .status-active {
            background: rgba(72, 187, 120, 0.2);
            color: #48bb78;
        }

        .status-stopped {
            background: rgba(245, 101, 101, 0.2);
            color: #f56565;
        }

        .positions-grid {
            display: grid;
            gap: 16px;
        }

        .position-card {
            background: rgba(255, 255, 255, 0.03);
            border-radius: 12px;
            padding: 16px;
            border-left: 4px solid;
            transition: transform 0.2s;
        }

        .position-card:hover {
            transform: translateX(4px);
        }

        .position-long {
            border-left-color: #48bb78;
        }

        .position-short {
            border-left-color: #f56565;
        }

        .position-header {
            display: flex;
            justify-content: space-between;
            margin-bottom: 12px;
        }

        .position-symbol {
            font-size: 18px;
            font-weight: 700;
        }

        .position-side {
            font-size: 12px;
            padding: 4px 8px;
            border-radius: 8px;
            font-weight: 600;
        }

        .sidebar {
            max-height: calc(100vh - 40px);
            overflow-y: auto;
            padding-right: 5px;
        }

        /* Custom scrollbar for sidebar */
        .sidebar::-webkit-scrollbar {
            width: 4px;
        }
        .sidebar::-webkit-scrollbar-track {
            background: rgba(255, 255, 255, 0.05);
        }
        .sidebar::-webkit-scrollbar-thumb {
            background: rgba(102, 126, 234, 0.5);
            border-radius: 2px;
        }

        .side-long {
            background: rgba(72, 187, 120, 0.2);
            color: #48bb78;
        }

        .side-short {
            background: rgba(245, 101, 101, 0.2);
            color: #f56565;
        }

        .position-details {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 8px;
            font-size: 13px;
        }

        .detail-item {
            display: flex;
            justify-content: space-between;
        }

        .empty-state {
            text-align: center;
            padding: 40px;
            color: #a0aec0;
        }

        @media (max-width: 768px) {
            .container {
                grid-template-columns: 1fr;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="sidebar">
            <div class="card">
                <h1>📊 Trading Bot</h1>
                <div class="stat-row">
                    					<span class="stat-label">Статус</span>
					<span class="stat-value" id="status">-</span>
				</div>
                <div class="stat-row">
                    <span class="stat-label">Режим</span>
                    <span class="stat-value" id="trading_mode">-</span>
                </div>
				<div class="stat-row">
					<span class="stat-label">💰 Баланс</span>
                    <span class="stat-value" id="balance">-</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">📈 В позициях</span>
                    <span class="stat-value" id="in_positions">-</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">📋 Слоты</span>
                    <span class="stat-value" id="slots_info">-</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">💎 Unrealized P&L</span>
                    <span class="stat-value" id="unrealized_pl">-</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">💰 Revenue P&L</span>
                    <span class="stat-value" id="total_pl">-</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">📅 Всего сделок</span>
                    <span class="stat-value" id="total_trades">-</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">📊 Винрейт</span>
                    <span class="stat-value" id="win_rate">-</span>
                </div>
            </div>
            
            <div class="card" style="margin-top: 20px;">
                <h2 style="font-size: 18px; margin-bottom: 12px;">🔧 API Status</h2>
                <button id="test-apis" style="width: 100%; padding: 12px; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); border: none; border-radius: 8px; color: white; font-weight: 600; cursor: pointer; margin-bottom: 12px;">
                    Test All APIs
                </button>
                <div id="health-status" style="font-size: 13px;"></div>
            </div>

            <div class="card" style="margin-top: 20px;">
                <h2 style="font-size: 18px; margin-bottom: 20px;">🚀 Engine Control</h2>
                <div style="display: grid; grid-template-columns: repeat(2, 1fr); gap: 10px; margin-bottom: 15px;">
                    <button onclick="engineAction('start')" style="padding: 12px; border: none; border-radius: 8px; background: linear-gradient(135deg, #48bb78 0%, #38a169 100%); color: white; font-weight: 700; cursor: pointer;">Start</button>
                    <button onclick="engineAction('stop')" style="padding: 12px; border: none; border-radius: 8px; background: linear-gradient(135deg, #f56565 0%, #e53e3e 100%); color: white; font-weight: 700; cursor: pointer;">Stop</button>
                </div>
                <button onclick="engineAction('analyze')" style="width: 100%; padding: 12px; border: none; border-radius: 8px; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; font-weight: 700; cursor: pointer;">Force Search (Phase 1-3)</button>
            </div>

            <div class="card" style="margin-top: 20px;">
                <h2 style="font-size: 18px; margin-bottom: 20px;">🛡️ Strategy Control</h2>
                
                <div style="margin-bottom: 20px;">
                    <div style="display: flex; justify-content: space-between; margin-bottom: 8px;">
                        <span class="stat-label">Режим торговли</span>
                    </div>
                    <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 8px;">
                        <button id="btn-mode-futures" onclick="switchMode('FUTURES')" style="padding: 8px; border: 1px solid rgba(255,255,255,0.1); border-radius: 6px; background: rgba(255,255,255,0.05); color: #a0aec0; cursor: pointer;">Futures</button>
                        <button id="btn-mode-spot" onclick="switchMode('SPOT')" style="padding: 8px; border: 1px solid rgba(255,255,255,0.1); border-radius: 6px; background: rgba(255,255,255,0.05); color: #a0aec0; cursor: pointer;">Spot</button>
                    </div>
                </div>

                <div style="margin-bottom: 20px;">
                    <div style="display: flex; justify-content: space-between; margin-bottom: 8px;">
                        <span class="stat-label">Тип счета</span>
                    </div>
                    <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 8px;">
                        <button id="btn-type-sim" onclick="switchType(true)" style="padding: 8px; border: 1px solid rgba(255,255,255,0.1); border-radius: 6px; background: rgba(255,255,255,0.05); color: #a0aec0; cursor: pointer;">🔥 Emulator</button>
                        <button id="btn-type-real" onclick="switchType(false)" style="padding: 8px; border: 1px solid rgba(255,255,255,0.1); border-radius: 6px; background: rgba(255,255,255,0.05); color: #a0aec0; cursor: pointer;">💰 Real Money</button>
                    </div>
                </div>

                <div style="margin-bottom: 12px;">
                    <div style="display: flex; justify-content: space-between; margin-bottom: 8px;">
                        <span class="stat-label">Strictness</span>
                        <span id="strictness-value" class="stat-value" style="color: #667eea; font-weight: 700;">100</span>
                    </div>
                    <input type="range" id="strictness-slider" min="1" max="250" value="100" style="width: 100%; height: 6px; background: rgba(255,255,255,0.1); border-radius: 3px; outline: none; -webkit-appearance: none; appearance: none; cursor: pointer;">
                    <div style="display: flex; justify-content: space-between; font-size: 10px; color: #a0aec0; margin-top: 8px;">
                        <span>Aggressive</span>
                        <span>Balanced (100)</span>
                        <span>Stricter</span>
                    </div>
                </div>

                <div style="margin-bottom: 20px;">
                    <div style="display: flex; justify-content: space-between; margin-bottom: 8px;">
                        <span class="stat-label">Макс. слотов (Спот)</span>
                        <span id="max-slots-value" class="stat-value" style="color: #667eea; font-weight: 700;">-</span>
                    </div>
                    <input type="range" id="max-slots-slider" min="1" max="30" value="5" style="width: 100%; height: 6px; background: rgba(255,255,255,0.1); border-radius: 3px; outline: none; -webkit-appearance: none; appearance: none; cursor: pointer;">
                    <div style="display: flex; justify-content: space-between; font-size: 10px; color: #a0aec0; margin-top: 8px;">
                        <span>1</span>
                        <span>15</span>
                        <span>30</span>
                    </div>
                </div>
            </div>
        </div>

        <div class="main">
            <div class="card">
                <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
                    <h2>Открытые позиции</h2>
                    <div style="display: flex; gap: 10px;">
                        <input type="text" id="filter-input" placeholder="Фильтр (тикер)..." style="padding: 8px; border-radius: 6px; border: 1px solid rgba(255,255,255,0.1); background: rgba(255,255,255,0.05); color: white; outline: none;">
                        <select id="sort-select" style="padding: 8px; border-radius: 6px; border: 1px solid rgba(255,255,255,0.1); background: rgba(255,255,255,0.05); color: white; outline: none;">
                            <option value="symbol">Тикер (A-Z)</option>
                            <option value="size_desc">Размер (High-Low)</option>
                            <option value="size_asc">Размер (Low-High)</option>
                            <option value="pnl_desc">PnL (High-Low)</option>
                            <option value="pnl_asc">PnL (Low-High)</option>
                        </select>
                    </div>
                </div>
                <div class="positions-grid" id="positions">
                    <div class="empty-state">Загрузка...</div>
                </div>
            </div>

            <div class="card" style="margin-top: 20px;">
                <h2>История сделок</h2>
                <div style="overflow-x: auto;">
                    <table style="width: 100%; border-collapse: collapse; font-size: 13px;">
                        <thead>
                            <tr style="text-align: left; color: #a0aec0; border-bottom: 1px solid rgba(255,255,255,0.1);">
                                <th style="padding: 12px;">Время</th>
                                <th style="padding: 12px;">Тикер</th>
                                <th style="padding: 12px;">Сайд</th>
                                <th style="padding: 12px;">P&L (USDT)</th>
                                <th style="padding: 12px;">Сумма (USDT)</th>
                            </tr>
                        </thead>
                        <tbody id="history-table">
                            <tr><td colspan="5" class="empty-state">Загрузка...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    </div>

    <script>
        function formatNumber(num, decimals = 2) {
            return num.toFixed(decimals);
        }

        function formatPL(num) {
            const formatted = formatNumber(num, 2);
            return num >= 0 ? '+' + formatted : formatted;
        }

        function formatDate(dateStr) {
            const date = new Date(dateStr);
            return date.toLocaleString();
        }

        function formatDuration(seconds) {
            if (seconds < 60) return seconds + 's';
            const mins = Math.floor(seconds / 60);
            if (mins < 60) return mins + 'm ' + (seconds % 60) + 's';
            const hours = Math.floor(mins / 60);
            return hours + 'h ' + (mins % 60) + 'm';
        }

        let currentFilter = '';
        let currentSort = 'symbol';

        document.getElementById('filter-input').addEventListener('input', (e) => {
            currentFilter = e.target.value.toLowerCase();
            updateData(); // Trigger immediate update
        });

        document.getElementById('sort-select').addEventListener('change', (e) => {
            currentSort = e.target.value;
            updateData(); // Trigger immediate update
        });

        async function updateData() {
            try {
                const [statsRes, positionsRes, signalsRes, historyRes] = await Promise.all([
                    fetch('/api/stats'),
                    fetch('/api/positions'),
                    fetch('/api/signals'),
                    fetch('/api/history')
                ]);

                const stats = await statsRes.json();
                let positions = await positionsRes.json();
                const signals = await signalsRes.json();
                const history = await historyRes.json();

                // Update stats
                document.getElementById('status').innerHTML = stats.running 
                    ? '<span class="status-badge status-active">▶️ Активен</span>'
                    : '<span class="status-badge status-stopped">⏸️ Остановлен</span>';
                
                document.getElementById('trading_mode').innerHTML = '<span class="status-badge" style="background: rgba(102, 126, 234, 0.2); color: #667eea;">' + stats.trading_mode + '</span>';
                updateModeUI(stats.trading_mode);
                updateTypeUI(stats.is_simulated);

                document.getElementById('balance').textContent = formatNumber(stats.balance) + ' USDT';
                document.getElementById('in_positions').textContent = formatNumber(stats.in_positions) + ' USDT';
                
                // Slots
                const usedSlots = stats.max_slots - stats.free_slots;
                document.getElementById('slots_info').textContent = usedSlots + ' / ' + stats.max_slots;
                
                const unrealizedPL = document.getElementById('unrealized_pl');
                unrealizedPL.textContent = formatPL(stats.unrealized_pl) + ' USDT';
                unrealizedPL.className = 'stat-value ' + (stats.unrealized_pl >= 0 ? 'positive' : 'negative');
                
                const totalPL = document.getElementById('total_pl');
                totalPL.textContent = formatPL(stats.total_pl) + ' USDT';
                totalPL.className = 'stat-value ' + (stats.total_pl >= 0 ? 'positive' : 'negative');
                
                document.getElementById('total_trades').textContent = stats.total_trades;
                document.getElementById('win_rate').textContent = formatNumber(stats.win_rate, 1) + '%';

                // FILTER & SORT POSITIONS
                if (currentFilter) {
                    positions = positions.filter(p => p.symbol.toLowerCase().includes(currentFilter));
                }

                positions.sort((a, b) => {
                    switch (currentSort) {
                        case 'symbol':
                            return a.symbol.localeCompare(b.symbol);
                        case 'size_desc':
                            return b.position_size - a.position_size;
                        case 'size_asc':
                            return a.position_size - b.position_size;
                        case 'pnl_desc':
                            return b.unrealized_pl - a.unrealized_pl;
                        case 'pnl_asc':
                            return a.unrealized_pl - b.unrealized_pl;
                        default:
                            return 0;
                    }
                });

                // Update positions
                const positionsContainer = document.getElementById('positions');
                if (positions.length === 0) {
                    positionsContainer.innerHTML = '<div class="empty-state">Нет открытых позиций</div>';
                } else {
                    positionsContainer.innerHTML = positions.map(p => {
                        const sideClass = p.side === 'LONG' ? 'long' : 'short';
                        const plClass = p.pl_percent >= 0 ? 'positive' : 'negative';
                        const showSide = stats.trading_mode !== 'SPOT';
                        
                        const now = Math.floor(Date.now() / 1000);
                        const boughtAt = new Date(p.open_time * 1000).toLocaleTimeString();
                        const cartTime = Math.max(0, p.open_time - p.created_at);

                        // Format log history
                        const historyRows = (p.log || []).map(l => {
                             const typeClass = l.type === 'OPEN' ? 'positive' : 'warning'; // OPEN green, DCA yellow
                             return ` + "`" + `
                                <div style="display: flex; justify-content: space-between; font-size: 11px; padding: 4px 0; border-bottom: 1px solid rgba(255,255,255,0.05);">
                                    <span>${new Date(l.time * 1000).toLocaleTimeString()}</span>
                                    <span class="${typeClass}">${l.type}</span>
                                    <span>${formatNumber(l.price, 4)}</span>
                                    <span>${formatNumber(l.size)}</span>
                                </div>
                             ` + "`" + `;
                        }).join('');

                        return ` + "`" + `
                            <div class="position-card position-` + "${sideClass}" + `">
                                <div class="position-header">
                                    <span class="position-symbol">` + "${p.symbol}" + `</span>
                                    <div style="display: flex; align-items: center; gap: 8px;">
                                        ` + "${showSide ? `<span class=\"position-side side-${sideClass}\">${p.side}</span>` : ''}" + `
                                        <button onclick="closePosition('` + "${p.id}" + `', ${p.unrealized_pl}, ${p.position_size})" style="padding: 4px 8px; font-size: 11px; background: rgba(245, 101, 101, 0.2); border: 1px solid #f56565; color: #f56565; border-radius: 4px; cursor: pointer;">Close</button>
                                    </div>
                                </div>
                                <div class="position-details">
                                    <div class="detail-item">
                                        <span>Вход:</span>
                                        <span>` + "${formatNumber(p.entry_price, 4)}" + `</span>
                                    </div>
                                    <div class="detail-item">
                                        <span>Текущая:</span>
                                        <span>` + "${formatNumber(p.current_price, 4)}" + `</span>
                                    </div>
                                    <div class="detail-item">
                                        <span>Куплен:</span>
                                        <span>` + "${boughtAt}" + `</span>
                                    </div>
                                    <div class="detail-item">
                                        <span>В корзине:</span>
                                        <span>` + "${formatDuration(cartTime)}" + `</span>
                                    </div>
                                    <div class="detail-item">
                                        <span>Размер:</span>
                                        <span>` + "${formatNumber(p.position_size)}" + ` USDT</span>
                                    </div>
                                    <div class="detail-item">
                                        <span class="` + "${plClass}" + `">P&L:</span>
                                        <span class="` + "${plClass}" + `">` + "${formatPL(p.unrealized_pl)}" + ` (` + "${formatPL(p.pl_percent)}" + `%)</span>
                                    </div>
                                    <div class="detail-item">
                                        <span>TP:</span>
                                        <span>` + "${formatNumber(p.take_profit, 4)}" + `</span>
                                    </div>
                                    <div class="detail-item">
                                        <span>SL:</span>
                                        <span>` + "${formatNumber(p.stop_loss, 4)}" + `</span>
                                    </div>
                                </div>
                                <details style="margin-top: 10px; border-top: 1px solid rgba(255,255,255,0.1); padding-top: 8px;">
                                    <summary style="font-size: 12px; color: #a0aec0; cursor: pointer; outline: none;">Детали и история</summary>
                                    <div style="margin-top: 8px;">
                                        <div style="font-size: 11px; color: #cbd5e0; margin-bottom: 8px;">
                                            <strong>Причина входа:</strong><br>
                                            ${p.reasoning || 'N/A'}
                                        </div>
                                        <div style="font-size: 10px; color: #a0aec0; margin-bottom: 4px;">История сделок (Time | Type | Price | Size)</div>
                                        ${historyRows}
                                    </div>
                                </details>
                            </div>
                        ` + "`" + `;
                    }).join('');
                }

                // Update Signals (Removed as per user request)

                // Update History
                const historyTable = document.getElementById('history-table');
                if (history && history.length > 0) {
                    historyTable.innerHTML = history.reverse().map(t => {
                         const plClass = t.RealizedPL >= 0 ? 'positive' : 'negative';
                         const sideClass = t.Side === 'LONG' ? 'side-long' : 'side-short';
                         const showSide = stats.trading_mode !== 'SPOT';
                         return ` + "`" + `
                            <tr style="border-bottom: 1px solid rgba(255,255,255,0.05);">
                                <td style="padding: 12px;">` + "${formatDate(t.CloseTime)}" + `</td>
                                <td style="padding: 12px; font-weight: 600;">` + "${t.Symbol}" + `</td>
                                <td style="padding: 12px;">` + "${showSide ? `<span class=\"position-side ${sideClass}\">${t.Side}</span>` : ''}" + `</td>
                                <td style="padding: 12px;" class="` + "${plClass}" + `">` + "${formatPL(t.RealizedPL)}" + `</td>
                                <td style="padding: 12px;">` + "${formatNumber(t.PositionSize)}" + `</td>
                            </tr>
                         ` + "`" + `;
                    }).join('');
                } else {
                    historyTable.innerHTML = '<tr><td colspan="5" class="empty-state">Нет истории сделок</td></tr>';
                }

            } catch (error) {
                console.error('Error updating data:', error);
            }
        }

        // Update every 5 seconds
        updateData();
        setInterval(updateData, 5000);

        // Health check functionality
        document.getElementById('test-apis').addEventListener('click', async function() {
            const button = this;
            const statusDiv = document.getElementById('health-status');
            
            button.disabled = true;
            button.textContent = 'Testing...';
            statusDiv.innerHTML = '<div style="color: #a0aec0;">Running tests...</div>';
            
            try {
                const response = await fetch('/api/health');
                const data = await response.json();
                
                let html = '';
                data.services.forEach(service => {
                    let statusColor = '#48bb78'; // green
                    let statusIcon = '✅';
                    
                    if (service.status === 'error') {
                        statusColor = '#f56565'; // red
                        statusIcon = '❌';
                    } else if (service.status === 'configured') {
                        statusColor = '#ed8936'; // orange
                        statusIcon = '⚙️';
                    }
                    
                    html += ` + "`" + `
                        <div style="padding: 8px 0; border-bottom: 1px solid rgba(255,255,255,0.05);">
                            <div style="display: flex; justify-content: space-between; align-items: center;">
                                <span style="font-weight: 600;">` + "${statusIcon} ${service.name}" + `</span>
                                ` + "${service.latency > 0 ? `<span style=\"color: #a0aec0; font-size: 11px;\">${service.latency}ms</span>` : ''}" + `
                            </div>
                            <div style="color: ` + "${statusColor}" + `; font-size: 11px; margin-top: 4px;">
                                ` + "${service.message}" + `
                            </div>
                        </div>
                    ` + "`" + `;
                });
                
                statusDiv.innerHTML = html;
            } catch (error) {
                statusDiv.innerHTML = ` + "`" + `<div style="color: #f56565;">❌ Error: ` + "${error.message}" + `</div>` + "`" + `;
            } finally {
                button.disabled = false;
                button.textContent = 'Test All APIs';
            }
        });

        // Strictness slider functionality
        const slider = document.getElementById('strictness-slider');
        const sliderValue = document.getElementById('strictness-value');
        let sliderTimeout;

        slider.addEventListener('input', function() {
            sliderValue.textContent = this.value;
            
            // Debounce the API call
            clearTimeout(sliderTimeout);
            sliderTimeout = setTimeout(async () => {
                try {
                    await fetch('/api/strictness', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ strictness: parseInt(this.value) })
                    });
                } catch (error) {
                    console.error('Error updating strictness:', error);
                }
            }, 300);
        });

        // Max Slots slider functionality
        const maxSlotsSlider = document.getElementById('max-slots-slider');
        const maxSlotsValue = document.getElementById('max-slots-value');
        let maxSlotsTimeout;

        maxSlotsSlider.addEventListener('input', function() {
            maxSlotsValue.textContent = this.value;
            
            clearTimeout(maxSlotsTimeout);
            maxSlotsTimeout = setTimeout(async () => {
                try {
                    await fetch('/api/max-slots', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ max_slots: parseInt(this.value) })
                    });
                } catch (error) {
                    console.error('Error updating max slots:', error);
                }
            }, 300);
        });

        // Initial sync of sliders from stats
        async function syncSliders() {
            try {
                const res = await fetch('/api/stats');
                const data = await res.json();
                if (data.strictness) {
                    slider.value = data.strictness;
                    sliderValue.textContent = data.strictness;
                }
                if (data.max_slots) {
                    maxSlotsSlider.value = data.max_slots;
                    maxSlotsValue.textContent = data.max_slots;
                }
            } catch (e) {}
        }
        syncSliders();

        async function closePosition(id, unrealizedPL, size) {
            const payout = size + unrealizedPL;
            let message = '';
            
            if (unrealizedPL < 0) {
                 message = ` + "`" + `⚠️ ЗАКРЫТИЕ В МИНУС!\n\nВы теряете: ${unrealizedPL.toFixed(2)} USDT\nВы получите на баланс: ${payout.toFixed(2)} USDT\n\nВы уверены?` + "`" + `;
            } else {
                 message = ` + "`" + `Закрыть позицию?\n\nПрибыль: +${unrealizedPL.toFixed(2)} USDT\nВы получите на баланс: ${payout.toFixed(2)} USDT` + "`" + `;
            }

            if (!confirm(message)) {
                return;
            }

            try {
                const res = await fetch('/api/positions/' + id, {
                     method: 'DELETE' 
                });
                
                if (res.ok) {
                    updateData();
                } else {
                    const err = await res.text();
                    alert('Ошибка: ' + err);
                }
            } catch (error) {
                console.error('Error closing position:', error);
            }
        }

        async function engineAction(action) {
            try {
                const res = await fetch('/api/engine/action', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ action: action })
                });
                if (res.ok) {
                    setTimeout(updateData, 500); // Small delay to let engine update state
                }
            } catch (error) {
                console.error('Error with engine action:', error);
            }
        }

        async function switchMode(mode) {
             if (!confirm('Switching mode will reset positions and emulator balance. Continue?')) return;
             try {
                const res = await fetch('/api/mode', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ mode: mode })
                });
                if (res.ok) {
                    updateData();
                }
             } catch (e) {
                 alert('Error switching mode');
             }
        }

        async function switchType(isSimulated) {
             const typeName = isSimulated ? 'Emulator' : 'REAL MONEY';
             if (!confirm('Switching to ' + typeName + '. This will reset stats. Are you sure?')) return;
             
             try {
                const res = await fetch('/api/type', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ is_simulated: isSimulated })
                });
                if (res.ok) {
                    updateData();
                }
             } catch (e) {
                 alert('Error switching type');
             }
        }

        function updateTypeUI(isSimulated) {
            const btnSim = document.getElementById('btn-type-sim');
            const btnReal = document.getElementById('btn-type-real');
            
            const baseStyle = "padding: 8px; border: 1px solid rgba(255,255,255,0.1); border-radius: 6px; background: rgba(255,255,255,0.05); color: #a0aec0; cursor: pointer;";
            const activeSimStyle = "padding: 8px; border: 1px solid #ecc94b; border-radius: 6px; background: rgba(236, 201, 75, 0.2); color: #ecc94b; cursor: pointer; font-weight: bold;";
            const activeRealStyle = "padding: 8px; border: 1px solid #48bb78; border-radius: 6px; background: rgba(72, 187, 120, 0.2); color: #48bb78; cursor: pointer; font-weight: bold;";

            if (isSimulated) {
                btnSim.style = activeSimStyle;
                btnReal.style = baseStyle;
            } else {
                btnSim.style = baseStyle;
                btnReal.style = activeRealStyle;
            }
        }

        function updateModeUI(mode) {
            const btnFutures = document.getElementById('btn-mode-futures');
            const btnSpot = document.getElementById('btn-mode-spot');
            
            // Reset styles
            const baseStyle = "padding: 8px; border: 1px solid rgba(255,255,255,0.1); border-radius: 6px; background: rgba(255,255,255,0.05); color: #a0aec0; cursor: pointer;";
            const activeStyle = "padding: 8px; border: 1px solid #667eea; border-radius: 6px; background: rgba(102, 126, 234, 0.2); color: white; cursor: pointer; font-weight: bold;";

            if (mode === 'FUTURES') {
                btnFutures.style = activeStyle;
                btnSpot.style = baseStyle;
            } else {
                btnFutures.style = baseStyle;
                btnSpot.style = activeStyle;
            }
        }
    </script>
</body>
</html>`
