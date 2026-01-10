package telegram

import (
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/gomodule/redigo/redis"
	"golang.org/x/crypto/bcrypt"

	"github.com/Ptt-Alertor/ptt-alertor/command"
	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/models/account"
	"github.com/Ptt-Alertor/ptt-alertor/models/binding"
	"github.com/Ptt-Alertor/ptt-alertor/myutil"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/mail"
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
	chatID := update.CallbackQuery.Message.Chat.ID
	data := update.CallbackQuery.Data

	// Answer callback query immediately to prevent Telegram from retrying
	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	bot.Request(callback)

	switch {
	case data == "CANCEL":
		responseText = "取消"
	case data == "m_x":
		responseText = "ℹ️ 已取消寄信"
	case strings.HasPrefix(data, "m_p:"):
		// Show mail preview with confirm/cancel buttons
		handleMailPreview(data, chatID)
		return
	case strings.HasPrefix(data, "m_c:"):
		// Send "processing" message first for mail (takes time)
		SendTextMessage(chatID, "⏳ 正在寄送信件...")
		responseText = handleMailConfirm(data, chatID)
	default:
		responseText = command.HandleCommand(data, userID, true)
	}
	SendTextMessage(chatID, responseText)
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
	case "start":
		args := update.Message.CommandArguments()
		// Check if this is a bind request via deep link
		if strings.HasPrefix(args, "BIND_") {
			code := strings.TrimPrefix(args, "BIND_")
			responseText = handleBindCode(code, chatID)
		} else {
			responseText = "歡迎使用 PTT Alertor！\n\n" +
				"📌 如何開始：\n" +
				"1. 前往網站註冊/登入\n" +
				"2. 使用 /bind 綁定帳號\n\n" +
				"🔗 網站：https://ptt.luan.com.tw\n\n" +
				"輸入 /help 查看更多指令"
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
			// Check if already bound
			handleBindCommand(chatID)
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
var accountRepo = &account.Postgres{}

const waitingEmailPrefix = "telegram:waiting_email:"
const waitingEmailExpiry = 300 // 5 minutes

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

// handleBindCommand handles /bind command - check if bound or ask for email
func handleBindCommand(chatID int64) {
	chatIDStr := strconv.FormatInt(chatID, 10)

	// Check if already bound
	existingBinding, err := bindingRepo.FindByServiceID(binding.ServiceTelegram, chatIDStr)
	if err == nil && existingBinding != nil {
		// Already bound - get account info
		acc, err := accountRepo.FindByID(existingBinding.UserID)
		if err == nil {
			SendTextMessage(chatID, "✅ 已綁定帳號："+acc.Email)
			return
		}
	}

	// Not bound - ask for email
	conn := connections.Redis()
	defer conn.Close()

	// Set waiting state in Redis
	key := waitingEmailPrefix + chatIDStr
	_, err = conn.Do("SETEX", key, waitingEmailExpiry, "1")
	if err != nil {
		log.WithError(err).Error("Failed to set waiting email state")
		SendTextMessage(chatID, "發生錯誤，請稍後再試")
		return
	}

	SendTextMessage(chatID, "請輸入您的 Email：")
}

// isWaitingForEmail checks if chatID is waiting for email input
func isWaitingForEmail(chatID int64) bool {
	conn := connections.Redis()
	defer conn.Close()

	key := waitingEmailPrefix + strconv.FormatInt(chatID, 10)
	exists, err := redis.Bool(conn.Do("EXISTS", key))
	if err != nil {
		return false
	}
	return exists
}

// clearWaitingEmail clears the waiting email state
func clearWaitingEmail(chatID int64) {
	conn := connections.Redis()
	defer conn.Close()

	key := waitingEmailPrefix + strconv.FormatInt(chatID, 10)
	conn.Do("DEL", key)
}

// handleEmailInput handles email input for registration and binding
func handleEmailInput(chatID int64, email string) {
	chatIDStr := strconv.FormatInt(chatID, 10)

	// Clear waiting state
	clearWaitingEmail(chatID)

	// Validate email format
	if !isValidEmail(email) {
		SendTextMessage(chatID, "❌ Email 格式不正確，請重新輸入 /bind")
		return
	}

	// Get webapp URL for messages
	webappURL := os.Getenv("WEBAPP_URL")
	if webappURL == "" {
		webappURL = "https://ptt.luan.com.tw"
	}

	// Check if email already registered
	existingAcc, err := accountRepo.FindByEmail(email)
	if err == nil && existingAcc != nil {
		// Email exists - check if this account is already bound to another Telegram
		existingBinding, _ := bindingRepo.FindByUserAndService(existingAcc.ID, binding.ServiceTelegram)
		if existingBinding != nil && existingBinding.ServiceID != "" {
			// Already bound to another Telegram
			SendTextMessage(chatID, "❌ 此帳號已綁定其他 Telegram")
			return
		}

		// Account exists but not bound - bind it
		SendTextMessage(chatID, "已有帳號，為您綁定中...")

		// Create or update binding
		if existingBinding != nil {
			// Binding record exists but no service ID - update it
			err = bindingRepo.ConfirmBinding(existingAcc.ID, binding.ServiceTelegram, chatIDStr)
		} else {
			// No binding record - create new
			_, err = bindingRepo.Create(existingAcc.ID, binding.ServiceTelegram, chatIDStr)
		}

		if err != nil {
			log.WithError(err).Error("Failed to bind existing account")
			SendTextMessage(chatID, "❌ 綁定失敗，請稍後再試")
			return
		}

		// Sync subscriptions to Redis
		go (&account.RedisSync{}).SyncAllSubscriptions(existingAcc.ID)

		SendTextMessage(chatID, "✅ 綁定成功！\n\n🔗 前往網站管理訂閱："+webappURL)
		return
	}

	// New user - create account
	password := generateRandomPassword()

	// Hash password
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.WithError(err).Error("Failed to hash password")
		SendTextMessage(chatID, "❌ 建立帳號失敗，請稍後再試")
		return
	}

	// Create account
	newAcc, err := accountRepo.Create(email, string(passwordHash), "user")
	if err != nil {
		log.WithError(err).Error("Failed to create account")
		SendTextMessage(chatID, "❌ 建立帳號失敗，請稍後再試")
		return
	}

	// Create binding
	_, err = bindingRepo.Create(newAcc.ID, binding.ServiceTelegram, chatIDStr)
	if err != nil {
		log.WithError(err).Error("Failed to create binding")
		SendTextMessage(chatID, "❌ 綁定失敗，請稍後再試")
		return
	}

	// Send success message
	successMsg := "✅ 帳號建立成功！\n\n" +
		"📧 Email: " + email + "\n" +
		"🔑 臨時密碼: " + password + "\n\n" +
		"⚠️ 請記得至網頁修改密碼\n" +
		"🔗 " + webappURL
	SendTextMessage(chatID, successMsg)
}

// isValidEmail validates email format
func isValidEmail(email string) bool {
	pattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	matched, _ := regexp.MatchString(pattern, email)
	return matched
}

// generateRandomPassword generates a random 6-letter password
func generateRandomPassword() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	result := make([]byte, 6)
	for i := range result {
		result[i] = letters[rand.Intn(len(letters))]
	}
	return string(result)
}

