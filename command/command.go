package command

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/models"
	"github.com/Ptt-Alertor/ptt-alertor/models/account"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/top"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"
)

const subArticlesLimit int = 50
const updateFailedMsg string = "失敗，請嘗試封鎖再解封鎖，並重新執行註冊步驟。\n若問題未解決，請至粉絲團或 LINE 首頁留言。"

var subscriptionRepo = &account.SubscriptionPostgres{}
var accountRepoCmd = &account.Postgres{}

var inputErrorTips = []string{
	"指令格式錯誤。",
	"1. 需以空白分隔動作、板名、參數",
	"2. 板名欄位開頭與結尾不可有逗號",
	"3. 板名欄位間不允許空白字元。",
}

// CommandItem represents a command and its description
type CommandItem struct {
	Cmd string
	Doc string
}

// CommandCategory represents a category of commands
type CommandCategory struct {
	Name  string
	Items []CommandItem
}

// Commands is ordered commands documents
var Commands = []CommandCategory{
	{
		Name: "一般",
		Items: []CommandItem{
			{"指令", "可使用的指令清單"},
			{"清單", "設定的看板、關鍵字、作者"},
			{"排行", "前五名追蹤的關鍵字、作者"},
		},
	},
	{
		Name: "關鍵字相關",
		Items: []CommandItem{
			{"新增 看板 關鍵字", "新增追蹤關鍵字"},
			{"刪除 看板 關鍵字", "取消追蹤關鍵字"},
			{"範例", "新增 gossiping,movie 金城武,結衣"},
		},
	},
	{
		Name: "作者相關",
		Items: []CommandItem{
			{"新增作者 看板 作者", "新增追蹤作者"},
			{"刪除作者 看板 作者", "取消追蹤作者"},
			{"範例", "新增作者 gossiping ffaarr,obov"},
		},
	},
	{
		Name: "推噓文數相關",
		Items: []CommandItem{
			{"新增(推/噓)文數 看板 總數", "通知推或噓文數"},
			{"範例", "新增推文數 joke,beauty 10"},
			{"歸零即刪除", "新增噓文數 joke 0"},
		},
	},
	{
		Name: "推文相關",
		Items: []CommandItem{
			{"新增推文 網址", "新增推文追蹤"},
			{"刪除推文 網址", "刪除推文追蹤"},
			{"推文清單", "查看追蹤的文章"},
			{"清理推文", "清理已失效的文章"},
			{"範例", "新增推文 https://www.ptt.cc/bbs/EZsoft/M.1497363598.A.74E.html"},
		},
	},
	{
		Name: "進階應用",
		Items: []CommandItem{
			{"參考連結", "https://ptt.luan.com.tw/docs"},
		},
	},
}

