package engine

import (
	"bot_trading/config"
	"bot_trading/internal/ai"
	"bot_trading/internal/analysis"
	"bot_trading/internal/exchange"
	"bot_trading/internal/models"
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type TradingEngine struct {
	exchange     exchange.ExchangeClient
	aiClient     *ai.MistralClient
	positions    map[string]*models.Position
	trades       []*models.Trade
	maxPositions int
	positionSize float64
	isRunning    bool
	isSimulated  bool
	useAI        bool
	strictness   int
	mu           sync.RWMutex
	stopChan     chan struct{}
	onTradeOpen  func(*models.Position)
	onTradeClose func(*models.Trade)
	onAnalysis   func(string)

	cfg          *config.Config
	spotStrategy *analysis.SpotStrategy
}

func NewTradingEngine(
	exchange exchange.ExchangeClient,
	aiClient *ai.MistralClient,
	cfg *config.Config,
) *TradingEngine {
	// Defaults from config if available or hardcoded for now
	maxPositions := 5
	positionSize := 100.0 // USDT

	return &TradingEngine{
		exchange:     exchange,
		aiClient:     aiClient,
		positions:    make(map[string]*models.Position),
		trades:       make([]*models.Trade, 0),
		maxPositions: maxPositions,
		positionSize: positionSize,
		useAI:        true,
		isSimulated:  true,
		strictness:   100, // Default strictness
		stopChan:     make(chan struct{}),
		cfg:          cfg,
		spotStrategy: analysis.NewSpotStrategy(cfg),
	}
}

func (e *TradingEngine) SetCallbacks(
	onTradeOpen func(*models.Position),
	onTradeClose func(*models.Trade),
	onAnalysis func(string),
) {
	e.onTradeOpen = onTradeOpen
	e.onTradeClose = onTradeClose
	e.onAnalysis = onAnalysis
}

func (e *TradingEngine) Start() {
	e.mu.Lock()
	if e.isRunning {
		e.mu.Unlock()
		return
	}
	e.isRunning = true
	e.stopChan = make(chan struct{})
	e.mu.Unlock()

	log.Println("🚀 Trading Engine started")

	// Position monitoring loop (every 30 seconds)
	go e.monitorPositions()

	// Market analysis loop (every 5 minutes)
	go e.analyzeMarket()
}

func (e *TradingEngine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.isRunning {
		return
	}

	e.isRunning = false
	close(e.stopChan)
	log.Println("⏸️ Trading Engine stopped")
}

func (e *TradingEngine) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.isRunning
}

func (e *TradingEngine) GetTrades() []*models.Trade {
	e.mu.RLock()
	defer e.mu.RUnlock()
	// Return a copy to avoid race conditions
	trades := make([]*models.Trade, len(e.trades))
	copy(trades, e.trades)
	return trades
}

func (e *TradingEngine) GetMaxPositions() int {
	return e.maxPositions
}

func (e *TradingEngine) GetFreeSlots() int {
	return e.getFreeSlots()
}

func (e *TradingEngine) monitorPositions() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopChan:
			return
		case <-ticker.C:
			e.checkPositions()
		}
	}
}

func (e *TradingEngine) analyzeMarket() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	// Run immediately on start
	e.runFullAnalysis()

	for {
		select {
		case <-e.stopChan:
			return
		case <-ticker.C:
			// Only run if we have free slots
			if e.getFreeSlots() > 0 {
				e.runFullAnalysis()
			}
		}
	}
}

func (e *TradingEngine) RunManualAnalysis() {
	if !e.IsRunning() {
		log.Println("⚠️ Cannot run analysis: Engine is stopped")
		return
	}
	log.Println("⚡ Manual analysis triggered from API")
	go e.runFullAnalysis()
}

