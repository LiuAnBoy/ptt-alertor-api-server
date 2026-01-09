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
	conn     *websocket.Conn
	username string
	password string
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
	if err := c.connect(ctx); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	defer c.close()

	// Wait for initial screen
	time.Sleep(1 * time.Second)

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

// close closes the WebSocket connection
func (c *PTTClient) close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// login performs PTT login
func (c *PTTClient) login(ctx context.Context) error {
	log.Info("Starting PTT login")

	// Read until we see login prompt
	if err := c.waitForScreen(ctx, "請輸入代號", 10*time.Second); err != nil {
		log.WithField("error", err).Info("Waiting for login screen...")
	}

	// Send username
	log.WithField("username", c.username).Info("Sending username")
	if err := c.sendString(c.username + "\r"); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	// Wait for password prompt
	if err := c.waitForScreen(ctx, "請輸入您的密碼", 5*time.Second); err != nil {
		log.WithField("error", err).Info("Waiting for password screen...")
	}

	// Send password
	log.Info("Sending password")
	if err := c.sendString(c.password + "\r"); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)

	// Handle post-login screens (press any key, etc.)
	for i := 0; i < 5; i++ {
		screen, _ := c.readScreen(ctx, 2*time.Second)
		screenStr := string(screen)
		log.WithFields(log.Fields{
			"iteration": i,
			"screen":    screenStr,
		}).Info("Post-login screen")

		// Check for login failure
		if strings.Contains(screenStr, "密碼不對") || strings.Contains(screenStr, "錯誤") {
			return ErrLoginFailed
		}

		// Check for duplicate login
		if strings.Contains(screenStr, "您想刪除其他重複登入") {
			log.Info("Handling duplicate login prompt")
			c.sendString("n\r") // Don't kick other login
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Press enter/space to continue through various prompts
		if strings.Contains(screenStr, "請按任意鍵繼續") ||
			strings.Contains(screenStr, "按任意鍵") ||
			strings.Contains(screenStr, "您要刪除以上錯誤嘗試") {
			log.Info("Pressing space to continue")
			c.sendString(" ")
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Check if we reached main menu
		if strings.Contains(screenStr, "主功能表") {
			log.Info("Login successful, reached main menu")
			return nil
		}
	}

	log.Warn("Login finished but main menu not detected")
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
func (c *PTTClient) readScreen(ctx context.Context, timeout time.Duration) (result []byte, err error) {
	// Recover from panic (gorilla/websocket panics on failed connection read)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("websocket panic: %v", r)
			result = nil
		}
	}()

	if c.conn == nil {
		return nil, errors.New("connection is nil")
	}

	c.conn.SetReadDeadline(time.Now().Add(timeout))

	var screenData []byte
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		_, data, readErr := c.conn.ReadMessage()
		if readErr != nil {
			// Any error means we should stop reading
			break
		}
		screenData = append(screenData, data...)
	}

	// Convert Big5 to UTF-8
	decoder := traditionalchinese.Big5.NewDecoder()
	utf8Bytes, _, decodeErr := transform.Bytes(decoder, screenData)
	if decodeErr != nil {
		return screenData, nil
	}

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

		screen, err := c.readScreen(ctx, 1*time.Second)
		if err != nil {
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

	// Wait for initial screen
	time.Sleep(1 * time.Second)

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
