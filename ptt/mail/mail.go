package mail

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"golang.org/x/crypto/ssh"
)

const (
	pttHost        = "ptt.cc:22"
	pttSSHUser     = "bbsu"
	connectTimeout = 15 * time.Second
)

var (
	ErrLoginFailed    = errors.New("PTT login failed")
	ErrSendMailFailed = errors.New("failed to send PTT mail")
	ErrTimeout        = errors.New("operation timeout")
	ErrUserNotFound   = errors.New("recipient user not found")
)

// PTTClient represents a PTT SSH client
type PTTClient struct {
	username   string
	password   string
	client     *ssh.Client
	session    *ssh.Session
	stdin      io.WriteCloser
	stdout     io.Reader
	screenBuf  bytes.Buffer
	screenLock sync.Mutex
	stopRead   chan struct{}
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
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Connect to PTT via SSH
	log.Info("Connecting to PTT via SSH...")
	if err := c.connect(ctx); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	defer c.close()
	log.Info("PTT SSH connected")

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

// connect establishes SSH connection to PTT
func (c *PTTClient) connect(_ context.Context) error {
	cfg := &ssh.ClientConfig{
		User: pttSSHUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(c.password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         connectTimeout,
	}

	client, err := ssh.Dial("tcp", pttHost, cfg)
	if err != nil {
		return fmt.Errorf("ssh dial failed: %w", err)
	}
	c.client = client

	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		return fmt.Errorf("new session failed: %w", err)
	}
	c.session = sess

	// Request PTY (critical for PTT UI)
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm", 40, 120, modes); err != nil {
		sess.Close()
		client.Close()
		return fmt.Errorf("request pty failed: %w", err)
	}

	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return fmt.Errorf("stdin pipe failed: %w", err)
	}
	c.stdin = stdin

	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return fmt.Errorf("stdout pipe failed: %w", err)
	}
	c.stdout = stdout

	if err := sess.Shell(); err != nil {
		sess.Close()
		client.Close()
		return fmt.Errorf("start shell failed: %w", err)
	}

	// Start background reader
	c.stopRead = make(chan struct{})
	go c.readLoop()

	return nil
}

// readLoop continuously reads from stdout and accumulates in buffer
func (c *PTTClient) readLoop() {
	reader := bufio.NewReader(c.stdout)
	buf := make([]byte, 4096)
	totalRead := 0

	for {
		select {
		case <-c.stopRead:
			log.WithField("total_bytes", totalRead).Debug("readLoop stopped")
			return
		default:
		}

		n, err := reader.Read(buf)
		if n > 0 {
			totalRead += n
			c.screenLock.Lock()
			c.screenBuf.Write(buf[:n])
			bufLen := c.screenBuf.Len()
			// Prevent unbounded growth
			if bufLen > 50000 {
				tmp := c.screenBuf.Bytes()
				c.screenBuf.Reset()
				c.screenBuf.Write(tmp[len(tmp)-20000:])
			}
			c.screenLock.Unlock()

			// Log first few reads for debugging
			if totalRead <= 10000 {
				log.WithFields(log.Fields{
					"bytes_read":  n,
					"total_read":  totalRead,
					"buffer_size": bufLen,
				}).Debug("readLoop received data")
			}
		}
		if err != nil {
			if err != io.EOF {
				log.WithError(err).Debug("readLoop error")
			}
			return
		}
	}
}

// close closes the SSH connection
func (c *PTTClient) close() {
	if c.stopRead != nil {
		close(c.stopRead)
	}
	if c.session != nil {
		c.session.Close()
	}
	if c.client != nil {
		c.client.Close()
	}
}

// send sends a string to PTT
// Note: When using SSH user "bbsu", PTT expects UTF-8, no conversion needed
func (c *PTTClient) send(s string) error {
	if c.stdin == nil {
		return errors.New("stdin is nil")
	}

	// bbsu user expects UTF-8, send directly
	_, err := c.stdin.Write([]byte(s))
	return err
}

// sendLine sends a string followed by Enter
func (c *PTTClient) sendLine(s string) error {
	return c.send(s + "\r")
}

// sendByte sends a single byte (for control characters)
func (c *PTTClient) sendByte(b byte) error {
	if c.stdin == nil {
		return errors.New("stdin is nil")
	}
	_, err := c.stdin.Write([]byte{b})
	return err
}

