package jobs

import (
	"errors"

	"github.com/Ptt-Alertor/ptt-alertor/models"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
)

var platforms = map[string]bool{
	"telegram": true,
}

type Broadcaster struct {
	Checker
	Msg string
}

func (bc Broadcaster) String() string {
	return bc.Msg
}

func (bc Broadcaster) Send(plfms []string) error {
	var platformBl = make(map[string]bool)
	for _, plfm := range plfms {
		if _, ok := platforms[plfm]; !ok {
			return errors.New("platform " + plfm + " is not in broadcast list")
		}
		platformBl[plfm] = true
	}

	for _, u := range models.User().All() {
		bc.subType = "broadcast"
		if platformBl["telegram"] {
			go bc.sendTelegram(u)
		}
	}
	return nil
}

func (bc Broadcaster) sendTelegram(u *user.User) {
	bc.Profile.Telegram = u.Profile.Telegram
	bc.Profile.TelegramChat = u.Profile.TelegramChat
	ckCh <- bc
}
