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
	readCh     chan []byte // Channel for receiving data from read goroutine
	closeCh    chan struct{} // Channel to signal read goroutine to stop
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
	c.readCh = make(chan []byte, 100) // Buffer for incoming data
	c.closeCh = make(chan struct{})

	// Start a single goroutine to read from websocket
	// This prevents concurrent reads which cause gorilla/websocket to panic
	go c.readLoop()

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
	// Signal read goroutine to stop
	if c.closeCh != nil {
		close(c.closeCh)
	}
	if c.conn != nil {
		c.conn.Close()
	}
}


// readLoop continuously reads from websocket and sends data to channel
// This ensures only one goroutine reads from the connection at a time
func (c *PTTClient) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.WithField("panic", r).Warn("readLoop panic recovered")
			c.connFailed = true
		}
	}()

	for {
		select {
		case <-c.closeCh:
			return
		default:
		}

		_, data, err := c.conn.ReadMessage()
		if err != nil {
			errStr := err.Error()
			// Check for fatal errors
			if strings.Contains(errStr, "use of closed") ||
				strings.Contains(errStr, "connection reset") ||
				strings.Contains(errStr, "broken pipe") ||
				strings.Contains(errStr, "EOF") {
				log.WithError(err).Warn("readLoop: connection error")
				c.connFailed = true
				return
			}
			// For other errors, continue
			continue
		}

		// Send data to channel (non-blocking)
		select {
		case c.readCh <- data:
		default:
			// Channel full, drop data (shouldn't happen with buffer)
			log.Warn("readLoop: channel full, dropping data")
		}
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

		// Press space to continue - CHECK THIS FIRST (before "登入中")
		// Because screen might contain both "登入中" and "按任意鍵"
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

		// Handle duplicate login IMMEDIATELY - this is time-sensitive
		if strings.Contains(screenStr, "您想刪除其他重複登入") || strings.Contains(screenStr, "重複登入") {
			log.Info("Handling duplicate login prompt, sending N")
			if err := c.sendString("n\r"); err != nil {
				log.WithError(err).Warn("Failed to send N for duplicate login")
			}
			continue
		}

		// "密碼正確" without "按任意鍵" - wait for next screen
		if strings.Contains(screenStr, "密碼正確") {
			log.Info("Password correct, waiting for next screen...")
			continue
		}

		// "登入中，請稍候" without "按任意鍵" - just wait
		if strings.Contains(screenStr, "登入中") || strings.Contains(screenStr, "請稍候") {
			log.Info("Login in progress, waiting...")
			continue
		}

		// Don't send anything for unknown screens - just wait
		log.WithField("screen", screenStr).Info("Unknown screen, waiting...")
	}

	// Even if main menu not detected, if we're not at login screen, consider it successful
	log.Info("Login flow completed")
	return nil
}