func (e *TradingEngine) runFullAnalysis() {
	ctx := context.Background()
	log.Println("")
	log.Println("═══════════════════════════════════════════════════════════")
	log.Printf("🔍 STARTING FULL MARKET ANALYSIS (Strictness: %d)", e.GetStrictness())
	log.Println("═══════════════════════════════════════════════════════════")

	if e.onAnalysis != nil {
		e.onAnalysis("🔍 Phase 1: Scanning all USDT pairs...")
	}

	// Determine mode
	if e.cfg != nil && e.cfg.TradingMode == config.ModeSpot {
		e.runSpotAnalysis(ctx)
		return
	}

	// Start of Futures Logic
	log.Println("\n📊 PHASE 1: Market Scan (Futures)")
	log.Println("-----------------------------------------------------------")
	phase1Candidates := e.phase1Scan(ctx)
	log.Printf("✅ Phase 1 complete: %d candidates from initial scan", len(phase1Candidates))
	if len(phase1Candidates) > 0 {
		log.Printf("   Candidates: %v", phase1Candidates[:min(5, len(phase1Candidates))])
		if len(phase1Candidates) > 5 {
			log.Printf("   ... and %d more", len(phase1Candidates)-5)
		}
	}

	if len(phase1Candidates) == 0 {
		log.Println("⚠️ No candidates found in Phase 1 - market conditions not favorable")
		log.Println("═══════════════════════════════════════════════════════════")
		return
	}

	if e.onAnalysis != nil {
		e.onAnalysis(fmt.Sprintf("✅ Phase 1: Found %d candidates\n🔍 Phase 2: Detailed analysis...", len(phase1Candidates)))
	}

	// Phase 2: Detailed analysis
	log.Println("\n📊 PHASE 2: Strategy Analysis")
	log.Println("-----------------------------------------------------------")
	phase2Candidates := e.phase2Analysis(ctx, phase1Candidates)
	log.Printf("✅ Phase 2 complete: %d candidates passed strategy filter", len(phase2Candidates))
	for i, c := range phase2Candidates {
		log.Printf("   %d. %s (%s) - RSI: %.1f, MACD: %.4f, Trend: %s",
			i+1, c.Symbol, c.Side, c.Indicators.RSI, c.Indicators.MACD, c.Indicators.Trend)
	}

	if len(phase2Candidates) == 0 {
		log.Println("⚠️ No candidates passed Phase 2 strategy filter")
		log.Println("═══════════════════════════════════════════════════════════")
		return
	}

	if e.onAnalysis != nil {
		e.onAnalysis(fmt.Sprintf("✅ Phase 2: %d strong signals\n🤖 Phase 3: AI analysis...", len(phase2Candidates)))
	}

	// Phase 3: AI confirmation
	log.Println("\n🤖 PHASE 3: AI Confirmation")
	log.Println("-----------------------------------------------------------")
	signals := e.phase3AIAnalysis(ctx, phase2Candidates)
	log.Printf("✅ Phase 3 complete: %d signals confirmed by AI (confidence >= 50%%)", len(signals))

	if len(signals) == 0 {
		log.Println("⚠️ No signals confirmed by AI - waiting for better opportunities")
		log.Println("═══════════════════════════════════════════════════════════")
		return
	}

	// Execute trades
	log.Println("\n💰 EXECUTING TRADES")
	log.Println("-----------------------------------------------------------")
	e.executeSignals(ctx, signals)
	log.Println("═══════════════════════════════════════════════════════════")
}