// HandleCommand handles command from chatbot
func HandleCommand(text string, userID string, isUser bool) string {
	command := strings.ToLower(strings.Fields(strings.TrimSpace(text))[0])
	if isUser {
		log.WithFields(log.Fields{
			"account": userID,
			"command": command,
		}).Info("Command Request")
	}
	switch command {
	case "debug":
		return handleDebug(userID)
	case "清單", "list":
		return handleList(userID)
	case "指令", "help":
		return stringCommands()
	case "排行", "ranking":
		return listTop()
	case "新增", "刪除":
		re := regexp.MustCompile("^(新增|刪除)\\s+([^,，][\\w-_,，\\.]*[^,，:\\s]):?\\s+(\\*|.*[^\\s])")
		if matched := re.MatchString(text); !matched {
			errorTips := inputErrorTips
			additionalTips := []string{
				"正確範例：",
				command + " gossiping,lol 問卦,爆卦",
			}
			errorTips = append(errorTips, additionalTips...)
			return strings.Join(errorTips, "\n")
		}
		args := re.FindStringSubmatch(text)
		result, err := handleKeyword(command, userID, args[2], args[3])
		if err != nil {
			return err.Error()
		}
		return result
	case "新增作者", "刪除作者":
		re := regexp.MustCompile("^(新增作者|刪除作者)\\s+([^,，][\\w-_,，\\.]*[^,，:\\s]):?\\s+(\\*|[\\s,\\w]+)$")
		matched := re.MatchString(text)
		if !matched {
			errorTips := inputErrorTips
			additionalTips := []string{
				"4. 作者為半形英文與數字組成。",
				"正確範例：",
				command + " gossiping,lol ffaarr,obov",
			}
			errorTips = append(errorTips, additionalTips...)
			return strings.Join(errorTips, "\n")
		}
		args := re.FindStringSubmatch(text)
		result, err := handleAuthor(command, userID, args[2], args[3])
		if err != nil {
			return err.Error()
		}
		return result
	case "新增推文數", "新增噓文數":
		re := regexp.MustCompile("^(新增推文數|新增噓文數)\\s+([^,，][\\w-_,，\\.]*[^,，:\\s]):?\\s+(100|[1-9][0-9]|[0-9])$")
		matched := re.MatchString(text)
		if !matched {
			errorTips := inputErrorTips
			additionalTips := []string{
				"4. 推噓文數需為介於 0-100 的數字",
				"正確範例：",
				command + " gossiping,beauty 100",
			}
			errorTips = append(errorTips, additionalTips...)
			return strings.Join(errorTips, "\n")
		}
		args := re.FindStringSubmatch(text)
		result, err := handlePushSum(command, userID, args[2], args[3])
		if err != nil {
			return err.Error()
		}
		return result
	case "新增推文", "刪除推文":
		re := regexp.MustCompile("^(新增推文|刪除推文)\\s+https?://www.ptt.cc/bbs/([\\w-_]*)/(M\\.\\d+.A.\\w*)\\.html$")
		matched := re.MatchString(text)
		if !matched {
			errorTips := []string{
				"指令格式錯誤。",
				"1. 網址與指令需至少一個空白。",
				"2. 網址錯誤格式。",
				"正確範例：",
				command + " https://www.ptt.cc/bbs/EZsoft/M.1497363598.A.74E.html",
			}
			return strings.Join(errorTips, "\n")
		}
		args := re.FindStringSubmatch(text)
		result, err := handleComment(command, userID, args[2], args[3])
		if err != nil {
			return err.Error()
		}
		return result
	case "清理推文":
		return cleanCommentList(userID)
	case "推文清單":
		return handleCommentList(userID)
	}
	if !isUser {
		return ""
	}
	return "無此指令，請打「指令」查看指令清單"
}

func handleDebug(account string) string {
	return models.User().Find(account).Profile.Account
}

func handleList(chatID string) string {
	// Get PostgreSQL userID from chatID
	userID, err := account.GetUserIDByTelegramChatID(chatID)
	if err != nil {
		if errors.Is(err, account.ErrUserNotBound) {
			return "請先綁定帳號，輸入 /bind"
		}
		log.WithError(err).Error("Failed to get userID from chatID")
		return "取得用戶資料失敗"
	}

	result, err := subscriptionRepo.ListFormatted(userID)
	if err != nil {
		log.WithError(err).Error("Failed to get subscription list")
		return "取得訂閱列表失敗"
	}
	return result
}

func cleanCommentList(chatID string) string {
	// Get PostgreSQL userID from chatID
	userID, err := account.GetUserIDByTelegramChatID(chatID)
	if err != nil {
		if errors.Is(err, account.ErrUserNotBound) {
			return "請先綁定帳號，輸入 /bind"
		}
		return "取得用戶資料失敗"
	}

	// Get all article subscriptions
	subs, err := subscriptionRepo.ListByUserIDAndType(userID, "article")
	if err != nil {
		return "取得推文清單失敗"
	}

	var cleaned int
	for _, sub := range subs {
		a := models.Article()
		a.Code = sub.Value
		exists, err := a.Exist()
		if err != nil {
			continue
		}
		if !exists {
			// Delete invalid article subscription
			subscriptionRepo.Delete(sub.ID, userID)
			cleaned++
		}
	}
	return fmt.Sprintf("清理 %d 則推文", cleaned)
}

