package telegram

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/gomodule/redigo/redis"

	"github.com/Ptt-Alertor/ptt-alertor/command"
	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/models/account"
	"github.com/Ptt-Alertor/ptt-alertor/models/binding"
	"github.com/Ptt-Alertor/ptt-alertor/myutil"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/julienschmidt/httprouter"
)

var (
	bot   *tgbotapi.BotAPI
	err   error
	token = os.Getenv("TELEGRAM_TOKEN")
	host  = os.Getenv("APP_HOST")
)

func init() {
	bot, err = tgbotapi.NewBotAPI(token)
	if err != nil {
		log.WithError(err).Fatal("Telegram Bot Initialize Failed")
	}
	// bot.Debug = true
	log.Info("Telegram Authorized on " + bot.Self.UserName)

	webhookConfig, err := tgbotapi.NewWebhook(host + "/telegram/" + token)
	if err != nil {
		log.WithError(err).Error("Telegram Bot Create Webhook Failed")
		return
	}
	webhookConfig.MaxConnections = 100
	_, err = bot.Request(webhookConfig)
	if err != nil {
		log.WithError(err).Error("Telegram Bot Set Webhook Failed - will retry on next restart")
		return
	}
	log.Info("Telegram Bot Sets Webhook Success")
}

// HandleRequest handles request from webhook
func HandleRequest(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.WithError(err).Error("Telegram Read Request Body Failed")
	}

	var update tgbotapi.Update
	json.Unmarshal(bytes, &update)

	if update.CallbackQuery != nil {
		handleCallbackQuery(update)
		return
	}

	if update.Message != nil {
		if update.Message.IsCommand() {
			handleCommand(update)
			return
		}
		if update.Message.Text != "" {
			handleText(update)
			return
		}
	}
}

func handleCallbackQuery(update tgbotapi.Update) {
	var responseText string
	userID := strconv.FormatInt(update.CallbackQuery.From.ID, 10)
	switch update.CallbackQuery.Data {
	case "CANCEL":
		responseText = "取消"
	default:
		responseText = command.HandleCommand(update.CallbackQuery.Data, userID, true)
	}
	SendTextMessage(update.CallbackQuery.Message.Chat.ID, responseText)
}

// help - 所有指令清單
// list - 設定清單
// ranking - 熱門關鍵字、作者、推文數
// add - 新增看板關鍵字、作者、推文數
// del - 刪除看板關鍵字、作者、推文數
// bind - 綁定網頁帳號
// showkeyboard - 顯示快捷小鍵盤
// hidekeyboard - 隱藏快捷小鍵盤
func handleCommand(update tgbotapi.Update) {
	var responseText string
	userID := strconv.FormatInt(update.Message.From.ID, 10)
	chatID := update.Message.Chat.ID

	switch update.Message.Command() {
	case "add", "del":
		text := update.Message.Command() + " " + update.Message.CommandArguments()
		responseText = command.HandleCommand(text, userID, true)
	case "start":
		args := update.Message.CommandArguments()
		// Check if this is a bind request via deep link
		if strings.HasPrefix(args, "BIND_") {
			code := strings.TrimPrefix(args, "BIND_")
			responseText = handleBindCode(code, chatID)
		} else {
			command.HandleTelegramFollow(userID, chatID)
			responseText = "歡迎使用 Ptt Alertor\n輸入「指令」查看相關功能。\n\n觀看Demo:\nhttps://media.giphy.com/media/3ohzdF6vidM6I49lQs/giphy.gif"
		}
	case "help":
		responseText = command.HandleCommand("help", userID, true)
	case "list":
		responseText = command.HandleCommand("list", userID, true)
	case "ranking":
		responseText = command.HandleCommand("ranking", userID, true)
	case "bind":
		args := update.Message.CommandArguments()
		if args == "" {
			// No arguments - send Web App button for binding
			sendWebAppBindButton(chatID)
			return
		}
		// Has arguments - legacy bind code flow from Dashboard
		responseText = handleBindCode(args, chatID)
	case "showkeyboard":
		showReplyKeyboard(chatID)
		return
	case "hidekeyboard":
		hideReplyKeyboard(chatID)
		return
	default:
		responseText = "I don't know the command"
	}
	SendTextMessage(chatID, responseText)
}

var bindingRepo = &binding.Postgres{}

const webAppBindTokenPrefix = "webapp:bind:"
const webAppBindTokenExpiry = 600 // 10 minutes