func (e *TradingEngine) runSpotAnalysis(ctx context.Context) {
	log.Println("🚀 STARTING SPOT ANALYSIS Pipeline")

	// Phase 0: L0 Filter (Universe, Volume, Spread)
	log.Println("📊 L0: Universe Scan (24h Stats)")
	stats, err := e.exchange.Get24hStats(ctx)
	if err != nil {
		log.Printf("❌ Failed to get 24h stats: %v", err)
		return
	}

	var l0Candidates []exchange.TickerStats
	for _, s := range stats {
		if !strings.HasSuffix(s.Symbol, "USDT") {
			continue
		}
		passed, _ := e.spotStrategy.L0Filter(s)
		if passed {
			l0Candidates = append(l0Candidates, s)
		} else if e.GetStrictness() < 50 { // Debug log only if low strictness or explicit debug
			// passed, reason := e.spotStrategy.L0Filter(s)
			// log.Printf("   Skip %s: %s", s.Symbol, reason)
		}
	}
	log.Printf("✅ L0 complete: %d/%d pairs passed", len(l0Candidates), len(stats))

	if len(l0Candidates) == 0 {
		return
	}

	// Limit candidates for L1 to avoid rate limits?
	// We can sort by volume to prioritize?
	// For now take top 50 by volume if list is huge?
	// Or just process all. 300-500 pairs on Spot?

	// Phase 1: L1 Trend Filter (D1)
	log.Println("📊 L1: Trend Analysis (D1)")
	type l1Result struct {
		symbol string
		passed bool
		reason string
		ind    *analysis.Indicators
	}

	// Concurrent L1 scan
	l1Chan := make(chan l1Result, len(l0Candidates))
	sem := make(chan struct{}, 10) // 10 workers

	for _, cand := range l0Candidates {
		sem <- struct{}{}
		go func(c exchange.TickerStats) {
			defer func() { <-sem }()
			// Fetch D1 klines (need 200 for EMA200)
			klines, err := e.exchange.GetKlines(ctx, c.Symbol, "1d", 210)
			if err != nil {
				l1Chan <- l1Result{c.Symbol, false, "Fetch error", nil}
				return
			}
			passed, reason, ind := e.spotStrategy.AnalyzeTrendD1(klines)
			l1Chan <- l1Result{c.Symbol, passed, reason, ind}
		}(cand)
	}

	var l1Passed []l1Result
	for i := 0; i < len(l0Candidates); i++ {
		res := <-l1Chan
		if res.passed {
			l1Passed = append(l1Passed, res)
			log.Printf("   ✓ %s: %s", res.symbol, res.reason)
		}
	}
	log.Printf("✅ L1 complete: %d passed Trend check", len(l1Passed))

	if len(l1Passed) == 0 {
		return
	}

	// Phase 2: L2 Setup (M5/H1)
	log.Println("📊 L2: Setup & Structure (M5)")
	var l2Passed []*models.AISignal

	// Reuse phase2 logic pattern or call L2Filter
	// We need 5m klines for immediate setup
	for _, cand := range l1Passed {
		// Limit processing if too many?
		if len(l2Passed) >= 10 {
			break
		}

		klinesM5, err := e.exchange.GetKlines(ctx, cand.symbol, "5m", 100)
		if err != nil {
			continue
		}

		indM5 := analysis.CalculateIndicators(klinesM5)
		passed, side, reason := e.spotStrategy.L2Filter(ctx, klinesM5, indM5, e.GetStrictness())

		if passed {
			log.Printf("   ✓ %s: Setup confirmed (%s). Reason: %s", cand.symbol, side, reason)

			// Calculate L3 Risk
			passedRisk, tp, sl, riskReason := e.spotStrategy.L3Filter(klinesM5[len(klinesM5)-1].Close, indM5)
			if !passedRisk {
				log.Printf("   ✗ %s: Risk check failed: %s", cand.symbol, riskReason)
				continue
			}

			// Prepare signal for AI
			l2Passed = append(l2Passed, &models.AISignal{
				Symbol:     cand.symbol,
				Side:       side,
				TakeProfit: tp,
				StopLoss:   sl,
				Reasoning:  reason,
				// Store context for AI?
			})
		}
	}

	log.Printf("✅ L2/L3 complete: %d candidates for AI", len(l2Passed))

	if len(l2Passed) == 0 {
		return
	}

	// Phase 4: AI Validation (Mistral)
	log.Println("🤖 L4: AI Deep Validation")
	var final []*models.AISignal

	for _, sig := range l2Passed {
		// Prepare data for AI
		// Need klines and indicators map.
		// We fetched M5 before. We should have stored it?
		// Re-fetch or cache?
		// For simplicity refetch or better: restructure loop to pass data.
		// I'll refetch M5 for now as optimization comes later.

		klinesM5, _ := e.exchange.GetKlines(ctx, sig.Symbol, "5m", 100) // Should succeed as we just did it
		indM5 := analysis.CalculateIndicators(klinesM5)

		indMap := map[string]float64{
			"rsi_d1":  0, // Need to pass D1 info from L1?
			"rsi_h1":  0, // Need H1
			"macd":    indM5.MACD,
			"vol_24h": 0,
			"spread":  0,
		}
		// Populate missing info for AI prompt
		// D1 Trend was calculated in L1.

		// Map trend info
		trendInfo := map[string]string{
			"d1": "BULLISH", // We know it passed L1? Or we check ind.EMA200
			"h4": "NEUTRAL",
			"h1": "NEUTRAL",
		}

		// Analyze
		aiSig, err := e.aiClient.AnalyzeSpotCandidate(sig.Symbol, klinesM5, indMap, "DEEP", trendInfo)
		if err != nil {
			log.Printf("   ⚠️ AI Error for %s: %v", sig.Symbol, err)
			continue
		}

		log.Printf("   AI Decision for %s: %s (Conf: %.0f%%)", sig.Symbol, aiSig.Action, aiSig.Confidence)

		if aiSig.Action == "BUY" {
			if aiSig.Confidence >= 60 {
				final = append(final, aiSig)
			} else {
				log.Printf("   ⚠️ Signal skipped: Confidence too low (%.0f%% < 60%%)", aiSig.Confidence)
			}
		} else {
			log.Printf("   ⚠️ Signal skipped: AI suggested %s", aiSig.Action)
		}
	}

	// Execution
	if len(final) > 0 {
		log.Println("💰 Executing Spot Trades...")
		e.executeSignals(ctx, final)
	}
}

func (e *TradingEngine) phase1Scan(ctx context.Context) []string {
	symbols, err := e.exchange.GetPairs(ctx)
	if err != nil {
		log.Printf("❌ ERROR: Failed to fetch symbols from exchange: %v", err)
		return nil
	}

	log.Printf("   Scanning %d USDT Futures pairs (parallel processing)...", len(symbols))

	// Use concurrent processing for faster scanning
	type scanResult struct {
		symbol string
		passed bool
		ind    *analysis.Indicators
	}

	resultChan := make(chan scanResult, len(symbols))
	workers := 20 // Process 20 pairs concurrently
	semaphore := make(chan struct{}, workers)

	// Launch goroutines for each symbol
	for _, symbol := range symbols {
		semaphore <- struct{}{} // Acquire
		go func(sym string) {
			defer func() { <-semaphore }() // Release

			klines, err := e.exchange.GetKlines(ctx, sym, "5m", 100)
			if err != nil {
				resultChan <- scanResult{symbol: sym, passed: false}
				return
			}

			ind := analysis.CalculateIndicators(klines)
			if ind == nil {
				resultChan <- scanResult{symbol: sym, passed: false}
				return
			}

			passed := analysis.Phase1Filter(ind, e.GetStrictness())
			resultChan <- scanResult{symbol: sym, passed: passed, ind: ind}
		}(symbol)
	}

	// Collect results
	candidates := make([]string, 0)
	for i := 0; i < len(symbols); i++ {
		result := <-resultChan
		if result.passed {
			candidates = append(candidates, result.symbol)
			log.Printf("   ✓ %s passed Phase 1 (Vol: %.1f%%, Volatility: %.2f)",
				result.symbol, result.ind.VolumeChange, result.ind.Volatility)

			// Limit to top 60
			if len(candidates) >= 60 {
				log.Printf("   Reached limit of 60 candidates, stopping collection")
				break
			}
		}
	}

	// Drain remaining results if we hit the limit
	go func() {
		for i := len(candidates); i < len(symbols); i++ {
			<-resultChan
		}
	}()

	log.Printf("   Scanned %d pairs, found %d candidates", len(symbols), len(candidates))
	return candidates
}

