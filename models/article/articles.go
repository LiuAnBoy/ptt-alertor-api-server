package article

import (
	"strconv"
	"strings"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/myutil"
	"github.com/gomodule/redigo/redis"
)

type Articles []Article

func (as Articles) List() []string {
	conn := connections.Redis()
	defer conn.Close()
	keys, err := redis.Strings(conn.Do("KEYS", prefix+"*"+subsSuffix))
	if err != nil {
		log.WithField("runtime", myutil.BasicRuntimeInfo()).WithError(err).Error()
	}
	codes := make([]string, 0)
	for _, key := range keys {
		code := strings.TrimSuffix(strings.TrimPrefix(key, prefix), subsSuffix)
		codes = append(codes, code)
	}
	return codes
}

func (as Articles) String() string {
	var content string
	// Show numbered format when multiple articles
	if len(as) > 1 {
		for i, a := range as {
			content += "\r\n\r\n#" + strconv.Itoa(i+1) + "\r\n" + a.String()
		}
	} else {
		for _, a := range as {
			content += "\r\n\r\n" + a.String()
		}
	}
	return content
}

func (as Articles) StringWithPushSum() string {
	var content string
	for _, a := range as {
		content += "\r\n\r\n" + a.StringWithPushSum()
	}
	return content
}