func handleCommentList(chatID string) string {
	// Get PostgreSQL userID from chatID
	userID, err := account.GetUserIDByTelegramChatID(chatID)
	if err != nil {
		if errors.Is(err, account.ErrUserNotBound) {
			return "請先綁定帳號，輸入 /bind"
		}
		return "取得用戶資料失敗"
	}

	// Get all article subscriptions
	subs, err := subscriptionRepo.ListByUserIDAndType(userID, "article")
	if err != nil {
		return "取得推文清單失敗"
	}

	if len(subs) == 0 {
		return "尚未追蹤任何文章。"
	}

	var result strings.Builder
	result.WriteString("推文追蹤清單，上限 50 篇：\n")
	for _, sub := range subs {
		result.WriteString(fmt.Sprintf("- %s: %s\n", sub.Board, sub.Value))
	}
	result.WriteString("\n輸入「清理推文」，可刪除無效連結。")
	return result.String()
}

func stringCommands() string {
	str := ""
	for _, cat := range Commands {
		str += "[" + cat.Name + "]\n"
		for _, item := range cat.Items {
			str += item.Cmd
			if item.Doc != "" {
				str += "：" + item.Doc
			}
			str += "\n"
		}
		str += "\n"
	}
	return strings.TrimSpace(str)
}

func listTop() string {
	return top.ListTopFormatted(5)
}

func handleKeyword(command, chatID, boardStr, keywordStr string) (string, error) {
	// Get PostgreSQL userID from chatID
	userID, err := account.GetUserIDByTelegramChatID(chatID)
	if err != nil {
		if errors.Is(err, account.ErrUserNotBound) {
			return "", errors.New("請先綁定帳號，輸入 /bind")
		}
		return "", errors.New("取得用戶資料失敗")
	}

	// Get account for role check
	acc, err := accountRepoCmd.FindByID(userID)
	if err != nil {
		return "", errors.New("取得帳號資料失敗")
	}

	isAdd := strings.HasPrefix(command, "新增")

	// Check subscription limit for add commands
	if isAdd {
		if err := subscriptionRepo.CheckLimit(userID, acc.Role); err != nil {
			if errors.Is(err, account.ErrSubscriptionLimitReached) {
				return "", errors.New("已達訂閱上限")
			}
			return "", errors.New("檢查訂閱限制失敗")
		}
	}

	boardNames := splitParamString(boardStr)
	var keywords []string
	if strings.HasPrefix(keywordStr, "regexp:") {
		if !checkRegexp(keywordStr) {
			return "", errors.New("正規表示式錯誤，請檢查規則。")
		}
		keywords = []string{keywordStr}
	} else {
		keywords = splitParamString(keywordStr)
	}

	log.WithFields(log.Fields{
		"id":      chatID,
		"userID":  userID,
		"command": command,
		"boards":  boardNames,
		"words":   keywords,
	}).Info("Keyword Command")

	// Process each board and keyword combination
	for _, boardName := range boardNames {
		for _, kw := range keywords {
			if isAdd {
				_, err := subscriptionRepo.Create(userID, boardName, "keyword", kw)
				if err != nil {
					if errors.Is(err, account.ErrSubscriptionExists) {
						continue // Skip if already exists
					}
					if errors.Is(err, account.ErrBoardNotFound) {
						return "", errors.New("板名錯誤，請確認拼字。")
					}
					if errors.Is(err, account.ErrSubscriptionLimitReached) {
						return "", errors.New("已達訂閱上限")
					}
					log.WithError(err).Error("Keyword Create Failed")
					return "", errors.New(command + updateFailedMsg)
				}
			} else {
				err := subscriptionRepo.DeleteByValue(userID, boardName, "keyword", kw)
				if err != nil {
					if errors.Is(err, account.ErrSubscriptionNotFound) {
						continue // Skip if not found
					}
					log.WithError(err).Error("Keyword Delete Failed")
					return "", errors.New(command + updateFailedMsg)
				}
			}
		}
	}

	return command + "成功", nil
}