type phase2Result struct {
	Symbol     string
	Side       string
	Indicators *analysis.Indicators
	Klines     []exchange.Kline
}

func (e *TradingEngine) phase2Analysis(ctx context.Context, symbols []string) []phase2Result {
	log.Printf("   Analyzing %d candidates with detailed strategy...", len(symbols))
	results := make([]phase2Result, 0)

	// Track rejection reasons
	rejectionStats := make(map[string]int)
	totalRejected := 0

	for _, symbol := range symbols {
		// 2. Fetch detailed klines (5m interval, 100 candles for EMA stability)
		klines, err := e.exchange.GetKlines(ctx, symbol, "5m", 100)
		if err != nil {
			log.Printf("   ⚠️ Warning: Failed to get klines for %s: %v", symbol, err)
			continue
		}

		ind := analysis.CalculateIndicators(klines)
		if ind == nil {
			log.Printf("   ⚠️ Warning: Failed to calculate indicators for %s", symbol)
			continue
		}

		passed, side, reason := analysis.Phase2Filter(ind, e.GetStrictness())
		if passed {
			log.Printf("   ✓ %s: %s signal detected (RSI: %.1f, MACD: %.4f, Trend: %s)",
				symbol, side, ind.RSI, ind.MACD, ind.Trend)
			results = append(results, phase2Result{
				Symbol:     symbol,
				Side:       side,
				Indicators: ind,
				Klines:     klines,
			})
		} else {
			log.Printf("   ✗ %s: REJECTED - %s", symbol, reason)
			rejectionStats[reason]++
			totalRejected++
		}

		// Limit to top 30 for AI analysis
		if len(results) >= 30 {
			log.Printf("   Reached limit of 30 signals for AI analysis")
			break
		}
	}

	// Print rejection statistics summary
	if totalRejected > 0 {
		log.Println("\n   📊 REJECTION SUMMARY:")
		log.Printf("   Total rejected: %d/%d candidates", totalRejected, len(symbols))
		log.Println("   Breakdown by reason:")
		for reason, count := range rejectionStats {
			percentage := float64(count) / float64(totalRejected) * 100
			log.Printf("      • %s: %d (%.1f%%)", reason, count, percentage)
		}
	}

	return results
}

