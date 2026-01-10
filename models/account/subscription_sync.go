package account

import (
	"encoding/json"
	"fmt"
	"strconv"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/models/binding"
	"github.com/Ptt-Alertor/ptt-alertor/models/subscription"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
)

// RedisSync handles syncing web user subscriptions to Redis
type RedisSync struct{}

var bindingRepo = &binding.Postgres{}

// WebAccountPrefix is the prefix for web user accounts in Redis
const WebAccountPrefix = "web_"

// GetWebAccount returns the Redis account key for a web user
func GetWebAccount(userID int) string {
	return fmt.Sprintf("%s%d", WebAccountPrefix, userID)
}

// SyncSubscriptionCreate syncs a new subscription to Redis
func (rs *RedisSync) SyncSubscriptionCreate(sub *Subscription, acc *Account) error {
	// Check if user has telegram binding
	telegramBinding, err := bindingRepo.FindByUserAndService(acc.ID, binding.ServiceTelegram)
	if err != nil || telegramBinding.ServiceID == "" {
		// User hasn't bound Telegram, no need to sync
		return nil
	}

	account := GetWebAccount(acc.ID)

	// 1. Add to board subscriber set
	if err := rs.addToSubscriberSet(sub.Board, sub.SubType, account); err != nil {
		return err
	}

	// 2. Update user data in Redis
	if err := rs.updateUserData(acc); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"account":  account,
		"board":    sub.Board,
		"sub_type": sub.SubType,
		"value":    sub.Value,
	}).Info("Synced subscription to Redis")

	return nil
}

// SyncSubscriptionDelete syncs a subscription deletion to Redis
func (rs *RedisSync) SyncSubscriptionDelete(sub *Subscription, acc *Account) error {
	account := GetWebAccount(acc.ID)

	// 1. Check if user has other subscriptions for the same board+type
	subs, err := (&SubscriptionPostgres{}).ListByUserID(acc.ID)
	if err != nil {
		return err
	}

	hasOtherSubs := false
	for _, s := range subs {
		if s.ID != sub.ID && s.Board == sub.Board && s.SubType == sub.SubType {
			hasOtherSubs = true
			break
		}
	}

	// 2. If no other subscriptions for this board+type, remove from subscriber set
	if !hasOtherSubs {
		if err := rs.removeFromSubscriberSet(sub.Board, sub.SubType, account); err != nil {
			return err
		}
	}

	// 3. Update user data in Redis
	if err := rs.updateUserData(acc); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"account":  account,
		"board":    sub.Board,
		"sub_type": sub.SubType,
		"value":    sub.Value,
	}).Info("Removed subscription from Redis")

	return nil
}

// SyncUserDelete removes all user data from Redis when user is deleted
func (rs *RedisSync) SyncUserDelete(acc *Account) error {
	account := GetWebAccount(acc.ID)
	conn := connections.Redis()
	defer conn.Close()

	// Delete user data
	userKey := "user:" + account
	_, err := conn.Do("DEL", userKey)
	if err != nil {
		log.WithError(err).Error("Failed to delete user from Redis")
		return err
	}

	log.WithField("account", account).Info("Deleted user from Redis")
	return nil
}

// addToSubscriberSet adds account to the board's subscriber set
func (rs *RedisSync) addToSubscriberSet(board, subType, account string) error {
	conn := connections.Redis()
	defer conn.Close()

	// Add board to boards set (so checker will scan it)
	_, err := conn.Do("SADD", "boards", board)
	if err != nil {
		log.WithError(err).Error("Failed to add board to boards set")
		return err
	}

	// Add account to subscriber set
	key := fmt.Sprintf("%s:%s:subs", subType, board)
	_, err = conn.Do("SADD", key, account)
	if err != nil {
		log.WithError(err).Error("Failed to add to subscriber set")
		return err
	}
	return nil
}

// removeFromSubscriberSet removes account from the board's subscriber set
func (rs *RedisSync) removeFromSubscriberSet(board, subType, account string) error {
	conn := connections.Redis()
	defer conn.Close()

	key := fmt.Sprintf("%s:%s:subs", subType, board)
	_, err := conn.Do("SREM", key, account)
	if err != nil {
		log.WithError(err).Error("Failed to remove from subscriber set")
		return err
	}
	return nil
}