func handleText(update tgbotapi.Update) {
	var responseText string
	userID := strconv.FormatInt(update.Message.From.ID, 10)
	chatID := update.Message.Chat.ID
	text := update.Message.Text

	// Check if waiting for email input
	if isWaitingForEmail(chatID) {
		handleEmailInput(chatID, strings.TrimSpace(text))
		return
	}

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


// MailButtonData contains data for mail button callback
type MailButtonData struct {
	UserID         int    `json:"u"` // User ID in PostgreSQL
	SubscriptionID int    `json:"s"` // Subscription ID
	ArticleAuthor  string `json:"a"` // PTT article author
	ArticleIndex   int    `json:"i"` // 1-based index for display
}

// SendMessageWithMailButton sends message with mail buttons for multiple articles
func SendMessageWithMailButton(chatID int64, text string, mailDataList []*MailButtonData) {
	for _, msg := range myutil.SplitTextByLineBreak(text, maxCharacters) {
		if len(mailDataList) > 0 {
			sendTextMessageWithMailButton(chatID, msg, mailDataList)
		} else {
			sendTextMessage(chatID, msg)
		}
	}
}

func sendTextMessageWithMailButton(chatID int64, text string, mailDataList []*MailButtonData) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = true

	// Create buttons for each article (max 8 to avoid too many buttons)
	var buttons []tgbotapi.InlineKeyboardButton
	maxButtons := 8
	if len(mailDataList) < maxButtons {
		maxButtons = len(mailDataList)
	}

	for i := 0; i < maxButtons; i++ {
		mailData := mailDataList[i]
		// Create callback data: m_p:<userID>:<subID>:<author>
		callbackData := "m_p:" + strconv.Itoa(mailData.UserID) + ":" +
			strconv.Itoa(mailData.SubscriptionID) + ":" + mailData.ArticleAuthor

		// Check if callback data is within Telegram's limit (64 bytes)
		if len(callbackData) <= 64 {
			// Button text: multiple articles show "寄信給#N作者", single shows "寄信給作者"
			var buttonText string
			if len(mailDataList) > 1 {
				buttonText = "📧 寄信給#" + strconv.Itoa(mailData.ArticleIndex) + "作者"
			} else {
				buttonText = "📧 寄信給作者"
			}
			buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(buttonText, callbackData))
		}
	}

	if len(buttons) > 0 {
		// Create rows with 2 buttons each
		var rows [][]tgbotapi.InlineKeyboardButton
		for i := 0; i < len(buttons); i += 2 {
			if i+1 < len(buttons) {
				rows = append(rows, tgbotapi.NewInlineKeyboardRow(buttons[i], buttons[i+1]))
			} else {
				rows = append(rows, tgbotapi.NewInlineKeyboardRow(buttons[i]))
			}
		}
		msg.ReplyMarkup = tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
	}

	_, err := bot.Send(msg)
	if err != nil {
		log.WithError(err).Error("Telegram Send Message With Mail Button Failed")
	}
}

