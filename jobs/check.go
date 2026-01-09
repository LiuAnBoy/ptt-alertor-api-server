package jobs

import (
	"strconv"
	"strings"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/channels/telegram"
	accountModel "github.com/Ptt-Alertor/ptt-alertor/models/account"
	"github.com/Ptt-Alertor/ptt-alertor/models/counter"
)

const workers = 300

var ckCh = make(chan check)

func init() {
	for i := 0; i < workers; i++ {
		go messageWorker(ckCh)
	}
}

func messageWorker(ckCh chan check) {
	for {
		ck := <-ckCh
		sendMessage(ck)
	}
}

type check interface {
	String() string
	Self() Checker
	Stop()
	Run()
}

func sendMessage(c check) {
	cr := c.Self()
	account := cr.Profile.Account

	if cr.Profile.Telegram == "" {
		log.WithFields(log.Fields{
			"account": account,
			"board":   cr.board,
			"type":    cr.subType,
			"word":    cr.word,
		}).Warn("Message Sent without Telegram Connection")
		return
	}

	sendTelegram(c)
	counter.IncrAlert()
	log.WithFields(log.Fields{
		"account":  account,
		"platform": "telegram",
		"board":    cr.board,
		"type":     cr.subType,
		"word":     cr.word,
	}).Info("Message Sent")
}

func sendTelegram(c check) {
	cr := c.Self()
	chatID := cr.Profile.TelegramChat
	text := c.String()

	// Check if we should show mail buttons
	mailDataList := getMailButtonData(cr)

	if len(mailDataList) > 0 {
		telegram.SendMessageWithMailButton(chatID, text, mailDataList)
	} else {
		telegram.SendTextMessage(chatID, text)
	}
}

// getMailButtonData checks if mail button should be shown and returns data for it
// Returns nil if conditions are not met
func getMailButtonData(cr Checker) []*telegram.MailButtonData {
	// Must have at least one article
	if len(cr.articles) == 0 {
		return nil
	}

	// Check if this is a web account (format: web_<userID>)
	account := cr.Profile.Account
	if !strings.HasPrefix(account, accountModel.WebAccountPrefix) {
		return nil
	}

	// Parse user ID from account
	userIDStr := strings.TrimPrefix(account, accountModel.WebAccountPrefix)
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		return nil
	}

	// Check user role (VIP or admin only)
	accRepo := &accountModel.Postgres{}
	acc, err := accRepo.FindByID(userID)
	if err != nil || (acc.Role != "vip" && acc.Role != "admin") {
		return nil
	}

	// Check if PTT account is bound
	pttRepo := &accountModel.PTTAccountPostgres{}
	hasPTT, err := pttRepo.Exists(userID)
	if err != nil || !hasPTT {
		return nil
	}

	// Find subscription with mail template
	subRepo := &accountModel.SubscriptionPostgres{}
	subs, err := subRepo.ListByUserID(userID)
	if err != nil {
		return nil
	}

	// Find matching subscription with mail template
	var matchingSub *accountModel.Subscription
	for _, sub := range subs {
		if sub.Board == cr.board && sub.SubType == cr.subType && sub.Value == cr.word && sub.Enabled {
			if sub.Mail != nil && (sub.Mail.Subject != "" || sub.Mail.Content != "") {
				matchingSub = sub
				break
			}
		}
	}

	if matchingSub == nil {
		return nil
	}

	// Create mail button data for each article with author
	var mailDataList []*telegram.MailButtonData
	for i, article := range cr.articles {
		if article.Author == "" {
			continue
		}
		mailDataList = append(mailDataList, &telegram.MailButtonData{
			UserID:         userID,
			SubscriptionID: matchingSub.ID,
			ArticleAuthor:  article.Author,
			ArticleIndex:   i + 1,    // 1-based index
		})
	}

	return mailDataList
}
