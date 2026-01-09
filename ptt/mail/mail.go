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
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
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

// send sends a string to PTT (converts to Big5)
func (c *PTTClient) send(s string) error {
	if c.stdin == nil {
		return errors.New("stdin is nil")
	}

	// Convert UTF-8 to Big5
	encoder := traditionalchinese.Big5.NewEncoder()
	big5Bytes, _, err := transform.Bytes(encoder, []byte(s))
	if err != nil {
		// Fallback to original bytes if encoding fails
		big5Bytes = []byte(s)
	}

	_, err = c.stdin.Write(big5Bytes)
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
func (c *PTTClient) getScreen() string {
	c.screenLock.Lock()
	defer c.screenLock.Unlock()

	if c.screenBuf.Len() == 0 {
		return ""
	}

	// Convert Big5 to UTF-8
	decoder := traditionalchinese.Big5.NewDecoder()
	utf8Bytes, _, err := transform.Bytes(decoder, c.screenBuf.Bytes())
	if err != nil {
		return c.screenBuf.String()
	}
	return string(utf8Bytes)
}

// clearScreen clears the screen buffer
func (c *PTTClient) clearScreen() {
	c.screenLock.Lock()
	defer c.screenLock.Unlock()
	c.screenBuf.Reset()
}

// waitFor waits for any of the specified texts to appear on screen
func (c *PTTClient) waitFor(ctx context.Context, timeout time.Duration, texts ...string) (string, bool) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return c.getScreen(), false
		case <-ticker.C:
			screen := c.getScreen()
			for _, text := range texts {
				if text != "" && strings.Contains(screen, text) {
					return screen, true
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
	// PTT might show: 請輸入代號, 代號, or just the PTT ASCII art
	screen, found := c.waitFor(ctx, 10*time.Second, "請輸入代號", "代號", "PTT", "批踢踢")
	log.WithField("screen_preview", truncate(screen, 500)).Info("Initial screen")

	// Even if we don't find exact prompt, if we got any screen data, try to proceed
	if !found && screen == "" {
		return errors.New("no response from PTT server")
	}

	// If screen has content but no login prompt, wait a bit more and check again
	if !strings.Contains(screen, "請輸入代號") && !strings.Contains(screen, "代號") {
		log.Info("Login prompt not found yet, waiting more...")
		time.Sleep(2 * time.Second)
		screen = c.getScreen()
		log.WithField("screen_preview", truncate(screen, 500)).Info("Screen after additional wait")
	}

	// Send username regardless - PTT should be waiting for it
	log.WithField("username", c.username).Info("Sending username")
	if err := c.sendLine(c.username); err != nil {
		return err
	}

	// Wait for password prompt
	screen, found = c.waitFor(ctx, 5*time.Second, "請輸入您的密碼", "密碼")
	if !found {
		log.WithField("screen", truncate(screen, 300)).Warn("Password prompt not found")
	}

	// Send password
	log.Info("Sending password")
	if err := c.sendLine(c.password); err != nil {
		return err
	}

	// Handle post-login screens
	for i := range 20 {
		time.Sleep(300 * time.Millisecond)
		screen = c.getScreen()
		log.WithFields(log.Fields{
			"iteration":      i,
			"screen_preview": truncate(screen, 300),
		}).Debug("Login screen check")

		// Check for login failure
		if strings.Contains(screen, "密碼不對") || strings.Contains(screen, "錯誤") && strings.Contains(screen, "密碼") {
			return ErrLoginFailed
		}

		// Check if we reached main menu
		if strings.Contains(screen, "主功能表") || strings.Contains(screen, "【主選單】") {
			log.Info("Login successful, reached main menu")
			return nil
		}

		// Handle duplicate login
		if strings.Contains(screen, "您想刪除其他重複登入") || strings.Contains(screen, "重複登入") {
			log.Info("Handling duplicate login, sending 'n'")
			c.sendLine("n")
			continue
		}

		// Press any key to continue
		if strings.Contains(screen, "請按任意鍵繼續") || strings.Contains(screen, "按任意鍵") {
			log.Info("Pressing Enter to continue")
			c.send("\r")
			continue
		}

		// Handle error attempts deletion prompt
		if strings.Contains(screen, "您要刪除以上錯誤嘗試") {
			log.Info("Deleting error attempts, sending 'y'")
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

	screen, found := c.waitFor(ctx, 5*time.Second, "郵件選單", "我的信箱", "電子郵件", "站內寄信", "寄發新信")
	log.WithField("screen_preview", truncate(screen, 300)).Info("Screen after M")

	if !found {
		// Not in mail section, try again
		log.Warn("Not in mail section, trying Enter then M")
		c.send("\r")
		time.Sleep(200 * time.Millisecond)
		c.send("M")
		screen, _ = c.waitFor(ctx, 5*time.Second, "郵件選單", "我的信箱", "電子郵件")
		log.WithField("screen_preview", truncate(screen, 300)).Info("Screen after retry M")
	}

	// Step 2: Press 'S' to send mail
	log.Info("Pressing 'S' for Send mail")
	if err := c.send("S"); err != nil {
		return fmt.Errorf("failed to send S: %w", err)
	}

	screen, found = c.waitFor(ctx, 5*time.Second, "收信人", "收件人", "站內寄信", "請輸入收件人")
	log.WithField("screen_preview", truncate(screen, 300)).Info("Screen after S")

	if !found {
		log.WithField("screen", truncate(screen, 500)).Warn("Recipient prompt not found")
	}

	// Step 3: Enter recipient
	log.WithField("recipient", recipient).Info("Entering recipient")
	if err := c.sendLine(recipient); err != nil {
		return fmt.Errorf("failed to send recipient: %w", err)
	}

	screen, found = c.waitFor(ctx, 5*time.Second, "標題", "主旨", "主題", "Subject", "無此帳號", "找不到")
	log.WithField("screen_preview", truncate(screen, 300)).Info("Screen after recipient")

	// Check for user not found
	if strings.Contains(screen, "無此帳號") || strings.Contains(screen, "找不到") {
		return ErrUserNotFound
	}

	// Step 4: Enter subject
	log.WithField("subject", subject).Info("Entering subject")
	if err := c.sendLine(subject); err != nil {
		return fmt.Errorf("failed to send subject: %w", err)
	}

	// Wait for editor
	screen, _ = c.waitFor(ctx, 3*time.Second, "編輯", "Ctrl", "內文")
	log.WithField("screen_preview", truncate(screen, 300)).Info("Screen after subject")

	// Step 5: Enter content (line by line)
	log.WithField("content_lines", len(strings.Split(content, "\n"))).Info("Entering mail content")
	// Convert content to CRLF for PTT
	contentWithCRLF := strings.ReplaceAll(content, "\n", "\r\n")
	if err := c.send(contentWithCRLF); err != nil {
		return fmt.Errorf("failed to send content: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Step 6: Press Ctrl+X to finish editing
	log.Info("Sending Ctrl+X to finish editing")
	if err := c.sendByte(0x18); err != nil {
		return fmt.Errorf("failed to send Ctrl+X: %w", err)
	}

	screen, _ = c.waitFor(ctx, 3*time.Second, "檔案處理", "存檔", "儲存", "(S)")
	log.WithField("screen_preview", truncate(screen, 300)).Info("Screen after Ctrl+X")

	// Step 7: Press 's' to save and send
	log.Info("Pressing 's' to save")
	if err := c.sendLine("s"); err != nil {
		return fmt.Errorf("failed to send s: %w", err)
	}

	// Wait for signature prompt or draft prompt
	screen, _ = c.waitFor(ctx, 3*time.Second, "簽名檔", "存底", "底稿", "自存底稿", "signature")
	log.WithField("screen_preview", truncate(screen, 300)).Info("Screen after save")

	// Handle signature prompt - select "0" for no signature
	if strings.Contains(screen, "簽名檔") || strings.Contains(screen, "signature") {
		log.Info("Selecting no signature (0)")
		if err := c.sendLine("0"); err != nil {
			return fmt.Errorf("failed to send 0: %w", err)
		}
		// Wait for draft prompt
		screen, _ = c.waitFor(ctx, 3*time.Second, "存底", "底稿", "自存底稿")
		log.WithField("screen_preview", truncate(screen, 300)).Info("Screen after signature selection")
	}

	// Step 8: Press 'n' to not save draft
	log.Info("Pressing 'n' to not save draft")
	if err := c.sendLine("n"); err != nil {
		return fmt.Errorf("failed to send n: %w", err)
	}

	// Wait for completion
	time.Sleep(500 * time.Millisecond)
	screen = c.getScreen()

	log.WithFields(log.Fields{
		"recipient":      recipient,
		"subject":        subject,
		"screen_preview": truncate(screen, 300),
	}).Info("PTT mail send completed")

	// Check for success indicators
	if strings.Contains(screen, "信件已送出") ||
		strings.Contains(screen, "順利寄出") ||
		strings.Contains(screen, "寄出") ||
		strings.Contains(screen, "成功") {
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
