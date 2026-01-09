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
	if err := c.connect(ctx); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	defer c.close()

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

	for {
		select {
		case <-c.stopRead:
			return
		default:
		}

		n, err := reader.Read(buf)
		if n > 0 {
			c.screenLock.Lock()
			c.screenBuf.Write(buf[:n])
			// Prevent unbounded growth
			if c.screenBuf.Len() > 50000 {
				tmp := c.screenBuf.Bytes()
				c.screenBuf.Reset()
				c.screenBuf.Write(tmp[len(tmp)-20000:])
			}
			c.screenLock.Unlock()
		}
		if err != nil {
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

// send sends a string to PTT (UTF-8)
func (c *PTTClient) send(s string) error {
	if c.stdin == nil {
		return errors.New("stdin is nil")
	}
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
func (c *PTTClient) getScreen() string {
	c.screenLock.Lock()
	defer c.screenLock.Unlock()

	if c.screenBuf.Len() == 0 {
		return ""
	}
	return c.screenBuf.String()
}

// clearScreen clears the screen buffer
func (c *PTTClient) clearScreen() {
	c.screenLock.Lock()
	defer c.screenLock.Unlock()
	c.screenBuf.Reset()
}

// stripANSI removes ANSI escape codes from string for easier text matching
func stripANSI(s string) string {
	result := make([]byte, 0, len(s))
	inEscape := false
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			inEscape = true
			continue
		}
		if inEscape {
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
	// Wait for PTT welcome screen
	screen, found := c.waitFor(ctx, 10*time.Second, "請輸入代號", "代號", "PTT", "批踢踢")

	if !found && screen == "" {
		return errors.New("no response from PTT server")
	}

	c.clearScreen()

	// Send username
	if err := c.sendLine(c.username); err != nil {
		return err
	}

	// Wait for password prompt
	c.waitFor(ctx, 5*time.Second, "請輸入您的密碼", "密碼")

	// Send password
	c.clearScreen()
	if err := c.sendLine(c.password); err != nil {
		return err
	}

	// Handle post-login screens
	for range 25 {
		time.Sleep(400 * time.Millisecond)
		screen = c.getScreen()
		strippedScreen := stripANSI(screen)

		// Check for login failure
		if strings.Contains(strippedScreen, "密碼不對") || (strings.Contains(strippedScreen, "錯誤") && strings.Contains(strippedScreen, "密碼")) {
			return ErrLoginFailed
		}

		// Check if we reached main menu
		if strings.Contains(strippedScreen, "主功能表") || strings.Contains(strippedScreen, "【主選單】") ||
			strings.Contains(strippedScreen, "主選單") {
			return nil
		}

		// Handle duplicate login
		if strings.Contains(strippedScreen, "您想刪除其他重複登入") || strings.Contains(strippedScreen, "重複登入") {
			c.clearScreen()
			c.sendLine("n")
			continue
		}

		// Press any key to continue
		if strings.Contains(strippedScreen, "請按任意鍵繼續") || strings.Contains(strippedScreen, "按任意鍵") {
			c.clearScreen()
			c.send("\r")
			continue
		}

		// Handle error attempts deletion prompt
		if strings.Contains(strippedScreen, "您要刪除以上錯誤嘗試") {
			c.clearScreen()
			c.sendLine("y")
			continue
		}
	}

	// Check final state
	screen = c.getScreen()
	if strings.Contains(screen, "主功能表") || strings.Contains(screen, "【主選單】") {
		return nil
	}

	return nil
}

// sendMailInternal sends mail after login
func (c *PTTClient) sendMailInternal(ctx context.Context, recipient, subject, content string) error {
	c.clearScreen()

	// Step 1: Press 'M' + Enter for Mail menu
	if err := c.send("M"); err != nil {
		return fmt.Errorf("failed to send M: %w", err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := c.send("\r"); err != nil {
		return fmt.Errorf("failed to send Enter after M: %w", err)
	}

	_, found := c.waitFor(ctx, 5*time.Second, "郵件選單", "我的信箱", "電子郵件", "站內寄信", "寄發新信")

	if !found {
		c.clearScreen()
		c.send("M")
		time.Sleep(200 * time.Millisecond)
		c.send("\r")
		c.waitFor(ctx, 5*time.Second, "郵件選單", "我的信箱", "電子郵件")
	}

	// Step 2: Press 'S' + Enter for Send mail
	c.clearScreen()
	if err := c.send("S"); err != nil {
		return fmt.Errorf("failed to send S: %w", err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := c.send("\r"); err != nil {
		return fmt.Errorf("failed to send Enter after S: %w", err)
	}

	c.waitFor(ctx, 5*time.Second, "收信人", "收件人", "站內寄信", "請輸入收件人")

	// Step 3: Enter recipient
	c.clearScreen()
	if err := c.sendLine(recipient); err != nil {
		return fmt.Errorf("failed to send recipient: %w", err)
	}

	screen, _ := c.waitFor(ctx, 5*time.Second, "標題", "主旨", "主題", "Subject", "無此帳號", "找不到")
	stripped := stripANSI(screen)

	if strings.Contains(stripped, "無此帳號") || strings.Contains(stripped, "找不到") {
		return ErrUserNotFound
	}

	// Step 4: Enter subject
	c.clearScreen()
	if err := c.sendLine(subject); err != nil {
		return fmt.Errorf("failed to send subject: %w", err)
	}

	c.waitFor(ctx, 3*time.Second, "編輯", "Ctrl", "內文")

	// Step 5: Enter content
	c.clearScreen()
	if err := c.send(content); err != nil {
		return fmt.Errorf("failed to send content: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Step 6: Press Ctrl+X to finish editing
	if err := c.sendByte(0x18); err != nil {
		return fmt.Errorf("failed to send Ctrl+X: %w", err)
	}

	c.waitFor(ctx, 3*time.Second, "檔案處理", "存檔", "儲存", "(S)")

	// Step 7: Press Enter to save/send
	c.clearScreen()
	if err := c.send("\r"); err != nil {
		return fmt.Errorf("failed to send Enter: %w", err)
	}

	c.waitFor(ctx, 3*time.Second, "簽名檔", "選擇簽名檔")

	// Step 8: Select '0' for no signature
	c.clearScreen()
	if err := c.send("0"); err != nil {
		return fmt.Errorf("failed to send 0: %w", err)
	}

	c.waitFor(ctx, 3*time.Second, "存底", "底稿", "自存底稿", "是否")

	// Step 9: Press 'n' + Enter to not save draft
	c.clearScreen()
	if err := c.sendLine("n"); err != nil {
		return fmt.Errorf("failed to send n: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	log.WithFields(log.Fields{
		"recipient": recipient,
		"subject":   subject,
	}).Info("PTT mail sent")

	return nil
}

// TestLogin tests if the PTT credentials are valid
func (c *PTTClient) TestLogin() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.connect(ctx); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	defer c.close()

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