// getScreen returns current screen content as UTF-8 string
// Note: When using SSH user "bbsu", PTT sends UTF-8 encoded text, no conversion needed
func (c *PTTClient) getScreen() string {
	c.screenLock.Lock()
	defer c.screenLock.Unlock()

	if c.screenBuf.Len() == 0 {
		return ""
	}

	// bbsu user sends UTF-8, no need to decode Big5
	return c.screenBuf.String()
}

// getScreenRaw returns current screen content as raw bytes for debugging
func (c *PTTClient) getScreenRaw() []byte {
	c.screenLock.Lock()
	defer c.screenLock.Unlock()
	result := make([]byte, c.screenBuf.Len())
	copy(result, c.screenBuf.Bytes())
	return result
}

// clearScreen clears the screen buffer
func (c *PTTClient) clearScreen() {
	c.screenLock.Lock()
	defer c.screenLock.Unlock()
	c.screenBuf.Reset()
}

// stripANSI removes ANSI escape codes from string for easier text matching
func stripANSI(s string) string {
	// Simple ANSI escape code stripper
	result := make([]byte, 0, len(s))
	inEscape := false
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b { // ESC
			inEscape = true
			continue
		}
		if inEscape {
			// End of escape sequence
			if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
				inEscape = false
			}
			continue
		}
		result = append(result, s[i])
	}
	return string(result)
}

// waitFor waits for any of the specified texts to appear on screen
func (c *PTTClient) waitFor(ctx context.Context, timeout time.Duration, texts ...string) (string, bool) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return c.getScreen(), false
		case <-ticker.C:
			screen := c.getScreen()
			// Try matching with both raw and ANSI-stripped versions
			strippedScreen := stripANSI(screen)
			for _, text := range texts {
				if text != "" {
					if strings.Contains(screen, text) || strings.Contains(strippedScreen, text) {
						return screen, true
					}
				}
			}
		}
	}
	return c.getScreen(), false
}

// login performs PTT login
func (c *PTTClient) login(ctx context.Context) error {
	log.Info("Starting PTT login")

	// Wait for PTT welcome screen - look for various indicators
	screen, found := c.waitFor(ctx, 10*time.Second, "請輸入代號", "代號", "PTT", "批踢踢")

	// Log both raw and stripped versions for debugging
	strippedScreen := stripANSI(screen)
	log.WithFields(log.Fields{
		"screen_preview":  truncate(screen, 300),
		"stripped_screen": truncate(strippedScreen, 300),
		"buffer_size":     len(c.getScreenRaw()),
	}).Info("Initial screen")

	// Even if we don't find exact prompt, if we got any screen data, try to proceed
	if !found && screen == "" {
		return errors.New("no response from PTT server")
	}

	// Clear buffer before sending username to get clean response
	c.clearScreen()

	// Send username
	log.WithField("username", c.username).Info("Sending username")
	if err := c.sendLine(c.username); err != nil {
		return err
	}

	// Wait for password prompt
	screen, found = c.waitFor(ctx, 5*time.Second, "請輸入您的密碼", "密碼")
	strippedScreen = stripANSI(screen)
	log.WithFields(log.Fields{
		"found":           found,
		"stripped_screen": truncate(strippedScreen, 200),
	}).Info("After sending username")

	if !found {
		// Check stripped version
		if !strings.Contains(strippedScreen, "密碼") {
			log.Warn("Password prompt not found")
		}
	}

	// Clear buffer and send password
	c.clearScreen()
	log.Info("Sending password")
	if err := c.sendLine(c.password); err != nil {
		return err
	}

	// Handle post-login screens
	for i := range 25 {
		time.Sleep(400 * time.Millisecond)
		screen = c.getScreen()
		strippedScreen = stripANSI(screen)

		if i < 5 || i%5 == 0 {
			log.WithFields(log.Fields{
				"iteration":       i,
				"stripped_screen": truncate(strippedScreen, 200),
			}).Info("Login screen check")
		}

		// Check for login failure
		if strings.Contains(strippedScreen, "密碼不對") || (strings.Contains(strippedScreen, "錯誤") && strings.Contains(strippedScreen, "密碼")) {
			return ErrLoginFailed
		}

		// Check if we reached main menu
		if strings.Contains(strippedScreen, "主功能表") || strings.Contains(strippedScreen, "【主選單】") ||
			strings.Contains(strippedScreen, "主選單") {
			log.Info("Login successful, reached main menu")
			return nil
		}

		// Handle duplicate login
		if strings.Contains(strippedScreen, "您想刪除其他重複登入") || strings.Contains(strippedScreen, "重複登入") {
			log.Info("Handling duplicate login, sending 'n'")
			c.clearScreen()
			c.sendLine("n")
			continue
		}

		// Press any key to continue
		if strings.Contains(strippedScreen, "請按任意鍵繼續") || strings.Contains(strippedScreen, "按任意鍵") {
			log.Info("Pressing Enter to continue")
			c.clearScreen()
			c.send("\r")
			continue
		}

		// Handle error attempts deletion prompt
		if strings.Contains(strippedScreen, "您要刪除以上錯誤嘗試") {
			log.Info("Deleting error attempts, sending 'y'")
			c.clearScreen()
			c.sendLine("y")
			continue
		}

		// Still logging in
		if strings.Contains(screen, "登入中") || strings.Contains(screen, "請稍候") {
			log.Debug("Login in progress...")
			continue
		}
	}

	// Check final state
	screen = c.getScreen()
	if strings.Contains(screen, "主功能表") || strings.Contains(screen, "【主選單】") {
		log.Info("Login successful")
		return nil
	}

	log.WithField("screen", truncate(screen, 500)).Warn("Login flow completed but main menu not detected")
	return nil
}

