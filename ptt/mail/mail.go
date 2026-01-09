package mail

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/gorilla/websocket"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

const (
	pttWSURL       = "wss://ws.ptt.cc/bbs"
	pttOrigin      = "https://term.ptt.cc"
	readTimeout    = 10 * time.Second
	writeTimeout   = 5 * time.Second
	connectTimeout = 15 * time.Second
)

var (
	ErrLoginFailed    = errors.New("PTT login failed")
	ErrSendMailFailed = errors.New("failed to send PTT mail")
	ErrTimeout        = errors.New("operation timeout")
	ErrUserNotFound   = errors.New("recipient user not found")
)

// PTTClient represents a PTT WebSocket client
type PTTClient struct {
	conn       *websocket.Conn
	username   string
	password   string
	connFailed bool // Flag to indicate connection has failed
}

// NewPTTClient creates a new PTT client
func NewPTTClient(username, password string) *PTTClient {
	return &PTTClient{
		username: username,
		password: password,
	}
}

// SendMail sends a mail to a recipient on PTT
func (c *PTTClient) SendMail(recipient, subject, content string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Connect to PTT
	log.Info("Connecting to PTT WebSocket...")
	if err := c.connect(ctx); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	defer c.close()
	log.Info("PTT WebSocket connected")

	// Read immediately and stop as soon as we see login prompt
	// This prevents the connection from being closed by PTT due to inactivity
	initialScreen, _ := c.readScreen(ctx, 5*time.Second, "請輸入代號")
	log.WithField("screen", string(initialScreen)).Info("Initial screen after connect")

	// Login
	if err := c.login(ctx); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Send mail
	if err := c.sendMailInternal(ctx, recipient, subject, content); err != nil {
		return fmt.Errorf("send mail failed: %w", err)
	}

	return nil
}

// connect establishes WebSocket connection to PTT
func (c *PTTClient) connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: connectTimeout,
	}

	header := make(map[string][]string)
	header["Origin"] = []string{pttOrigin}

	conn, _, err := dialer.DialContext(ctx, pttWSURL, header)
	if err != nil {
		return err
	}

	c.conn = conn
	return nil
}

// Telnet protocol constants
const (
	IAC  = 255 // Interpret As Command
	DONT = 254
	DO   = 253
	WONT = 252
	WILL = 251
	SB   = 250 // Sub-negotiation Begin
	SE   = 240 // Sub-negotiation End

	TERMINAL_TYPE = 24
	NAWS          = 31 // Negotiate About Window Size
)

// handleTelnetNegotiation processes and responds to Telnet commands in the data
func (c *PTTClient) handleTelnetNegotiation(data []byte) {
	if len(data) < 3 {
		return
	}

	i := 0
	for i <= len(data)-3 {
		if data[i] == IAC {
			cmd := data[i+1]
			opt := data[i+2]

			switch cmd {
			case DO:
				log.WithFields(log.Fields{"cmd": "DO", "opt": opt}).Debug("Telnet negotiation")
				switch opt {
				case TERMINAL_TYPE:
					c.sendBytes([]byte{IAC, WILL, TERMINAL_TYPE})
				case NAWS:
					// Send WILL NAWS and then window size: 80x24
					c.sendBytes([]byte{IAC, WILL, NAWS})
					c.sendBytes([]byte{IAC, SB, NAWS, 0, 80, 0, 24, IAC, SE})
				default:
					// Accept other options
					c.sendBytes([]byte{IAC, WILL, opt})
				}
				i += 3
			case WILL:
				log.WithFields(log.Fields{"cmd": "WILL", "opt": opt}).Debug("Telnet negotiation")
				c.sendBytes([]byte{IAC, DO, opt})
				i += 3
			case WONT:
				log.WithFields(log.Fields{"cmd": "WONT", "opt": opt}).Debug("Telnet negotiation")
				c.sendBytes([]byte{IAC, DONT, opt})
				i += 3
			case DONT:
				log.WithFields(log.Fields{"cmd": "DONT", "opt": opt}).Debug("Telnet negotiation")
				c.sendBytes([]byte{IAC, WONT, opt})
				i += 3
			case SB:
				// Sub-negotiation - find SE
				seFound := false
				for j := i + 3; j <= len(data)-2; j++ {
					if data[j] == IAC && data[j+1] == SE {
						// Terminal type request (SB TERMINAL_TYPE 1 ... IAC SE)
						if opt == TERMINAL_TYPE && i+3 < len(data) && data[i+3] == 1 {
							log.Info("Telnet: Sending terminal type VT100")
							response := []byte{IAC, SB, TERMINAL_TYPE, 0}
							response = append(response, []byte("VT100")...)
							response = append(response, IAC, SE)
							c.sendBytes(response)
						}
						i = j + 2
						seFound = true
						break
					}
				}
				if !seFound {
					// SE not found, skip this IAC
					i++
				}
			default:
				// Unknown command or escape sequence, skip
				i++
			}
		} else {
			i++
		}
	}
}