func handleAuthor(command, chatID, boardStr, authorStr string) (string, error) {
	// Get PostgreSQL userID from chatID
	userID, err := account.GetUserIDByTelegramChatID(chatID)
	if err != nil {
		if errors.Is(err, account.ErrUserNotBound) {
			return "", errors.New("請先綁定帳號，輸入 /bind")
		}
		return "", errors.New("取得用戶資料失敗")
	}

	// Get account for role check
	acc, err := accountRepoCmd.FindByID(userID)
	if err != nil {
		return "", errors.New("取得帳號資料失敗")
	}

	isAdd := strings.HasPrefix(command, "新增")

	// Check subscription limit for add commands
	if isAdd {
		if err := subscriptionRepo.CheckLimit(userID, acc.Role); err != nil {
			if errors.Is(err, account.ErrSubscriptionLimitReached) {
				return "", errors.New("已達訂閱上限")
			}
			return "", errors.New("檢查訂閱限制失敗")
		}
	}

	if ok, _ := regexp.MatchString("^(\\*|[\\s,\\w]+)$", authorStr); !ok {
		return "", errors.New("作者為半形英文與數字組成。")
	}

	boardNames := splitParamString(boardStr)
	authors := splitParamString(authorStr)

	log.WithFields(log.Fields{
		"id":      chatID,
		"userID":  userID,
		"command": command,
		"boards":  boardNames,
		"words":   authors,
	}).Info("Author Command")

	// Process each board and author combination
	for _, boardName := range boardNames {
		for _, author := range authors {
			if isAdd {
				_, err := subscriptionRepo.Create(userID, boardName, "author", author)
				if err != nil {
					if errors.Is(err, account.ErrSubscriptionExists) {
						continue // Skip if already exists
					}
					if errors.Is(err, account.ErrBoardNotFound) {
						return "", errors.New("板名錯誤，請確認拼字。")
					}
					if errors.Is(err, account.ErrSubscriptionLimitReached) {
						return "", errors.New("已達訂閱上限")
					}
					log.WithError(err).Error("Author Create Failed")
					return "", errors.New(command + updateFailedMsg)
				}
			} else {
				err := subscriptionRepo.DeleteByValue(userID, boardName, "author", author)
				if err != nil {
					if errors.Is(err, account.ErrSubscriptionNotFound) {
						continue // Skip if not found
					}
					log.WithError(err).Error("Author Delete Failed")
					return "", errors.New(command + updateFailedMsg)
				}
			}
		}
	}

	return command + "成功", nil
}

func handlePushSum(command, chatID, boardStr, sumStr string) (string, error) {
	// Get PostgreSQL userID from chatID
	userID, err := account.GetUserIDByTelegramChatID(chatID)
	if err != nil {
		if errors.Is(err, account.ErrUserNotBound) {
			return "", errors.New("請先綁定帳號，輸入 /bind")
		}
		return "", errors.New("取得用戶資料失敗")
	}

	// Get account for role check
	acc, err := accountRepoCmd.FindByID(userID)
	if err != nil {
		return "", errors.New("取得帳號資料失敗")
	}

	// Check subscription limit for add commands (pushsum 0 means delete)
	sum, err := strconv.Atoi(sumStr)
	if err != nil || sum < 0 || sum > 100 {
		return "", errors.New("推噓文數需為介於 0-100 的數字")
	}

	isAdd := sum > 0
	if isAdd {
		if err := subscriptionRepo.CheckLimit(userID, acc.Role); err != nil {
			if errors.Is(err, account.ErrSubscriptionLimitReached) {
				return "", errors.New("已達訂閱上限")
			}
			return "", errors.New("檢查訂閱限制失敗")
		}
	}

	boardNames := splitParamString(boardStr)

	for _, boardName := range boardNames {
		if strings.EqualFold(boardName, "allpost") {
			return "", errors.New("推文數通知不支持 ALLPOST 板。")
		}
	}

	// Determine sub_type based on command
	subType := "pushsum"
	var value string
	if strings.Contains(command, "推文數") {
		value = "+" + sumStr // up pushsum
	} else {
		value = "-" + sumStr // down pushsum (boo)
	}

	log.WithFields(log.Fields{
		"id":      chatID,
		"userID":  userID,
		"command": command,
		"boards":  boardNames,
		"value":   value,
	}).Info("PushSum Command")

	// Process each board
	for _, boardName := range boardNames {
		if isAdd {
			// Create or update pushsum subscription
			_, err := subscriptionRepo.Create(userID, boardName, subType, value)
			if err != nil {
				if errors.Is(err, account.ErrSubscriptionExists) {
					// Update existing pushsum subscription
					// For pushsum, we need to find and update the existing one
					continue // For simplicity, skip if exists (user should delete first)
				}
				if errors.Is(err, account.ErrBoardNotFound) {
					return "", errors.New("板名錯誤，請確認拼字。")
				}
				if errors.Is(err, account.ErrSubscriptionLimitReached) {
					return "", errors.New("已達訂閱上限")
				}
				log.WithError(err).Error("PushSum Create Failed")
				return "", errors.New(command + updateFailedMsg)
			}
		} else {
			// Delete pushsum subscription (sum == 0)
			err := subscriptionRepo.DeleteByValue(userID, boardName, subType, value)
			if err != nil {
				if errors.Is(err, account.ErrSubscriptionNotFound) {
					continue // Skip if not found
				}
				log.WithError(err).Error("PushSum Delete Failed")
				return "", errors.New(command + updateFailedMsg)
			}
		}
	}

	return command + "成功", nil
}