// handleMailPreview shows mail preview with confirm/cancel buttons
func handleMailPreview(callbackData string, chatID int64) {
	// Parse callback data: m_p:<userID>:<subID>:<author>
	parts := strings.Split(callbackData, ":")
	if len(parts) != 4 {
		SendTextMessage(chatID, "❌ 無效的請求")
		return
	}

	userID, err := strconv.Atoi(parts[1])
	if err != nil {
		SendTextMessage(chatID, "❌ 無效的使用者 ID")
		return
	}

	subID, err := strconv.Atoi(parts[2])
	if err != nil {
		SendTextMessage(chatID, "❌ 無效的訂閱 ID")
		return
	}

	recipient := parts[3]
	if recipient == "" {
		SendTextMessage(chatID, "❌ 無效的收件者")
		return
	}

	// Get subscription to get mail template
	subRepo := &account.SubscriptionPostgres{}
	sub, err := subRepo.FindByID(subID)
	if err != nil {
		log.WithError(err).Error("Failed to find subscription for mail preview")
		SendTextMessage(chatID, "📭 找不到訂閱設定")
		return
	}

	// Check ownership
	if sub.UserID != userID {
		SendTextMessage(chatID, "🚫 無權限使用此訂閱")
		return
	}

	// Check mail template
	if sub.Mail == nil || (sub.Mail.Subject == "" && sub.Mail.Content == "") {
		SendTextMessage(chatID, "📝 此訂閱尚未設定信件模板")
		return
	}

	// Build preview message
	previewText := "📧 寄信預覽\n\n"
	previewText += "收件人: " + recipient + "\n"
	previewText += "標題: " + sub.Mail.Subject + "\n"
	previewText += "─────────────\n"
	previewText += sub.Mail.Content

	// Create confirm callback data: m_c:<userID>:<subID>:<author>
	confirmData := "m_c:" + strconv.Itoa(userID) + ":" +
		strconv.Itoa(subID) + ":" + recipient

	// Send preview with confirm/cancel buttons
	msg := tgbotapi.NewMessage(chatID, previewText)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ 寄信", confirmData),
			tgbotapi.NewInlineKeyboardButtonData("❌ 取消", "m_x"),
		),
	)

	_, err = bot.Send(msg)
	if err != nil {
		log.WithError(err).Error("Failed to send mail preview")
	}
}

// handleMailConfirm handles the mail confirm button callback
func handleMailConfirm(callbackData string, chatID int64) string {
	// Parse callback data: m_c:<userID>:<subID>:<author>
	parts := strings.Split(callbackData, ":")
	if len(parts) != 4 {
		return "❌ 無效的請求"
	}

	userID, err := strconv.Atoi(parts[1])
	if err != nil {
		return "❌ 無效的使用者 ID"
	}

	subID, err := strconv.Atoi(parts[2])
	if err != nil {
		return "❌ 無效的訂閱 ID"
	}

	recipient := parts[3]
	if recipient == "" {
		return "❌ 無效的收件者"
	}

	// Get subscription to get mail template
	subRepo := &account.SubscriptionPostgres{}
	sub, err := subRepo.FindByID(subID)
	if err != nil {
		log.WithError(err).Error("Failed to find subscription for mail")
		return "📭 找不到訂閱設定"
	}

	// Check ownership
	if sub.UserID != userID {
		return "🚫 無權限使用此訂閱"
	}

	// Check mail template
	if sub.Mail == nil || (sub.Mail.Subject == "" && sub.Mail.Content == "") {
		return "📝 此訂閱尚未設定信件模板"
	}

	// Get PTT credentials
	pttRepo := &account.PTTAccountPostgres{}
	pttUsername, pttPassword, err := pttRepo.GetCredentials(userID)
	if err != nil {
		if err == account.ErrPTTAccountNotFound {
			return "⚠️ 尚未綁定 PTT 帳號"
		}
		log.WithError(err).Error("Failed to get PTT credentials")
		return "❌ 取得 PTT 帳號失敗"
	}

	// Send PTT mail
	mailClient := mail.NewPTTClient(pttUsername, pttPassword)
	err = mailClient.SendMail(recipient, sub.Mail.Subject, sub.Mail.Content)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"user_id":   userID,
			"recipient": recipient,
		}).Error("Failed to send PTT mail")

		if err == mail.ErrLoginFailed {
			return "🔑 帳號密碼錯誤，請重新設定"
		}
		if err == mail.ErrUserNotFound {
			return "👤 找不到此 PTT 使用者"
		}
		return "❌ 寄信失敗，請稍後再試"
	}

	log.WithFields(log.Fields{
		"user_id":   userID,
		"recipient": recipient,
		"subject":   sub.Mail.Subject,
	}).Info("PTT mail sent successfully via Telegram button")

	return "✅ 已成功寄信給 " + recipient
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