// sendMailInternal sends mail after login
func (c *PTTClient) sendMailInternal(ctx context.Context, recipient, subject, content string) error {
	log.WithField("recipient", recipient).Info("Starting mail send process")

	// Clear screen buffer for fresh reads
	c.clearScreen()

	// Step 1: Press 'M' for Mail menu
	log.Info("Pressing 'M' for Mail menu")
	if err := c.send("M"); err != nil {
		return fmt.Errorf("failed to send M: %w", err)
	}

	screen, found := c.waitFor(ctx, 5*time.Second, "郵件選單", "我的信箱", "電子郵件", "站內寄信", "寄發新信", "Mail")
	stripped := stripANSI(screen)
	log.WithField("stripped_screen", truncate(stripped, 300)).Info("Screen after M")

	if !found {
		// Not in mail section, try again
		log.Warn("Not in mail section, trying Enter then M")
		c.clearScreen()
		c.send("\r")
		time.Sleep(300 * time.Millisecond)
		c.send("M")
		screen, _ = c.waitFor(ctx, 5*time.Second, "郵件選單", "我的信箱", "電子郵件", "Mail")
		stripped = stripANSI(screen)
		log.WithField("stripped_screen", truncate(stripped, 300)).Info("Screen after retry M")
	}

	// Step 2: Press 'S' to send mail
	c.clearScreen()
	log.Info("Pressing 'S' for Send mail")
	if err := c.send("S"); err != nil {
		return fmt.Errorf("failed to send S: %w", err)
	}

	screen, found = c.waitFor(ctx, 5*time.Second, "收信人", "收件人", "站內寄信", "請輸入收件人")
	stripped = stripANSI(screen)
	log.WithField("stripped_screen", truncate(stripped, 300)).Info("Screen after S")

	if !found {
		log.Warn("Recipient prompt not found")
	}

	// Step 3: Enter recipient
	c.clearScreen()
	log.WithField("recipient", recipient).Info("Entering recipient")
	if err := c.sendLine(recipient); err != nil {
		return fmt.Errorf("failed to send recipient: %w", err)
	}

	screen, _ = c.waitFor(ctx, 5*time.Second, "標題", "主旨", "主題", "Subject", "無此帳號", "找不到")
	stripped = stripANSI(screen)
	log.WithField("stripped_screen", truncate(stripped, 300)).Info("Screen after recipient")

	// Check for user not found
	if strings.Contains(stripped, "無此帳號") || strings.Contains(stripped, "找不到") {
		return ErrUserNotFound
	}

	// Step 4: Enter subject
	c.clearScreen()
	log.WithField("subject", subject).Info("Entering subject")
	if err := c.sendLine(subject); err != nil {
		return fmt.Errorf("failed to send subject: %w", err)
	}

	// Wait for editor
	screen, _ = c.waitFor(ctx, 3*time.Second, "編輯", "Ctrl", "內文")
	stripped = stripANSI(screen)
	log.WithField("stripped_screen", truncate(stripped, 200)).Info("Screen after subject")

	// Step 5: Enter content
	c.clearScreen()
	log.WithField("content_lines", len(strings.Split(content, "\n"))).Info("Entering mail content")
	// Convert content to CRLF for PTT
	contentWithCRLF := strings.ReplaceAll(content, "\n", "\r\n")
	if err := c.send(contentWithCRLF); err != nil {
		return fmt.Errorf("failed to send content: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Step 6: Press Ctrl+X to finish editing
	log.Info("Sending Ctrl+X to finish editing")
	if err := c.sendByte(0x18); err != nil {
		return fmt.Errorf("failed to send Ctrl+X: %w", err)
	}

	screen, _ = c.waitFor(ctx, 3*time.Second, "檔案處理", "存檔", "儲存", "(S)")
	stripped = stripANSI(screen)
	log.WithField("stripped_screen", truncate(stripped, 200)).Info("Screen after Ctrl+X")

	// Step 7: Press 's' to save and send
	c.clearScreen()
	log.Info("Pressing 's' to save")
	if err := c.sendLine("s"); err != nil {
		return fmt.Errorf("failed to send s: %w", err)
	}

	// Wait for signature prompt or draft prompt
	screen, _ = c.waitFor(ctx, 3*time.Second, "簽名檔", "存底", "底稿", "自存底稿", "signature")
	stripped = stripANSI(screen)
	log.WithField("stripped_screen", truncate(stripped, 200)).Info("Screen after save")

	// Handle signature prompt - select "0" for no signature
	if strings.Contains(stripped, "簽名檔") || strings.Contains(stripped, "signature") {
		log.Info("Selecting no signature (0)")
		c.clearScreen()
		if err := c.sendLine("0"); err != nil {
			return fmt.Errorf("failed to send 0: %w", err)
		}
		// Wait for draft prompt
		screen, _ = c.waitFor(ctx, 3*time.Second, "存底", "底稿", "自存底稿")
		stripped = stripANSI(screen)
		log.WithField("stripped_screen", truncate(stripped, 200)).Info("Screen after signature selection")
	}

	// Step 8: Press 'n' to not save draft
	c.clearScreen()
	log.Info("Pressing 'n' to not save draft")
	if err := c.sendLine("n"); err != nil {
		return fmt.Errorf("failed to send n: %w", err)
	}

	// Wait for completion
	time.Sleep(500 * time.Millisecond)
	screen = c.getScreen()
	stripped = stripANSI(screen)

	log.WithFields(log.Fields{
		"recipient":       recipient,
		"subject":         subject,
		"stripped_screen": truncate(stripped, 300),
	}).Info("PTT mail send completed")

	// Check for success indicators
	if strings.Contains(stripped, "信件已送出") ||
		strings.Contains(stripped, "順利寄出") ||
		strings.Contains(stripped, "寄出") ||
		strings.Contains(stripped, "成功") {
		log.Info("PTT mail sent successfully (confirmed)")
	} else {
		log.Info("PTT mail send completed (no explicit confirmation)")
	}

	return nil
}

// TestLogin tests if the PTT credentials are valid
func (c *PTTClient) TestLogin() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect to PTT via SSH
	if err := c.connect(ctx); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	defer c.close()

	// Login
	if err := c.login(ctx); err != nil {
		return err
	}

	return nil
}

// SendMail is a convenience function to send mail
func SendMail(username, password, recipient, subject, content string) error {
	client := NewPTTClient(username, password)
	return client.SendMail(recipient, subject, content)
}

// TestCredentials is a convenience function to test PTT credentials
func TestCredentials(username, password string) error {
	client := NewPTTClient(username, password)
	return client.TestLogin()
}

// truncate truncates a string to max length
func truncate(s string, maxLen int) string {
	// Remove ANSI escape codes for cleaner logging
	s = strings.ReplaceAll(s, "\x1b", "\\x1b")
	// Remove other control characters
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