// updateUserData updates the user data in Redis
func (rs *RedisSync) updateUserData(acc *Account) error {
	// Get telegram binding
	telegramBinding, err := bindingRepo.FindByUserAndService(acc.ID, binding.ServiceTelegram)
	if err != nil || telegramBinding.ServiceID == "" {
		return nil
	}

	// Parse telegram chat ID
	chatID, err := strconv.ParseInt(telegramBinding.ServiceID, 10, 64)
	if err != nil {
		log.WithError(err).Error("Failed to parse telegram chat ID")
		return err
	}

	// Get all subscriptions for this user
	subs, err := (&SubscriptionPostgres{}).ListByUserID(acc.ID)
	if err != nil {
		return err
	}

	// Build user struct for Redis
	account := GetWebAccount(acc.ID)
	u := user.User{
		Enable: acc.Enabled,
		Profile: user.Profile{
			Account:      account,
			Telegram:     account,
			TelegramChat: chatID,
		},
		Subscribes: buildSubscriptions(subs),
	}

	// Save to Redis
	conn := connections.Redis()
	defer conn.Close()

	userKey := "user:" + account
	uJSON, err := json.Marshal(u)
	if err != nil {
		log.WithError(err).Error("Failed to marshal user data")
		return err
	}

	_, err = conn.Do("SET", userKey, uJSON)
	if err != nil {
		log.WithError(err).Error("Failed to save user to Redis")
		return err
	}

	return nil
}

// buildSubscriptions converts account subscriptions to user subscription format
func buildSubscriptions(subs []*Subscription) []subscription.Subscription {
	// Group by board
	boardMap := make(map[string]*subscription.Subscription)

	for _, sub := range subs {
		if !sub.Enabled {
			continue
		}

		if _, exists := boardMap[sub.Board]; !exists {
			boardMap[sub.Board] = &subscription.Subscription{
				Board:    sub.Board,
				Keywords: []string{},
				Authors:  []string{},
			}
		}

		switch sub.SubType {
		case "keyword":
			boardMap[sub.Board].Keywords = append(boardMap[sub.Board].Keywords, sub.Value)
		case "author":
			boardMap[sub.Board].Authors = append(boardMap[sub.Board].Authors, sub.Value)
		case "pushsum":
			// Parse pushsum value (e.g., "50" or "-20")
			ps := parsePushSum(sub.Value)
			boardMap[sub.Board].PushSum = ps
		}
	}

	// Convert map to slice
	result := make([]subscription.Subscription, 0, len(boardMap))
	for _, s := range boardMap {
		result = append(result, *s)
	}

	return result
}

// parsePushSum parses pushsum value string
func parsePushSum(value string) subscription.PushSum {
	var ps subscription.PushSum
	var num int
	fmt.Sscanf(value, "%d", &num)
	if num > 0 {
		ps.Up = num
	} else if num < 0 {
		ps.Down = num
	}
	return ps
}

// SyncAllSubscriptions syncs all subscriptions for a user to Redis
// This should be called after Telegram binding to sync existing subscriptions
func (rs *RedisSync) SyncAllSubscriptions(userID int) error {
	acc, err := (&Postgres{}).FindByID(userID)
	if err != nil {
		log.WithError(err).Error("Failed to find account for Redis sync")
		return err
	}

	subs, err := (&SubscriptionPostgres{}).ListByUserID(userID)
	if err != nil {
		log.WithError(err).Error("Failed to list subscriptions for Redis sync")
		return err
	}

	if len(subs) == 0 {
		return nil
	}

	// Sync each subscription to Redis
	for _, sub := range subs {
		if err := rs.SyncSubscriptionCreate(sub, acc); err != nil {
			log.WithFields(log.Fields{
				"user_id":  userID,
				"sub_id":   sub.ID,
				"board":    sub.Board,
				"sub_type": sub.SubType,
			}).WithError(err).Error("Failed to sync subscription to Redis")
		}
	}

	log.WithFields(log.Fields{
		"user_id": userID,
		"count":   len(subs),
	}).Info("Synced existing subscriptions to Redis after Telegram binding")

	return nil
}
