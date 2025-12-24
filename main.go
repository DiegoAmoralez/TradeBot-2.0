package main

import (
	"bot_trading/config"
	"bot_trading/internal/ai"
	"bot_trading/internal/engine"
	"bot_trading/internal/exchange"
	"bot_trading/internal/telegram"
	"bot_trading/internal/web"
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("🚀 Starting Trading Bot...")

	// Load configuration
	cfg := config.Load()

	// Initialize exchange client based on mode
	var realExchange exchange.ExchangeClient
	if cfg.TradingMode == config.ModeSpot {
		realExchange = exchange.NewSpotClient(cfg.BinanceAPIKey, cfg.BinanceSecretKey, false) // Spot doesn't fully support testnet same way, or assuming false for now. Todo: config testnet
		// If we want testnet for spot, we need to handle it. For now let's assume real key for spot or handle gracefully.
		// Actually go-binance supports spot testnet.
	} else {
		// Default to Futures
		realExchange = exchange.NewFuturesClient(cfg.BinanceAPIKey, cfg.BinanceSecretKey, true) // Default strictness/testnet? previous code hardcoded testnet=true in NewEmulatorClient context?
		// Wait, NewEmulatorClient previously created a REAL client with testnet=true properly inside.
		// Now we inject the client.
	}

	// Create Emulator using the chosen exchange for data
	// Note: We might want real trading later, but for now start with emulator as per logical flow or existing code.
	// Existing code: exchangeClient = emulator
	emulator := exchange.NewEmulatorClient(5000.0, realExchange)

	// Switch between Real and Emulator based on some flag?
	// The user prompt says "Mode: Emu/Real".
	// For now let's keep Emulator active as default but initialized with the correct base.
	var exchangeClient exchange.ExchangeClient
	exchangeClient = emulator

	// Initialize AI client
	aiClient := ai.NewMistralClient(cfg.MistralAPIKey)

	// Initialize trading engine
	tradingEngine := engine.NewTradingEngine(
		exchangeClient,
		aiClient,
		cfg,
	)

	// Pass Config settings to Engine if needed (e.g. strictness default)
	// engine.NewTradingEngine signature is simple for now.

	// Initialize Telegram bot
	bot, err := telegram.NewBot(cfg.TelegramToken, cfg.AuthorizedUserID, tradingEngine)
	if err != nil {
		log.Fatalf("Failed to create Telegram bot: %v", err)
	}

	// Set up callbacks
	tradingEngine.SetCallbacks(
		bot.SendTradeOpen,
		bot.SendTradeClose,
		bot.SendAnalysisUpdate,
	)

	// Initialize web server
	webServer := web.NewServer(tradingEngine, cfg.Port)
	webServer.Start()

	// Start Telegram bot
	go bot.Start()

	log.Println("✅ All systems initialized")
	log.Println("📱 Telegram bot is ready")
	log.Printf("🌐 Web dashboard: http://localhost:%s\n", cfg.Port)
	log.Println("⏸️ Trading engine is stopped. Use /start in Telegram to begin trading.")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("\n🛑 Shutting down...")
	tradingEngine.Stop()

	// Close all positions before exit
	ctx := context.Background()
	tradingEngine.CloseAllPositions(ctx)

	log.Println("👋 Goodbye!")
}