// sendBytes sends raw bytes without encoding
func (c *PTTClient) sendBytes(data []byte) error {
	if c.conn == nil {
		return errors.New("connection is nil")
	}
	c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

// close closes the WebSocket connection
func (c *PTTClient) close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// login performs PTT login
func (c *PTTClient) login(ctx context.Context) error {
	log.Info("Starting PTT login")

	// Send username immediately (we already saw the login prompt)
	log.WithField("username", c.username).Info("Sending username")
	if err := c.sendString(c.username + "\r"); err != nil {
		return err
	}

	// Wait for password prompt
	screen, _ := c.readScreen(ctx, 3*time.Second, "請輸入您的密碼")
	if !strings.Contains(string(screen), "請輸入您的密碼") {
		log.WithField("screen", string(screen)).Info("Did not see password prompt")
	}

	// Send password
	log.Info("Sending password")
	if err := c.sendString(c.password + "\r"); err != nil {
		return err
	}

	// Handle post-login screens - keep reading until main menu or error
	for i := range 15 {
		if c.connFailed {
			log.Warn("Connection failed during login")
			break
		}

		// Read screen with multiple stop conditions
		screen, _ = c.readScreen(ctx, 3*time.Second, "主功能表", "重複登入", "按任意鍵", "密碼不對", "錯誤")
		screenStr := string(screen)

		if screenStr == "" {
			continue
		}

		log.WithFields(log.Fields{"iteration": i, "screen": screenStr}).Info("Login screen")

		// Check for login failure FIRST
		if strings.Contains(screenStr, "密碼不對") {
			return ErrLoginFailed
		}

		// Check if we reached main menu
		if strings.Contains(screenStr, "主功能表") {
			log.Info("Login successful, reached main menu")
			return nil
		}

		// Handle duplicate login IMMEDIATELY - this is time-sensitive
		if strings.Contains(screenStr, "您想刪除其他重複登入") || strings.Contains(screenStr, "重複登入") {
			log.Info("Handling duplicate login prompt, sending N")
			if err := c.sendString("n\r"); err != nil {
				log.WithError(err).Warn("Failed to send N for duplicate login")
			}
			continue
		}

		// Press space to continue through various prompts
		if strings.Contains(screenStr, "請按任意鍵繼續") ||
			strings.Contains(screenStr, "按任意鍵") ||
			strings.Contains(screenStr, "您要刪除以上錯誤嘗試") ||
			strings.Contains(screenStr, "錯誤嘗試") {
			log.Info("Pressing space to continue")
			if err := c.sendString(" "); err != nil {
				log.WithError(err).Warn("Failed to send space")
			}
			continue
		}

		// If we got here with some content but didn't match anything, try pressing Enter
		if len(screenStr) > 50 {
			log.Info("Unknown screen, pressing Enter")
			c.sendString("\r")
		}
	}

	// Even if main menu not detected, if we're not at login screen, consider it successful
	log.Info("Login flow completed")
	return nil
}

// sendMailInternal sends mail after login
func (c *PTTClient) sendMailInternal(ctx context.Context, recipient, subject, content string) error {
	log.WithField("recipient", recipient).Info("Starting mail send process")

	// Go to mail section: press 'M' for Mail
	log.Info("Pressing 'M' for Mail menu")
	if err := c.sendString("M"); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)

	// Read screen after pressing M
	screen, _ := c.readScreen(ctx, 2*time.Second)
	log.WithField("screen", string(screen)).Info("Screen after pressing M")

	// Press 'S' for Send mail
	log.Info("Pressing 'S' for Send mail")
	if err := c.sendString("S"); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)

	// Read screen after pressing S
	screen, _ = c.readScreen(ctx, 2*time.Second)
	log.WithField("screen", string(screen)).Info("Screen after pressing S")

	// Wait for recipient prompt
	if err := c.waitForScreen(ctx, "收信人", 3*time.Second); err != nil {
		log.WithField("error", err).Info("Did not find recipient prompt")
	}

	// Enter recipient
	if err := c.sendString(recipient + "\r"); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)

	// Check if user exists
	screen, _ = c.readScreen(ctx, 2*time.Second)
	screenStr := string(screen)
	log.WithField("screen", screenStr).Info("Screen after entering recipient")

	if strings.Contains(screenStr, "無此帳號") || strings.Contains(screenStr, "找不到這位使用者") {
		return ErrUserNotFound
	}

	// Wait for subject prompt
	if err := c.waitForScreen(ctx, "標題", 3*time.Second); err != nil {
		log.WithField("error", err).Info("Did not find subject prompt")
	}

	// Enter subject
	if err := c.sendString(subject + "\r"); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	// Enter content (in editor)
	time.Sleep(500 * time.Millisecond)
	log.WithField("content_lines", len(strings.Split(content, "\n"))).Info("Entering mail content")

	// Split content into lines and send
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if err := c.sendString(line + "\r"); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Save and send: Ctrl+X
	log.Info("Sending Ctrl+X to save")
	if err := c.sendByte(0x18); err != nil { // Ctrl+X
		return err
	}
	time.Sleep(500 * time.Millisecond)

	// Confirm send: press 's' or enter
	log.Info("Confirming send with 's'")
	if err := c.sendString("s\r"); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)

	// Check result
	screen, _ = c.readScreen(ctx, 2*time.Second)
	screenStr = string(screen)

	log.WithFields(log.Fields{
		"recipient": recipient,
		"subject":   subject,
		"screen":    screenStr,
	}).Info("PTT mail send result screen")

	if strings.Contains(screenStr, "信件已送出") || strings.Contains(screenStr, "順利寄出") {
		log.WithFields(log.Fields{
			"recipient": recipient,
			"subject":   subject,
		}).Info("PTT mail sent successfully")
		return nil
	}

	// Mail may not have been sent successfully
	log.WithFields(log.Fields{
		"recipient": recipient,
		"subject":   subject,
		"screen":    screenStr,
	}).Warn("PTT mail send result unclear")

	return ErrSendMailFailed
}