// sendMailInternal sends mail after login
func (c *PTTClient) sendMailInternal(ctx context.Context, recipient, subject, content string) error {
	log.WithField("recipient", recipient).Info("Starting mail send process")

	if c.connFailed {
		return errors.New("connection already failed before mail send")
	}

	// Step 1: Press 'm' to enter mail section
	log.Info("Pressing 'm' for Mail menu")
	if err := c.sendString("m"); err != nil {
		return fmt.Errorf("failed to send m: %w", err)
	}

	// Wait for mail menu
	screen, _ := c.readScreen(ctx, 5*time.Second, "郵件選單", "信箱", "寄信", "讀信")
	screenStr := string(screen)
	log.WithField("screen", screenStr).Info("Screen after pressing m")

	if c.connFailed {
		return errors.New("connection failed after pressing m")
	}

	// Step 2: Press 's' to enter send mail
	log.Info("Pressing 's' for Send mail")
	if err := c.sendString("s"); err != nil {
		return fmt.Errorf("failed to send s: %w", err)
	}

	// Wait for recipient prompt
	screen, _ = c.readScreen(ctx, 5*time.Second, "收信人", "代號", "帳號")
	screenStr = string(screen)
	log.WithField("screen", screenStr).Info("Screen after pressing s")

	if c.connFailed {
		return errors.New("connection failed after pressing s")
	}

	// Step 3: Enter recipient and press Enter
	log.WithField("recipient", recipient).Info("Entering recipient")
	if err := c.sendString(recipient + "\r"); err != nil {
		return fmt.Errorf("failed to send recipient: %w", err)
	}

	// Wait for subject prompt or error
	screen, _ = c.readScreen(ctx, 5*time.Second, "標題", "主題", "無此帳號", "找不到")
	screenStr = string(screen)
	log.WithField("screen", screenStr).Info("Screen after entering recipient")

	if strings.Contains(screenStr, "無此帳號") ||
		strings.Contains(screenStr, "找不到這位使用者") ||
		strings.Contains(screenStr, "找不到") {
		return ErrUserNotFound
	}

	if c.connFailed {
		return errors.New("connection failed after entering recipient")
	}

	// Step 4: Enter subject and press Enter
	log.WithField("subject", subject).Info("Entering subject")
	if err := c.sendString(subject + "\r"); err != nil {
		return fmt.Errorf("failed to send subject: %w", err)
	}

	// Wait for content editor
	screen, _ = c.readScreen(ctx, 3*time.Second, "編輯", "內文", "Ctrl")
	log.WithField("screen", string(screen)).Info("Screen after entering subject")

	if c.connFailed {
		return errors.New("connection failed after entering subject")
	}

	// Step 5: Enter content line by line
	log.WithField("content_lines", len(strings.Split(content, "\n"))).Info("Entering mail content")
	for _, line := range strings.Split(content, "\n") {
		if err := c.sendString(line + "\r"); err != nil {
			return fmt.Errorf("failed to send content line: %w", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Step 6: Press Ctrl+X to finish editing
	log.Info("Sending Ctrl+X to finish editing")
	if err := c.sendByte(0x18); err != nil {
		return fmt.Errorf("failed to send Ctrl+X: %w", err)
	}

	// Wait for file handling prompt
	screen, _ = c.readScreen(ctx, 3*time.Second, "檔案處理", "存檔", "儲存")
	log.WithField("screen", string(screen)).Info("Screen after Ctrl+X")

	// Step 7: Press Enter to confirm
	log.Info("Pressing Enter to confirm")
	if err := c.sendString("\r"); err != nil {
		return fmt.Errorf("failed to send Enter: %w", err)
	}

	// Wait for save draft prompt
	screen, _ = c.readScreen(ctx, 3*time.Second, "存底", "底稿", "自存底稿")
	log.WithField("screen", string(screen)).Info("Screen after Enter")

	// Step 8: Press 'N' to not save draft
	log.Info("Pressing 'N' to not save draft")
	if err := c.sendString("N"); err != nil {
		return fmt.Errorf("failed to send N: %w", err)
	}

	// Check result
	screen, _ = c.readScreen(ctx, 5*time.Second, "信件已", "寄出", "完成", "成功")
	screenStr = string(screen)

	log.WithFields(log.Fields{
		"recipient": recipient,
		"subject":   subject,
		"screen":    screenStr,
	}).Info("PTT mail send result screen")

	if strings.Contains(screenStr, "信件已送出") ||
		strings.Contains(screenStr, "順利寄出") ||
		strings.Contains(screenStr, "寄出") ||
		strings.Contains(screenStr, "成功") {
		log.WithFields(log.Fields{
			"recipient": recipient,
			"subject":   subject,
		}).Info("PTT mail sent successfully")
		return nil
	}

	// Even if we don't see success message, it might have worked
	log.WithFields(log.Fields{
		"recipient": recipient,
		"subject":   subject,
		"screen":    screenStr,
	}).Warn("PTT mail send result unclear, assuming success")

	return nil
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
	stopReading := false

	for time.Now().Before(deadline) && !stopReading && !c.connFailed {
		// Wait for data from channel or timeout
		select {
		case data := <-c.readCh:
			readCount++
			if readCount <= 5 {
				log.WithFields(log.Fields{
					"dataLen":  len(data),
					"rawBytes": fmt.Sprintf("%v", data[:min(50, len(data))]),
				}).Info("ReadMessage received data")
			}

			// Handle Telnet negotiation commands in the data
			c.handleTelnetNegotiation(data)

			screenData = append(screenData, data...)

			// Check if we should stop reading (found any stop text)
			if len(stopText) > 0 {
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

		case <-time.After(500 * time.Millisecond):
			// Timeout waiting for this chunk
			// If we have data, we can return it
			if len(screenData) > 0 {
				stopReading = true
			}
			// Otherwise continue trying until overall deadline
		}
	}

	log.WithFields(log.Fields{
		"readCount":  readCount,
		"totalBytes": len(screenData),
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