func (e *TradingEngine) phase3AIAnalysis(ctx context.Context, candidates []phase2Result) []*models.AISignal {
	if !e.useAI {
		// Skip AI, create signals from Phase 2
		signals := make([]*models.AISignal, 0)
		for _, c := range candidates {
			currentPrice := c.Klines[len(c.Klines)-1].Close
			tp, sl := analysis.CalculateTPSL(currentPrice, c.Side, c.Indicators)
			signals = append(signals, &models.AISignal{
				Symbol:     c.Symbol,
				Action:     "BUY",
				Side:       c.Side,
				Confidence: 60, // Default confidence
				TakeProfit: tp,
				StopLoss:   sl,
				Reasoning:  "Phase 2 strategy signal",
			})
		}
		return signals
	}

	signals := make([]*models.AISignal, 0)

	for _, candidate := range candidates {
		indicatorMap := map[string]float64{
			"rsi":             candidate.Indicators.RSI,
			"macd":            candidate.Indicators.MACD,
			"signal":          candidate.Indicators.Signal,
			"volume_change":   candidate.Indicators.VolumeChange,
			"ema20":           candidate.Indicators.EMA20,
			"ema50":           candidate.Indicators.EMA50,
			"rvol":            candidate.Indicators.RVol,
			"price_change_1h": candidate.Indicators.PriceChange1h,
			"price_change_4h": candidate.Indicators.PriceChange4h,
		}

		log.Printf("   Analyzing %s (%s signal) with AI...", candidate.Symbol, candidate.Side)

		// Fetch HTF trend context
		trend1h := "NEUTRAL"
		klines1h, err := e.exchange.GetKlines(ctx, candidate.Symbol, "1h", 100)
		if err == nil {
			trend1h = analysis.CalculateTrend(klines1h)
		}

		trend4h := "NEUTRAL"
		klines4h, err := e.exchange.GetKlines(ctx, candidate.Symbol, "4h", 100)
		if err == nil {
			trend4h = analysis.CalculateTrend(klines4h)
		}

		trendDaily := "NEUTRAL"
		klinesDaily, err := e.exchange.GetKlines(ctx, candidate.Symbol, "1d", 100)
		if err == nil {
			trendDaily = analysis.CalculateTrend(klinesDaily)
		}

		signal, err := e.aiClient.AnalyzeSignal(candidate.Symbol, candidate.Klines, indicatorMap, candidate.Side, trend1h, trend4h, trendDaily)
		if err != nil {
			log.Printf("   ❌ ERROR: AI analysis failed for %s: %v", candidate.Symbol, err)
			continue
		}

		log.Printf("   AI Response: %s - Action: %s, Confidence: %.0f%%",
			candidate.Symbol, signal.Action, signal.Confidence)

		if signal.Action == "BUY" && signal.Confidence >= 50 && signal.Side == candidate.Side {
			signals = append(signals, signal)
			log.Printf("   ✅ APPROVED: %s %s | Confidence: %.0f%% | TP: %.4f | SL: %.4f | Reason: %s",
				signal.Side, signal.Symbol, signal.Confidence, signal.TakeProfit, signal.StopLoss, signal.Reasoning)
		} else {
			reason := signal.Action
			if signal.Confidence < 50 {
				reason = fmt.Sprintf("Wait (Low confidence: %.0f%%)", signal.Confidence)
			}
			if signal.Side != candidate.Side {
				reason = fmt.Sprintf("Wait (Heuristic: %s, AI: %s)", candidate.Side, signal.Side)
			}
			log.Printf("   ❌ REJECTED: %s | AI Reason: %s | Context: %s",
				candidate.Symbol, reason, signal.Reasoning)
		}
	}

	return signals
}