// sendString sends a string to PTT (converts to Big5)
func (c *PTTClient) sendString(s string) error {
	if c.conn == nil {
		return errors.New("connection is nil")
	}

	// Convert UTF-8 to Big5
	encoder := traditionalchinese.Big5.NewEncoder()
	big5Bytes, _, err := transform.Bytes(encoder, []byte(s))
	if err != nil {
		// Fallback to original bytes if encoding fails
		big5Bytes = []byte(s)
	}

	c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return c.conn.WriteMessage(websocket.BinaryMessage, big5Bytes)
}

// sendByte sends a single byte (for control characters)
func (c *PTTClient) sendByte(b byte) error {
	if c.conn == nil {
		return errors.New("connection is nil")
	}
	c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return c.conn.WriteMessage(websocket.BinaryMessage, []byte{b})
}

// readScreen reads screen data from PTT
// stopText: if non-empty, stop reading when this text is found (allows early return)
func (c *PTTClient) readScreen(_ context.Context, timeout time.Duration, stopText ...string) ([]byte, error) {
	if c.conn == nil {
		return nil, errors.New("connection is nil")
	}
	if c.connFailed {
		return nil, errors.New("connection already failed")
	}

	var screenData []byte
	deadline := time.Now().Add(timeout)
	readCount := 0
	errorCount := 0
	var lastError error
	stopReading := false

	// Read in a separate function to catch panic
	func() {
		defer func() {
			if r := recover(); r != nil {
				c.connFailed = true // Mark connection as failed
				log.WithFields(log.Fields{
					"panic":     r,
					"readCount": readCount,
					"dataLen":   len(screenData),
				}).Warn("readScreen panic recovered, marking connection as failed")
			}
		}()

		for time.Now().Before(deadline) && !stopReading {
			// Set deadline for each read operation
			c.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

			msgType, data, readErr := c.conn.ReadMessage()
			if readErr != nil {
				errorCount++
				lastError = readErr
				errStr := readErr.Error()

				// Timeout is expected, continue trying
				if strings.Contains(errStr, "i/o timeout") {
					// If we already have data and got timeout, we can stop
					if len(screenData) > 0 {
						return
					}
					if time.Now().Before(deadline) {
						continue
					}
					return
				}

				// Fatal errors
				if strings.Contains(errStr, "use of closed") ||
					strings.Contains(errStr, "connection reset") ||
					strings.Contains(errStr, "broken pipe") ||
					strings.Contains(errStr, "websocket") {
					log.WithError(readErr).Warn("WebSocket connection error, stopping read")
					c.connFailed = true
					return
				}

				// Other errors - continue trying until deadline
				if time.Now().Before(deadline) {
					continue
				}
				return
			}
			readCount++
			if readCount <= 5 {
				log.WithFields(log.Fields{
					"msgType":  msgType,
					"dataLen":  len(data),
					"rawBytes": fmt.Sprintf("%v", data[:min(50, len(data))]),
				}).Info("ReadMessage received data")
			}

			// Handle Telnet negotiation commands in the data
			c.handleTelnetNegotiation(data)

			screenData = append(screenData, data...)

			// Check if we should stop reading (found any stop text)
			if len(stopText) > 0 {
				// Decode current data to check for stop text
				decoder := traditionalchinese.Big5.NewDecoder()
				utf8Bytes, _, _ := transform.Bytes(decoder, screenData)
				utf8Str := string(utf8Bytes)
				for _, st := range stopText {
					if st != "" && strings.Contains(utf8Str, st) {
						log.WithField("stopText", st).Info("Found stop text, stopping read")
						stopReading = true
						break
					}
				}
			}
		}
	}()

	log.WithFields(log.Fields{
		"readCount":  readCount,
		"errorCount": errorCount,
		"totalBytes": len(screenData),
		"lastError":  fmt.Sprintf("%v", lastError),
	}).Info("readScreen finished")

	if len(screenData) == 0 {
		return nil, nil
	}

	// Convert Big5 to UTF-8
	decoder := traditionalchinese.Big5.NewDecoder()
	utf8Bytes, _, decodeErr := transform.Bytes(decoder, screenData)
	if decodeErr != nil {
		log.WithError(decodeErr).Warn("Big5 decode failed, returning raw data")
		return screenData, nil
	}

	log.WithField("utf8Len", len(utf8Bytes)).Info("Big5 decode success")
	return utf8Bytes, nil
}

