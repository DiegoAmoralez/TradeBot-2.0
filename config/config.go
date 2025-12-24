package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type TradingMode string

const (
	ModeFutures TradingMode = "FUTURES"
	ModeSpot    TradingMode = "SPOT"
)

type Config struct {
	TelegramToken      string
	BinanceAPIKey      string
	BinanceSecretKey   string
	AuthorizedUserID   int64
	MistralAPIKey      string
	Port               string
	TradingMode        TradingMode
	SpotMinQuoteVolume float64
	SpotMaxSpread      float64
	MaxPositionSize    float64 // Configurable max size per position
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	userID, err := strconv.ParseInt(os.Getenv("AUTHORIZED_USER_ID"), 10, 64)
	if err != nil {
		log.Fatal("Invalid AUTHORIZED_USER_ID")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	modeStr := os.Getenv("TRADING_MODE")
	mode := ModeFutures // Default to Futures
	if modeStr == "SPOT" {
		mode = ModeSpot
	}

	// Default Spot settings (can be overridden by UI later, but good to have defaults)
	minVol := 5000000.0 // 5M USDT
	if v := os.Getenv("SPOT_MIN_VOLUME"); v != "" {
		if val, err := strconv.ParseFloat(v, 64); err == nil {
			minVol = val
		}
	}

	maxSpread := 0.1 // 0.1%
	if s := os.Getenv("SPOT_MAX_SPREAD"); s != "" {
		if val, err := strconv.ParseFloat(s, 64); err == nil {
			maxSpread = val
		}
	}

	maxPosSize := 500.0 // Default 500 USDT
	if m := os.Getenv("MAX_POSITION_SIZE"); m != "" {
		if val, err := strconv.ParseFloat(m, 64); err == nil {
			maxPosSize = val
		}
	}

	return &Config{
		TelegramToken:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		BinanceAPIKey:      os.Getenv("BINANCE_API_KEY"),
		BinanceSecretKey:   os.Getenv("BINANCE_SECRET_KEY"),
		AuthorizedUserID:   userID,
		MistralAPIKey:      os.Getenv("MISTRAL_API_KEY"),
		Port:               port,
		TradingMode:        mode,
		SpotMinQuoteVolume: minVol,
		SpotMaxSpread:      maxSpread,
		MaxPositionSize:    maxPosSize,
	}
}
