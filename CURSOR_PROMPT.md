# Cursor/Composer Prompt: Go Binance Trading Bot

You are an expert Go developer and quantitative trading engineer.
Your task is to build a complete Trading Bot for Binance Futures.

## Context & Environment
- **Language**: Go (Golang)
- **Exchange**: Binance Futures (USDT-Margined)
- **AI Integration**: Mistral API
- **User Interface**: Telegram Bot (Telebot) + Web Dashboard (HTML/JS)
- **Environment**: Keys are provided in `.env` (TELEGRAM_BOT_TOKEN, BINANCE_API_KEY, BINANCE_SECRET_KEY, AUTHORIZED_USER_ID, MISTRAL_API_KEY).

## Core Requirements

### 1. System Architecture
The system must run a continuous loop with the following components:
- **Telegram Listener**: Handles user commands command updates.
- **Trading Engine**: The core logic loop.
- **Web Server**: Serves the dashboard on port 8080.

### 2. Trading Workflow
**Start Condition**: System waits for "Start" command from Telegram.

**Analysis Cycle (Every 5 minutes, if slots available):**
1.  **Phase 1 (Scan)**: Fetch all USDT Futures pairs. Filter for basic liquidity/volatility suitable for trading.
2.  **Phase 2 (Strategy)**: Apply technical analysis (RSI, MACD, Trend) to select best candidates for Long/Short.
    - *Goal*: Select top candidates.
3.  **Phase 3 (AI)**: Send market data of candidates to Mistral API.
    - *Input*: Price history, indicators.
    - *Output*: Decision (BUY/WAIT), Confidence Score, Suggested TP/SL.
    - *Rule*: Only execute if Confidence > 55.

**Position Management (Every 30 seconds):**
- Monitor all open positions (Real or Emulated).
- Update Trailing Stop (1% of TP).
- Dynamic TP Adjustment: If price moves favorably, adjust TP to maximize profit.
- Close position if SL hit, TP hit, or AI reversal signal.

### 3. Emulation vs Real Trading
- Implement an interface `ExchangeClient` with two implementations: `BinanceClient` (Real) and `PaperTradingClient` (Mock).
- **Emulation**:
    - Start Balance: 5000 USDT.
    - Simulate fills using real-time market price.
    - Track P&L realistically (including fee simulation).
- Switch modes via Telegram Settings.

### 4. Interfaces

#### Telegram Bot
- **Auth**: Only respond to `AUTHORIZED_USER_ID`.
- **Menu (Buttons)**:
    - `[ ▶️ Start ]` / `[ ⏸️ Stop ]`
    - `[ 📊 Stats ]`
    - `[ ⚙️ Settings ]`
- **Notifications**:
    - Buy/Sell events with formatted messages (Entry, TP, SL, Volume).
    - P&L Realized updates.
- **Stats Message**:
    - formatted markdown with Balance, Open/Closed P&L, Winrate, Active Pairs.

#### Web Dashboard
- **Left Panel**: Global Stats (Balance, P&L, Active Slots).
- **Right Panel**: Cards for each active pair showing:
    - Pair Name, Side (Long/Short).
    - Entry Price, Current Price, P&L %.
    - Mini-chart (optional, or just price trend stats).
- **Style**: Dark mode, premium aesthetics (CSS Variables, clean UI).

## Implementation Steps
1.  **Setup**: Initialize `go.mod`, setup project structure (`cmd`, `internal`, `web`).
2.  **Infrastructure**: Implement `.env` loader, Logging.
3.  **Exchange Layer**: Build the `ExchangeClient` interface.
4.  **Bot Logic**: Implement the 3-phase analysis engine.
5.  **Telegram**: Build the bot handler and UI.
6.  **Web**: Build the status JSON API and the frontend.

## Important
- **Error Handling**: Log errors but do not crash the main loop.
- **Concurrency**: Use Goroutines for the Web server and Bot listener. Use Channels or Mutexes for shared state (Account Balance, Active Positions).