// waitForScreen waits for specific text to appear on screen
func (c *PTTClient) waitForScreen(ctx context.Context, text string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if connection already failed
		if c.connFailed {
			return errors.New("connection already failed")
		}

		screen, err := c.readScreen(ctx, 1*time.Second)
		if err != nil {
			// If connection failed, stop retrying
			if c.connFailed ||
				strings.Contains(err.Error(), "connection") {
				return err
			}
			continue
		}

		if strings.Contains(string(screen), text) {
			return nil
		}
	}

	return ErrTimeout
}

// SendMail is a convenience function to send mail
func SendMail(username, password, recipient, subject, content string) error {
	client := NewPTTClient(username, password)
	return client.SendMail(recipient, subject, content)
}

// TestLogin tests if the PTT credentials are valid
func (c *PTTClient) TestLogin() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect to PTT
	if err := c.connect(ctx); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	defer c.close()

	// Read immediately and stop as soon as we see login prompt
	c.readScreen(ctx, 5*time.Second, "請輸入代號")

	// Login
	if err := c.login(ctx); err != nil {
		return err
	}

	return nil
}

// TestCredentials is a convenience function to test PTT credentials
func TestCredentials(username, password string) error {
	client := NewPTTClient(username, password)
	return client.TestLogin()
}