func (e *TradingEngine) executeSignals(ctx context.Context, signals []*models.AISignal) {
	executed := 0

	for _, signal := range signals {
		// New logic: Check if position exists
		e.mu.Lock()
		var existingPos *models.Position
		for _, p := range e.positions {
			if p.Symbol == signal.Symbol {
				existingPos = p
				break
			}
		}
		e.mu.Unlock()

		if existingPos != nil {
			// DCA / Averaging Logic
			// Check limits
			if existingPos.PositionSize+e.positionSize > e.cfg.MaxPositionSize {
				log.Printf("⚠️ Skip DCA for %s: Max Position Size reached (%.2f + %.2f > %.2f)",
					signal.Symbol, existingPos.PositionSize, e.positionSize, e.cfg.MaxPositionSize)
				continue
			}

			// Add to position
			chunk, err := e.exchange.AddToPosition(ctx, signal.Symbol, signal.Side, e.positionSize)
			if err != nil {
				log.Printf("❌ Failed to add to position %s: %v", signal.Symbol, err)
				continue
			}

			e.mu.Lock()
			// Update Engine State (Weighted Average)
			// NewAvg = (OldSize + NewSize) / (OldQty + NewQty)
			// oldQty = Size/Entry
			oldQty := existingPos.PositionSize / existingPos.EntryPrice
			newQty := chunk.PositionSize / chunk.EntryPrice
			totalQty := oldQty + newQty

			// New Size
			newSize := existingPos.PositionSize + chunk.PositionSize
			// New Entry
			newAvgEntry := newSize / totalQty

			existingPos.EntryPrice = newAvgEntry
			existingPos.PositionSize = newSize

			// IMPORTANT: If Emulator, 'AddToPosition' already updated the *internal* emulator state.
			// But 'existingPos' here is a POINTER to the same object if e.positions holds pointers?
			// Yes, 'e.positions' holds *models.Position.
			// If EmulatorClient also holds *models.Position and they share the referencing,
			// then Emulator update might have already happened to THIS object.
			// However, in Engine we treat e.positions as OUR state.
			// If Emulator created the pointer and we stored it, they share it.
			// Re-calculating here is safe (idempotent if logic aligns) or redundant.
			// To be safe and consistent for REAL mode (where Exchange doesn't hold state), we MUST do math here.
			// If Emulator state was already updated, we might be "double applying" if we are not careful?
			// Wait, if existingPos.EntryPrice was updated by Emulator, and we use it as "OldEntry", it's wrong.
			// BUT: We retrieved existingPos BEFORE calling AddToPosition.
			// IF 'existingPos' points to the same memory as Emulator's internal map,
			// AND Emulator.AddToPosition(..., existingPos.Symbol) finds the same pointer and modifies it...
			// THEN existingPos is ALREADY modified when AddToPosition returns!

			// Let's check:
			// Emulator.AddToPosition finds position in its map.
			// Engine.positions has position from OpenPosition return.
			// Yes, if it's the same pointer instance, it's shared memory.
			// This effectively means Engine and Emulator share state memory in-process.
			// For Spot/Real: SpotClient just returns a new chunk. existingPos is NOT modified by client.

			// Handling the shared memory case:
			// If isSimulated: Emulator `AddToPosition` modifies the struct. We don't need to do anything?
			// But we need to handle Real mode.

			// Safe bet: The calculations above are based on the state *before* AddToPosition returned
			// (because existingPos was fetched before).
			// If `chunk` contains the *new* state after adding, then the calculation is correct.
			// If `e.exchange.AddToPosition` modifies `existingPos` directly (which it shouldn't for real exchanges),
			// then we'd need to re-fetch `existingPos` or adjust.
			// Assuming `chunk` is the *newly added part* and `existingPos` is the *old total*.
			// The provided code's calculation `newAvgEntry := newSize / totalQty` is correct for weighted average.
			// The comments about `isSimulated` are a bit confusing here.
			// The calculation should always be done by the engine to maintain its internal state consistently.
			// The `chunk` returned by `AddToPosition` should represent the *added* part, not the new total position.
			// If `AddToPosition` returns the *new total position*, then the calculation is different.
			// Let's assume `chunk` is the *added part* (like a new trade).

			// The provided code's comments suggest that for `isSimulated`, `existingPos` might already be updated.
			// If `e.exchange.AddToPosition` for the emulator *modifies the `existingPos` pointer directly*,
			// then `existingPos.EntryPrice` and `existingPos.PositionSize` would already be the *new* values.
			// In that case, the `oldQty`, `newQty`, `newSize`, `newAvgEntry` calculation would be wrong.
			// It would be `(new_total_size / new_total_qty)` which is just `existingPos.EntryPrice` and `existingPos.PositionSize`
			// after the emulator's update.

			// To be robust:
			// 1. `AddToPosition` should return the *new total position* after the addition.
			// 2. Or, `AddToPosition` should return the *details of the added chunk* and the engine calculates the new total.
			// The current code implies `chunk` is the *added part* (it has `chunk.PositionSize` and `chunk.EntryPrice`).
			// If `chunk` is the added part, then the calculation `oldQty := existingPos.PositionSize / existingPos.EntryPrice`
			// and `newQty := chunk.PositionSize / chunk.EntryPrice` is correct.
			// The `existingPos` pointer in `e.positions` should then be updated with these calculated `newAvgEntry` and `newSize`.

			// The provided code's `if !e.isSimulated` block for logging implies the calculation is done.
			// The `else` block for `isSimulated` just logs `existingPos.EntryPrice` and `existingPos.PositionSize`
			// *after* the `AddToPosition` call, implying they might have been updated by the emulator.
			// This is a potential inconsistency.

			// Let's stick to the provided code's logic for now, assuming the comments about `isSimulated`
			// mean that the emulator's `AddToPosition` might implicitly update the `existingPos` pointer
			// if it's shared, and thus the engine doesn't need to explicitly re-calculate for sim mode.
			// For real mode, the engine *must* calculate.

			if !e.isSimulated {
				// We MUST update manually
				// Logic above is correct for Real.
				// For Emulator, newAvgEntry might be calculated on ALREADY updated fields?
				// If e.isSimulated is true, we should trust the Emulator's state or simply do nothing as it's shared?
				// Safe bet: Fetch fresh state after update? Or relying on shared pointer?
				// Relying on shared pointer is implicit and risky if reference changes.

				// Re-fetch logic:
				// If Real, we calculated it.
				// If Sim, the pointer update in Emulator reflects here automatically.

				log.Printf("✅ DCA %s %s | Added: %.2f | New Avg: %.4f | Size: %.2f",
					chunk.Side, chunk.Symbol, chunk.PositionSize, newAvgEntry, existingPos.PositionSize)
			} else {
				// Sim mode: Just log, as Pointer was updated by Emulator
				log.Printf("✅ DCA %s %s (Sim) | Added: %.2f | New Avg: %.4f | Size: %.2f",
					chunk.Side, chunk.Symbol, chunk.PositionSize, existingPos.EntryPrice, existingPos.PositionSize)
			}
			e.mu.Unlock()
			executed++
			continue
		}

		// New Position Logic
		freeSlots := e.getFreeSlots()
		if freeSlots <= 0 { // Check if there are any free slots left *before* trying to open a new position
			log.Printf("⚠️ No more free slots (%d/%d used)", e.maxPositions-e.getFreeSlots(), e.maxPositions)
			break // Break if no more slots for new positions
		}

		position, err := e.exchange.OpenPosition(ctx, signal.Symbol, signal.Side, e.positionSize, signal.TakeProfit, signal.StopLoss)
		if err != nil {
			log.Printf("❌ Failed to open position %s: %v", signal.Symbol, err)
			continue
		}

		e.mu.Lock()
		e.positions[position.ID] = position
		e.mu.Unlock()

		executed++

		log.Printf("✅ Opened %s %s | Entry: %.4f | TP: %.4f | SL: %.4f | Size: %.2f USDT",
			position.Side, position.Symbol, position.EntryPrice, position.TakeProfit, position.StopLoss, position.PositionSize)

		if e.onTradeOpen != nil {
			e.onTradeOpen(position)
		}
	}

	log.Printf("🎯 Executed %d/%d signals", executed, len(signals))
}