// sendWebAppBindButton sends a Web App button for account binding
func sendWebAppBindButton(chatID int64) {
	// Generate token and store in Redis with chat_id
	token, err := binding.GenerateBindCode()
	if err != nil {
		log.WithError(err).Error("Failed to generate webapp bind token")
		SendTextMessage(chatID, "產生綁定連結失敗，請稍後再試")
		return
	}

	// Store token -> chat_id in Redis
	conn := connections.Redis()
	defer conn.Close()

	key := webAppBindTokenPrefix + token
	_, err = conn.Do("SETEX", key, webAppBindTokenExpiry, chatID)
	if err != nil {
		log.WithError(err).Error("Failed to store webapp bind token")
		SendTextMessage(chatID, "產生綁定連結失敗，請稍後再試")
		return
	}

	// Build Web App URL
	dashboardURL := os.Getenv("DASHBOARD_URL")
	if dashboardURL == "" {
		dashboardURL = "http://localhost:3000"
	}
	webAppURL := dashboardURL + "/telegram/bind?token=" + token

	// Send message with URL button (opens in browser)
	msg := tgbotapi.NewMessage(chatID, "點擊下方按鈕登入並綁定帳號")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("🔗 登入綁定", webAppURL),
		),
	)

	_, err = bot.Send(msg)
	if err != nil {
		log.WithError(err).Error("Failed to send Web App button")
	}
}

// GetWebAppBindChatID retrieves chat_id from Redis by token
func GetWebAppBindChatID(token string) (int64, error) {
	conn := connections.Redis()
	defer conn.Close()

	key := webAppBindTokenPrefix + token
	chatID, err := redis.Int64(conn.Do("GET", key))
	if err != nil {
		return 0, err
	}

	// Delete token after use
	conn.Do("DEL", key)

	return chatID, nil
}

// handleBindCode handles the /bind <code> command (legacy flow from Dashboard)
func handleBindCode(args string, chatID int64) string {
	code := strings.TrimSpace(args)
	if code == "" {
		return "請輸入綁定碼\n格式: /bind <綁定碼>"
	}

	// Find binding by bind code
	b, err := bindingRepo.FindByBindCode(binding.ServiceTelegram, code)
	if err != nil {
		if err == binding.ErrBindingNotFound {
			return "綁定碼無效或已過期，請重新產生"
		}
		log.WithError(err).Error("Failed to find binding by bind code")
		return "綁定失敗，請稍後再試"
	}

	// Check if service ID is already set (already bound)
	if b.ServiceID != "" {
		return "此帳號已綁定 Telegram"
	}

	// Check if this chat ID is already bound to another account
	existingBinding, err := bindingRepo.FindByServiceID(binding.ServiceTelegram, strconv.FormatInt(chatID, 10))
	if err == nil && existingBinding != nil {
		return "此 Telegram 已綁定其他帳號，請先解除綁定"
	}

	// Confirm binding with chat ID
	if err := bindingRepo.ConfirmBinding(b.UserID, binding.ServiceTelegram, strconv.FormatInt(chatID, 10)); err != nil {
		log.WithError(err).Error("Failed to confirm binding")
		return "綁定失敗，請稍後再試"
	}

	// Sync existing subscriptions to Redis after binding
	go (&account.RedisSync{}).SyncAllSubscriptions(b.UserID)

	return "綁定成功！您現在可以在網頁上管理訂閱，通知將發送到此 Telegram。"
}

func handleText(update tgbotapi.Update) {
	var responseText string
	userID := strconv.FormatInt(update.Message.From.ID, 10)
	chatID := update.Message.Chat.ID
	text := update.Message.Text
	if match, _ := regexp.MatchString("^(刪除|刪除作者)+\\s.*\\*+", text); match {
		sendConfirmation(chatID, text)
		return
	}
	responseText = command.HandleCommand(text, userID, true)
	SendTextMessage(chatID, responseText)
}

func sendConfirmation(chatID int64, cmd string) {
	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("是", cmd),
			tgbotapi.NewInlineKeyboardButtonData("否", "CANCEL"),
		))
	msg := tgbotapi.NewMessage(chatID, "確定"+cmd+"？")
	msg.ReplyMarkup = markup
	_, err := bot.Send(msg)
	if err != nil {
		log.WithError(err).Error("Telegram Send Confirmation Failed")
	}
}

const maxCharacters = 4096

// SendTextMessage sends text message to chatID
func SendTextMessage(chatID int64, text string) {
	for _, msg := range myutil.SplitTextByLineBreak(text, maxCharacters) {
		sendTextMessage(chatID, msg)
	}
}

func sendTextMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = true
	_, err := bot.Send(msg)
	if err != nil {
		log.WithError(err).Error("Telegram Send Message Failed")
	}
}

func showReplyKeyboard(chatID int64) {
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("清單"),
			tgbotapi.NewKeyboardButton("推文清單"),
			tgbotapi.NewKeyboardButton("排行"),
			tgbotapi.NewKeyboardButton("指令"),
		))
	msg := tgbotapi.NewMessage(chatID, "顯示小鍵盤")
	msg.ReplyMarkup = keyboard
	_, err := bot.Send(msg)
	if err != nil {
		log.WithError(err).Error("Telegram Show Reply Keyboard Failed")
	}
}

func hideReplyKeyboard(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "隱藏小鍵盤")
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	_, err := bot.Send(msg)
	if err != nil {
		log.WithError(err).Error("Telegram Hide Reply Keyboard Failed")
	}
}
