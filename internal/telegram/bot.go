package telegram

import (
	"bot_trading/internal/engine"
	"bot_trading/internal/models"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"
)

type Bot struct {
	bot          *tele.Bot
	engine       *engine.TradingEngine
	authorizedID int64
	startTime    time.Time
}

func NewBot(token string, authorizedID int64, engine *engine.TradingEngine) (*Bot, error) {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		return nil, err
	}

	bot := &Bot{
		bot:          b,
		engine:       engine,
		authorizedID: authorizedID,
		startTime:    time.Now(),
	}

	bot.setupHandlers()
	return bot, nil
}

func (b *Bot) Start() {
	log.Println("📱 Telegram bot started")
	b.bot.Start()
}

func (b *Bot) setupHandlers() {
	// Middleware for authorization
	b.bot.Use(func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			if c.Sender().ID != b.authorizedID {
				return c.Send("⛔ Unauthorized")
			}
			return next(c)
		}
	})

	// Commands
	b.bot.Handle("/start", b.handleStart)
	b.bot.Handle("/stats", b.handleStats)
	b.bot.Handle("/positions", b.handlePositions)
	b.bot.Handle("/settings", b.handleSettings)

	// Buttons
	b.bot.Handle(&btnStartTrading, b.handleStartTrading)
	b.bot.Handle(&btnStopTrading, b.handleStopTrading)
	b.bot.Handle(&btnStats, b.handleStats)
	b.bot.Handle(&btnPositions, b.handlePositions)
	b.bot.Handle(&btnSettings, b.handleSettings)
	b.bot.Handle(&btnRefresh, b.handleStats)
	b.bot.Handle(&btnCloseAll, b.handleCloseAll)
	b.bot.Handle(&btnBack, b.handleStart)
}

var (
	btnStartTrading = tele.Btn{Text: "▶️ Старт торговли", Unique: "start_trading"}
	btnStopTrading  = tele.Btn{Text: "⏸️ Остановить", Unique: "stop_trading"}
	btnStats        = tele.Btn{Text: "📊 Статистика", Unique: "stats"}
	btnPositions    = tele.Btn{Text: "📋 Позиции", Unique: "positions"}
	btnSettings     = tele.Btn{Text: "⚙️ Настройки", Unique: "settings"}
	btnRefresh      = tele.Btn{Text: "🔄 Обновить", Unique: "refresh"}
	btnCloseAll     = tele.Btn{Text: "❌ Закрыть все", Unique: "close_all"}
	btnBack         = tele.Btn{Text: "🔙 Назад", Unique: "back"}
)

func (b *Bot) handleStart(c tele.Context) error {
	menu := &tele.ReplyMarkup{}

	var startBtn tele.Btn
	if b.engine.IsRunning() {
		startBtn = btnStopTrading
	} else {
		startBtn = btnStartTrading
	}

	menu.Inline(
		menu.Row(startBtn),
		menu.Row(btnStats, btnPositions),
		menu.Row(btnSettings),
	)

	status := "⏸️ Остановлен"
	if b.engine.IsRunning() {
		status = "▶️ Активен"
	}

	msg := fmt.Sprintf(`🤖 *Торговый бот Binance Futures*

🔄 Статус: %s

Выберите действие:`, status)

	return c.Send(msg, menu, tele.ModeMarkdown)
}

func (b *Bot) handleStartTrading(c tele.Context) error {
	b.engine.Start()
	return b.handleStart(c)
}

func (b *Bot) handleStopTrading(c tele.Context) error {
	b.engine.Stop()
	return b.handleStart(c)
}

func (b *Bot) handleStats(c tele.Context) error {
	ctx := context.Background()
	stats := b.engine.GetStats(ctx)

	balance, _ := b.engine.GetBalance(ctx)
	positions := b.engine.GetPositions()

	inPositions := 0.0
	for _, p := range positions {
		inPositions += p.PositionSize
	}

	status := "⏸️ Остановлен"
	if b.engine.IsRunning() {
		status = "▶️ Активен"
	}

	plEmoji := "🟢"
	if stats.TotalPL < 0 {
		plEmoji = "🔴"
	} else if stats.TotalPL == 0 {
		plEmoji = "🟡"
	}

	uptime := time.Since(b.startTime)

	msg := fmt.Sprintf(`📊 *Торговая статистика*

🔄 Статус: %s
🎯 Режим: 📊 Эмуляция
💰 Баланс: %.2f USDT
📈 В сделках: %.2f USDT
📋 Открытых позиций: %d
💎 Нереализованный P&L: %+.2f USDT
📅 Сделок всего: %d
🏆 Прибыльных: %d
📉 Убыточных: %d
📊 Винрейт: %.1f%%
💰 Общий P&L: %s %+.2f USDT

🕐 Время работы: %s
🕐 Обновлено: %s`,
		status,
		balance,
		inPositions,
		len(positions),
		stats.UnrealizedPL,
		stats.TotalTrades,
		stats.ProfitableTrades,
		stats.LosingTrades,
		stats.WinRate,
		plEmoji,
		stats.TotalPL,
		formatUptime(uptime),
		time.Now().Format("15:04:05"),
	)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(btnRefresh, btnPositions),
		menu.Row(btnSettings, btnBack),
	)

	return c.Send(msg, menu, tele.ModeMarkdown)
}