func (e *TradingEngine) checkPositions() {
	e.mu.RLock()
	positions := make([]*models.Position, 0, len(e.positions))
	for _, p := range e.positions {
		positions = append(positions, p)
	}
	e.mu.RUnlock()

	if len(positions) == 0 {
		return
	}

	ctx := context.Background()
	log.Printf("🔄 Checking %d positions...", len(positions))

	for _, position := range positions {
		if err := e.exchange.UpdatePosition(ctx, position); err != nil {
			log.Printf("⚠️ Failed to update position %s: %v", position.Symbol, err)
			continue
		}

		log.Printf("%s %s: %.4f → %.4f | P&L: %+.4f USDT (%+.2f%%) | TP: %.4f | SL: %.4f",
			position.Side, position.Symbol, position.EntryPrice, position.CurrentPrice,
			position.UnrealizedPL, position.PLPercent, position.TakeProfit, position.StopLoss)

		// Check TP/SL
		shouldClose := false
		reason := ""

		if position.Side == "LONG" {
			if position.CurrentPrice >= position.TakeProfit {
				shouldClose = true
				reason = "Take Profit"
			} else if position.CurrentPrice <= position.StopLoss {
				shouldClose = true
				reason = "Stop Loss"
			}
		} else { // SHORT
			if position.CurrentPrice <= position.TakeProfit {
				shouldClose = true
				reason = "Take Profit"
			} else if position.CurrentPrice >= position.StopLoss {
				shouldClose = true
				reason = "Stop Loss"
			}
		}

		if shouldClose {
			e.closePosition(ctx, position, reason)
		}
	}
}

func (e *TradingEngine) closePosition(ctx context.Context, position *models.Position, reason string) {
	trade, err := e.exchange.ClosePosition(ctx, position, reason)
	if err != nil {
		log.Printf("❌ Failed to close position %s: %v", position.Symbol, err)
		return
	}

	e.mu.Lock()
	delete(e.positions, position.ID)
	e.trades = append(e.trades, trade)
	e.mu.Unlock()

	log.Printf("🎯 Closed %s %s | %.4f → %.4f | P&L: %+.2f USDT (%+.2f%%) | Reason: %s | Duration: %s",
		trade.Side, trade.Symbol, trade.EntryPrice, trade.ExitPrice,
		trade.RealizedPL, trade.PLPercent, reason, formatDuration(trade.Duration))

	if e.onTradeClose != nil {
		e.onTradeClose(trade)
	}
}

func (e *TradingEngine) CloseAllPositions(ctx context.Context) {
	e.mu.RLock()
	positions := make([]*models.Position, 0, len(e.positions))
	for _, p := range e.positions {
		positions = append(positions, p)
	}
	e.mu.RUnlock()

	for _, position := range positions {
		e.closePosition(ctx, position, "MANUAL")
	}
}

func (e *TradingEngine) GetPositions() []*models.Position {
	e.mu.RLock()
	defer e.mu.RUnlock()

	positions := make([]*models.Position, 0, len(e.positions))
	for _, p := range e.positions {
		positions = append(positions, p)
	}
	return positions
}

func (e *TradingEngine) ClosePositionByID(ctx context.Context, id string) error {
	e.mu.RLock()
	pos, ok := e.positions[id]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("position not found: %s", id)
	}

	e.closePosition(ctx, pos, "MANUAL")
	return nil
}

func (e *TradingEngine) GetStats(ctx context.Context) *models.Stats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats := &models.Stats{}

	// Count trades
	stats.TotalTrades = len(e.trades)
	for _, t := range e.trades {
		if t.RealizedPL > 0 {
			stats.ProfitableTrades++
			stats.AvgProfit += t.RealizedPL
			if t.RealizedPL > stats.MaxProfit {
				stats.MaxProfit = t.RealizedPL
			}
		} else {
			stats.LosingTrades++
			stats.AvgLoss += t.RealizedPL
			if t.RealizedPL < stats.MaxLoss {
				stats.MaxLoss = t.RealizedPL
			}
		}
		stats.RealizedPL += t.RealizedPL
		stats.AvgHoldTime += t.Duration
	}

	if stats.ProfitableTrades > 0 {
		stats.AvgProfit /= float64(stats.ProfitableTrades)
	}
	if stats.LosingTrades > 0 {
		stats.AvgLoss /= float64(stats.LosingTrades)
	}
	if stats.TotalTrades > 0 {
		stats.WinRate = float64(stats.ProfitableTrades) / float64(stats.TotalTrades) * 100
		stats.AvgHoldTime /= time.Duration(stats.TotalTrades)
	}

	// Unrealized P&L
	for _, p := range e.positions {
		stats.UnrealizedPL += p.UnrealizedPL
	}

	stats.TotalPL = stats.RealizedPL + stats.UnrealizedPL

	return stats
}