func handleComment(command, chatID, boardName, articleCode string) (string, error) {
	// Get PostgreSQL userID from chatID
	userID, err := account.GetUserIDByTelegramChatID(chatID)
	if err != nil {
		if errors.Is(err, account.ErrUserNotBound) {
			return "", errors.New("請先綁定帳號，輸入 /bind")
		}
		return "", errors.New("取得用戶資料失敗")
	}

	log.WithFields(log.Fields{
		"id":      chatID,
		"userID":  userID,
		"command": command,
		"board":   boardName,
		"article": articleCode,
	}).Info("Comment Command")

	isAdd := strings.EqualFold(command, "新增推文")

	if isAdd {
		// Check if article exists
		if !checkArticleExist(boardName, articleCode) {
			return "", errors.New("文章不存在")
		}
		// Check article tracking limit (50)
		count, err := subscriptionRepo.CountByUserIDAndType(userID, "article")
		if err != nil {
			log.WithError(err).Error("Failed to count article subscriptions")
			return "", errors.New("檢查訂閱數量失敗")
		}
		if count >= subArticlesLimit {
			return "", errors.New("推文追蹤最多 50 篇，輸入「推文清單」，整理追蹤列表。")
		}

		// Create article subscription
		_, err = subscriptionRepo.Create(userID, boardName, "article", articleCode)
		if err != nil {
			if errors.Is(err, account.ErrSubscriptionExists) {
				return "", errors.New("已追蹤此文章")
			}
			log.WithError(err).Error("Article Create Failed")
			return "", errors.New(command + updateFailedMsg)
		}
	} else {
		// Delete article subscription
		err := subscriptionRepo.DeleteByValue(userID, boardName, "article", articleCode)
		if err != nil {
			if errors.Is(err, account.ErrSubscriptionNotFound) {
				return "", errors.New("未追蹤此文章")
			}
			log.WithError(err).Error("Article Delete Failed")
			return "", errors.New(command + updateFailedMsg)
		}
	}

	return command + "成功", nil
}

func checkArticleExist(boardName, articleCode string) bool {
	a := models.Article()
	a.Code = articleCode
	if bl, _ := a.Exist(); bl {
		return true
	}
	if web.CheckArticleExist(boardName, articleCode) {
		a.Board = boardName
		initialArticle(a)
		return true
	}
	return false
}

func initialArticle(a *article.Article) error {
	atcl, err := web.FetchArticle(a.Board, a.Code)
	if err != nil {
		return err
	}
	a.Link = atcl.Link
	a.Title = atcl.Title
	a.ID = atcl.ID
	a.LastPushDateTime = atcl.LastPushDateTime
	a.Comments = atcl.Comments
	return a.Save()
}


func checkRegexp(input string) bool {
	pattern := strings.Replace(strings.TrimPrefix(input, "regexp:"), "//", "////", -1)
	_, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return true
}

func splitParamString(paramString string) (params []string) {
	paramString = strings.Trim(paramString, ",，")
	if !strings.ContainsAny(paramString, ",，") {
		return []string{paramString}
	}

	if strings.Contains(paramString, ",") {
		params = strings.Split(paramString, ",")
	} else {
		params = []string{paramString}
	}

	for i := 0; i < len(params); i++ {
		if strings.Contains(params[i], "，") {
			params = append(params[:i], append(strings.Split(params[i], "，"), params[i+1:]...)...)
			i--
		}
	}

	for i, param := range params {
		params[i] = strings.TrimSpace(param)
	}

	return params
}

