# Binance Trading Bot (Go)

A high-performance trading bot for Binance Futures written in Go.
Features include Real/Emulation modes, AI-driven analysis (Mistral), Telegram control, and a Web Dashboard.

## Features
- **Dual Mode**: Switch between Real Trading and Emulation interactively.
- **Three-Stage Analysis**:
    1.  **Market Scan**: Broad scan of all USDT Futures pairs.
    2.  **Strategy Filter**: Deep dive into candidates using technical indicators.
    3.  **AI Confirmation**: Mistral AI analysis of the final candidates.
- **Dynamic Risk Management**:
    - Auto TP/SL calculation.
    - Trailing Stop optimization.
    - P&L monitoring every 30 seconds.
- **Telegram Integration**:
    - Control bot via Buttons (Start, Stats, Settings).
    - Real-time notifications of trades and P&L.
- **Web Dashboard**:
    - Visual overview of active pairs and system stats.

## Configuration
Credentials are stored in `.env`.

## Usage
1.  Run the bot: `go run main.go`
2.  Open Telegram and interact with the bot.
3.  Open `http://localhost:8080` for the dashboard.