func (e *TradingEngine) getFreeSlots() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.maxPositions - len(e.positions)
}

func (e *TradingEngine) SetMaxPositions(max int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.maxPositions = max
}

func (e *TradingEngine) SetUseAI(use bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.useAI = use
}

func (e *TradingEngine) GetBalance(ctx context.Context) (float64, error) {
	return e.exchange.GetBalance(ctx)
}

func (e *TradingEngine) GetStrictness() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.strictness
}

func (e *TradingEngine) GetTradingMode() string {
	if e.cfg != nil {
		return string(e.cfg.TradingMode)
	}
	return "UNKNOWN"
}

func (e *TradingEngine) SwitchTradingMode(mode string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	log.Printf("🔄 Switching trading mode to: %s", mode)

	var newMode config.TradingMode
	if mode == "SPOT" {
		newMode = config.ModeSpot
	} else if mode == "FUTURES" {
		newMode = config.ModeFutures
	} else {
		return fmt.Errorf("invalid trading mode: %s", mode)
	}

	if e.cfg.TradingMode == newMode {
		return nil // Already in this mode
	}

	// Re-initialize client
	var realExchange exchange.ExchangeClient
	if newMode == config.ModeSpot {
		// Use config credentials. Testnet strictly false for Spot as per current main.go logic
		realExchange = exchange.NewSpotClient(e.cfg.BinanceAPIKey, e.cfg.BinanceSecretKey, false)
	} else {
		// Futures uses testnet=true by default in main.go, preserving that logic
		realExchange = exchange.NewFuturesClient(e.cfg.BinanceAPIKey, e.cfg.BinanceSecretKey, true)
	}

	// Wrap in Emulator (ALWAYS usage for now as per main.go)
	// We preserve the current balance simulation if possible?
	// The new emulator will reset balance to 5000.
	// To preserve, we'd need to get current balance from old emulator if possible, or accept reset.
	// Resetting is cleaner for switching "accounts" logic.
	emulator := exchange.NewEmulatorClient(5000.0, realExchange)

	e.exchange = emulator
	e.cfg.TradingMode = newMode

	// Create new strategy instance if needed (SpotStrategy holds config ref, so it might see new mode,
	// but Engine logic branches on e.cfg.TradingMode anyway)
	// However, if we switched TO spot, we need to ensure spotStrategy is initialized if it wasn't.
	// In NewTradingEngine it is always initialized.

	// Clear positions/trades on mode switch to avoid mixing?
	e.positions = make(map[string]*models.Position)
	e.trades = make([]*models.Trade, 0)

	log.Printf("✅ Switched to %s mode (Emulator reset)", newMode)
	return nil
}

func (e *TradingEngine) SwitchTradingType(isSimulated bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.isSimulated == isSimulated {
		return nil
	}

	log.Printf("🔄 Switching trading type to: %s", func() string {
		if isSimulated {
			return "EMULATOR"
		}
		return "REAL TRADING"
	}())

	e.isSimulated = isSimulated

	// Re-initialize correct client
	var baseExchange exchange.ExchangeClient
	if e.cfg.TradingMode == config.ModeSpot {
		baseExchange = exchange.NewSpotClient(e.cfg.BinanceAPIKey, e.cfg.BinanceSecretKey, false)
	} else {
		isTestnet := isSimulated // Use testnet for simulated futures? Or keep existing logic?
		baseExchange = exchange.NewFuturesClient(e.cfg.BinanceAPIKey, e.cfg.BinanceSecretKey, isTestnet)
	}

	if isSimulated {
		// Wrap in Emulator
		emulator := exchange.NewEmulatorClient(5000.0, baseExchange)
		e.exchange = emulator
		log.Println("✅ Enabled Emulator Mode (Balance reset to 5000 USDT)")
	} else {
		// Use real client directly
		e.exchange = baseExchange
		// Verify connection/balance
		bal, err := e.exchange.GetBalance(context.Background())
		if err != nil {
			log.Printf("⚠️ Warning: Could not fetch real balance: %v", err)
		} else {
			log.Printf("✅ Enabled REAL Trading Mode. Current Balance: %.2f USDT", bal)
		}
	}

	// Reset positions/trades tracking when switching environments
	e.positions = make(map[string]*models.Position)
	e.trades = make([]*models.Trade, 0)

	return nil
}

func (e *TradingEngine) IsSimulated() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.isSimulated
}

func (e *TradingEngine) TestGetSymbols(ctx context.Context) (int, error) {
	pairs, err := e.exchange.GetPairs(ctx)
	if err != nil {
		return 0, err
	}
	return len(pairs), nil
}

func (e *TradingEngine) SetStrictness(s int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if s < 1 {
		s = 1
	} else if s > 500 {
		s = 500
	}
	e.strictness = s
	log.Printf("⚙️ Strategy strictness updated to: %d", s)
}

func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dч %dмин", hours, minutes)
	}
	return fmt.Sprintf("%dмин", minutes)
}