func (b *Bot) handlePositions(c tele.Context) error {
	positions := b.engine.GetPositions()

	if len(positions) == 0 {
		menu := &tele.ReplyMarkup{}
		menu.Inline(menu.Row(btnBack))
		return c.Send("📋 Нет открытых позиций", menu)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 *Открытые позиции (%d)*\n\n", len(positions)))

	totalPL := 0.0
	for _, p := range positions {
		emoji := "🟢"
		if p.Side == "SHORT" {
			emoji = "🔴"
		}
		if p.PLPercent < 0 {
			emoji = "🟡"
		}

		sb.WriteString(fmt.Sprintf(`%s *%s %s* | %.2f USDT
   📊 %.4f → %.4f (%+.2f%%)
   💰 P&L: %+.2f USDT | TP: %.4f | SL: %.4f

`, emoji, p.Side, p.Symbol, p.PositionSize, p.EntryPrice, p.CurrentPrice, p.PLPercent, p.UnrealizedPL, p.TakeProfit, p.StopLoss))

		totalPL += p.UnrealizedPL
	}

	sb.WriteString(fmt.Sprintf("💎 Общий нереализованный P&L: %+.2f USDT", totalPL))

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(btnRefresh, btnCloseAll),
		menu.Row(btnBack),
	)

	return c.Send(sb.String(), menu, tele.ModeMarkdown)
}

func (b *Bot) handleSettings(c tele.Context) error {
	ctx := context.Background()
	balance, _ := b.engine.GetBalance(ctx)

	status := "⏸️ Остановлен"
	if b.engine.IsRunning() {
		status = "▶️ Активен"
	}

	msg := fmt.Sprintf(`⚙️ *Настройки системы*

Режим: 🟡 Режим эмуляции
Тип торговли: Фьючерсы
Максимум пар: 5
Баланс эмуляции: %.2f USDT

Статус: %s

💡 Система %s`,
		balance,
		status,
		func() string {
			if b.engine.IsRunning() {
				return "активна - анализирует рынок и открывает сделки"
			}
			return "остановлена"
		}(),
	)

	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(btnBack))

	return c.Send(msg, menu, tele.ModeMarkdown)
}

func (b *Bot) handleCloseAll(c tele.Context) error {
	ctx := context.Background()
	b.engine.CloseAllPositions(ctx)
	return c.Send("✅ Все позиции закрыты")
}

func (b *Bot) SendTradeOpen(position *models.Position) {
	msg := fmt.Sprintf(`✅ *ПОЗИЦИЯ ОТКРЫТА*

%s *%s %s*
💰 Размер: %.2f USDT
📊 Вход: %.4f
🎯 Take Profit: %.4f
🛡️ Stop Loss: %.4f

⏰ %s`,
		func() string {
			if position.Side == "LONG" {
				return "📈"
			}
			return "📉"
		}(),
		position.Side,
		position.Symbol,
		position.PositionSize,
		position.EntryPrice,
		position.TakeProfit,
		position.StopLoss,
		time.Now().Format("15:04:05"),
	)

	b.bot.Send(&tele.User{ID: b.authorizedID}, msg, tele.ModeMarkdown)
}

func (b *Bot) SendTradeClose(trade *models.Trade) {
	emoji := "✅"
	plEmoji := "💚"
	if trade.RealizedPL < 0 {
		emoji = "⚠️"
		plEmoji = "❤️"
	}

	msg := fmt.Sprintf(`%s *ПОЗИЦИЯ ЗАКРЫТА*

%s *%s %s* закрыт (%s)
%s P&L: %+.2f USDT (%+.2f%%)
⏱️ Длительность: %s
📊 %.4f → %.4f
💼 Новый баланс: %.2f USDT

⏰ %s`,
		emoji,
		func() string {
			if trade.Side == "LONG" {
				return "📈"
			}
			return "📉"
		}(),
		trade.Side,
		trade.Symbol,
		trade.CloseReason,
		plEmoji,
		trade.RealizedPL,
		trade.PLPercent,
		formatDuration(trade.Duration),
		trade.EntryPrice,
		trade.ExitPrice,
		0.0, // Will be updated
		time.Now().Format("15:04:05"),
	)

	b.bot.Send(&tele.User{ID: b.authorizedID}, msg, tele.ModeMarkdown)
}

func (b *Bot) SendAnalysisUpdate(message string) {
	b.bot.Send(&tele.User{ID: b.authorizedID}, "🔍 "+message)
}

func (b *Bot) GetBalance(ctx context.Context) (float64, error) {
	return b.engine.GetBalance(ctx)
}

func formatUptime(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dч %dмин", hours, minutes)
	}
	return fmt.Sprintf("%dмин", minutes)
}

func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dч %dмин", hours, minutes)
	}
	return fmt.Sprintf("%dмин", minutes)
}
