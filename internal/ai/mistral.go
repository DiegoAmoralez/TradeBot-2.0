package ai

import (
	"bot_trading/internal/exchange"
	"bot_trading/internal/models"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type MistralClient struct {
	apiKey string
	client *http.Client
}

func NewMistralClient(apiKey string) *MistralClient {
	return &MistralClient{
		apiKey: apiKey,
		client: &http.Client{},
	}
}

type mistralRequest struct {
	Model    string           `json:"model"`
	Messages []mistralMessage `json:"messages"`
}

type mistralMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type mistralResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (m *MistralClient) AnalyzeSignal(symbol string, klines []exchange.Kline, indicators map[string]float64, technicalSide string, trend1h string, trend4h string, trendDaily string) (*models.AISignal, error) {
	prompt := m.buildPrompt(symbol, klines, indicators, technicalSide, trend1h, trend4h, trendDaily)

	reqBody := mistralRequest{
		Model: "mistral-small-latest",
		Messages: []mistralMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://api.mistral.ai/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("mistral API error: %s", string(body))
	}

	var mistralResp mistralResponse
	if err := json.Unmarshal(body, &mistralResp); err != nil {
		return nil, err
	}

	if len(mistralResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from Mistral")
	}

	return m.parseResponse(symbol, mistralResp.Choices[0].Message.Content, klines)
}

// AnalyzeSpotCandidate performs AI analysis for Spot trading with specific prompt types
func (m *MistralClient) AnalyzeSpotCandidate(symbol string, klines []exchange.Kline, indicators map[string]float64, promptType string, trendInfo map[string]string) (*models.AISignal, error) {
	prompt := m.buildSpotPrompt(symbol, klines, indicators, promptType, trendInfo)

	reqBody := mistralRequest{
		Model: "mistral-small-latest",
		Messages: []mistralMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://api.mistral.ai/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("mistral API error: %s", string(body))
	}

	var mistralResp mistralResponse
	if err := json.Unmarshal(body, &mistralResp); err != nil {
		return nil, err
	}

	if len(mistralResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from Mistral")
	}

	// Parse Spot specific JSON response which might differ slightly (percentages vs prices)
	return m.parseSpotResponse(symbol, mistralResp.Choices[0].Message.Content, klines[len(klines)-1].Close)
}

func (m *MistralClient) buildPrompt(symbol string, klines []exchange.Kline, indicators map[string]float64, technicalSide string, trend1h string, trend4h string, trendDaily string) string {
	lastPrice := klines[len(klines)-1].Close

	return fmt.Sprintf(`You are an expert crypto momentum scalper and market structure analyst. 
A technical system has flagged a potential **%s** entry for %s.
Your job is to qualify this trade using Multi-Timeframe (MTF) analysis.

**BIG PICTURE CONTEXT:**
- Daily Trend (14d): %s  <-- PRIMARY FILTER
- 4h Trend (Weekly): %s
- 1h Trend (Daily): %s

**CURRENT MARKET STATE (5m):**
- Price: %.4f USDT
- Target Side: %s
- RSI: %.2f
- MACD: %.4f (Signal: %.4f)
- EMA 20: %.4f | EMA 50: %.4f
- RVol (Rel. Volume): %.2fx
- 1h Price Change: %.2f%%

**RECENT ACTION (Last 20 Candles):**
%s

**YOUR INSTRUCTIONS:**
1. **Trend Alignment**: A trade is HIGH PROBABILITY only if it aligns with the **Daily Trend**. 
2. If Technical is LONG but Daily Trend is BEARISH, you SHOULD reject unless there is an extreme structure break.
3. Confirm with "BUY" if the technical signal aligns with momentum and structure (confidence > 50%%).
4. Volume surge (> 1.5x) is required for strong conviction.

Provide your analysis in JSON ONLY:
{
  "action": "BUY" (confirm) or "WAIT" (reject),
  "side": "%s",
  "confidence": 0-100,
  "take_profit": price_level,
  "stop_loss": price_level,
  "reasoning": "Reason MUST detail why it aligns (or not) with the HTF Trend and Volume (max 2 sentences)"
}`,
		technicalSide,
		symbol,
		trendDaily,
		trend4h,
		trend1h,
		lastPrice,
		technicalSide,
		indicators["rsi"],
		indicators["macd"],
		indicators["signal"],
		indicators["ema20"],
		indicators["ema50"],
		indicators["rvol"],
		indicators["price_change_1h"],
		m.formatKlines(klines[len(klines)-20:]),
		technicalSide,
	)
}

func (m *MistralClient) formatKlines(klines []exchange.Kline) string {
	var sb strings.Builder
	for i, k := range klines {
		sb.WriteString(fmt.Sprintf("%d. O:%.4f H:%.4f L:%.4f C:%.4f V:%.0f\n",
			i+1, k.Open, k.High, k.Low, k.Close, k.Volume))
	}
	return sb.String()
}

func (m *MistralClient) buildSpotPrompt(symbol string, klines []exchange.Kline, indicators map[string]float64, promptType string, trendInfo map[string]string) string {
	lastPrice := klines[len(klines)-1].Close

	basePrompt := fmt.Sprintf(`You are a expert crypto spot trader. Return ONLY valid JSON.
Symbol: %s
Current Price: %.4f
Market Data:
- RSI (D1): %.2f
- RSI (H1): %.2f
- MACD (H1): %.4f
- Trend D1: %s
- Trend H4: %s
- Trend H1: %s
- Volume 24h: %.0f
- Spread: %.2f%%

Recent Candles (M5):
%s
`,
		symbol, lastPrice,
		indicators["rsi_d1"], indicators["rsi_h1"], indicators["macd"],
		trendInfo["d1"], trendInfo["h4"], trendInfo["h1"],
		indicators["vol_24h"], indicators["spread"],
		m.formatKlines(klines[len(klines)-10:]))

	if promptType == "TRIAGE" {
		return basePrompt + `
Task: Decide if opening a LONG on this symbol is reasonable NOW.
We only buy to sell in profit. Avoid falling knives.
INPUT:
See above.

REQUIREMENTS:
- If price is BELOW EMA200 on D1 (check trend D1), allow entry ONLY if reversal conditions are strong.
- Provide confidence 0-100.
- Provide TP_percent and SL_percent that are realistic.
- If uncertain, say NO.

OUTPUT JSON schema:
{
  "decision": "BUY" or "NO",
  "confidence": number,
  "tp_percent": number,
  "sl_percent": number,
  "reason": "short",
  "risk_flags": ["..."]
}`
	} else if promptType == "DEEP" {
		return basePrompt + `
Task: Deep Validation. Maximize probability of profitable spot LONG trades.
Analyze 30-90 day context (implied by D1/H4 trends) and intraday setup.

Provide:
- decision BUY/NO
- confidence
- recommended entry type: "market" or "limit_pullback"
- tp_plan: 2 targets (tp1,tp2) in %
- sl_percent
- invalidation: what condition cancels the trade

OUTPUT JSON:
{
  "decision": "BUY"|"NO",
  "confidence": 0-100,
  "entry_type": "market"|"limit_pullback",
  "tp1_percent": number,
  "tp2_percent": number,
  "sl_percent": number,
  "trailing_activate_at_profit_percent": number,
  "trailing_distance_percent": number,
  "invalidation": "text",
  "reason": "text",
  "risk_flags": ["..."]
}`
	}

	// Default to simple
	return basePrompt + "Recommend BUY or NO with confidence and reasoning in JSON."
}

func (m *MistralClient) parseSpotResponse(symbol, content string, currentPrice float64) (*models.AISignal, error) {
	// Try to extract JSON from the response
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("no JSON found in response")
	}

	jsonStr := content[start : end+1]

	// Generic struct to capture common fields from different prompt types
	var result struct {
		Decision   string  `json:"decision"`
		Action     string  `json:"action"` // Fallback if Decision used action
		Confidence float64 `json:"confidence"`
		TpPercent  float64 `json:"tp_percent"`
		Tp1Percent float64 `json:"tp1_percent"`
		SlPercent  float64 `json:"sl_percent"`
		Reason     string  `json:"reason"`
		Reasoning  string  `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %v", err)
	}

	// Normalize fields
	action := result.Decision
	if action == "" {
		action = result.Action
	}
	if action == "NO" {
		action = "WAIT"
	}

	reason := result.Reason
	if reason == "" {
		reason = result.Reasoning
	}

	tpPct := result.TpPercent
	if tpPct == 0 {
		tpPct = result.Tp1Percent
	}

	tpPrice := currentPrice * (1 + tpPct/100.0)
	slPrice := currentPrice * (1 - result.SlPercent/100.0)

	return &models.AISignal{
		Symbol:     symbol,
		Action:     action,
		Side:       "LONG", // Spot is always Long for now
		Confidence: result.Confidence,
		TakeProfit: tpPrice,
		StopLoss:   slPrice,
		Reasoning:  reason,
	}, nil
}

func (m *MistralClient) parseResponse(symbol, content string, klines []exchange.Kline) (*models.AISignal, error) {
	// Try to extract JSON from the response
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("no JSON found in response")
	}

	jsonStr := content[start : end+1]

	var result struct {
		Action     string  `json:"action"`
		Side       string  `json:"side"`
		Confidence float64 `json:"confidence"`
		TakeProfit float64 `json:"take_profit"`
		StopLoss   float64 `json:"stop_loss"`
		Reasoning  string  `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %v", err)
	}

	return &models.AISignal{
		Symbol:     symbol,
		Action:     result.Action,
		Side:       result.Side,
		Confidence: result.Confidence,
		TakeProfit: result.TakeProfit,
		StopLoss:   result.StopLoss,
		Reasoning:  result.Reasoning,
	}, nil
}
