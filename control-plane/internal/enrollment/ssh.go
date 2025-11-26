package enrollment

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHClient provides SSH operations for enrollment.
type SSHClient struct {
	client *ssh.Client
	host   string
}

// SSHConfig holds SSH connection configuration.
type SSHConfig struct {
	Host     string
	Port     int
	Username string
	// One of these must be provided
	Password   string
	PrivateKey []byte
	// Connection settings
	Timeout time.Duration
}

// ConnectSSH establishes an SSH connection.
func ConnectSSH(ctx context.Context, cfg SSHConfig) (*SSHClient, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Port == 0 {
		cfg.Port = 22
	}

	// Build auth methods
	var authMethods []ssh.AuthMethod
	if cfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(cfg.Password))
	}
	if len(cfg.PrivateKey) > 0 {
		signer, err := ssh.ParsePrivateKey(cfg.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("parsing private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication method provided")
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: implement proper host key verification
		Timeout:         cfg.Timeout,
	}

	address := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	// Use context for timeout
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", address, err)
	}

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, address, sshConfig)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("SSH handshake failed: %w", err)
	}

	client := ssh.NewClient(sshConn, chans, reqs)

	return &SSHClient{
		client: client,
		host:   cfg.Host,
	}, nil
}

// Close closes the SSH connection.
func (c *SSHClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// Run executes a command and returns the output.
func (c *SSHClient) Run(ctx context.Context, cmd string) (string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	// Capture output
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// Create a channel to signal completion
	done := make(chan error, 1)
	go func() {
		done <- session.Run(cmd)
	}()

	// Wait for completion or context cancellation
	select {
	case err := <-done:
		if err != nil {
			// Include stderr in error message
			if stderr.Len() > 0 {
				return stdout.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
			}
			return stdout.String(), err
		}
		return stdout.String(), nil
	case <-ctx.Done():
		session.Signal(ssh.SIGTERM)
		return "", ctx.Err()
	}
}

// RunWithSudo executes a command with sudo.
func (c *SSHClient) RunWithSudo(ctx context.Context, cmd string) (string, error) {
	// Use sudo with -n (non-interactive) flag first to check if password is needed
	sudoCmd := fmt.Sprintf("sudo -n %s", cmd)
	output, err := c.Run(ctx, sudoCmd)
	if err != nil {
		// If sudo requires password, the enrollment process should have already
		// established passwordless sudo, or we need to use a different approach
		return output, fmt.Errorf("sudo failed (passwordless sudo may not be configured): %w", err)
	}
	return output, nil
}

// RunScript executes a multi-line script.
func (c *SSHClient) RunScript(ctx context.Context, script string) (string, error) {
	// Use bash -c with heredoc style
	return c.Run(ctx, fmt.Sprintf("bash -c '%s'", escapeForBash(script)))
}

// WriteFile writes content to a file on the remote host.
func (c *SSHClient) WriteFile(ctx context.Context, path string, content []byte, mode string) error {
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	// Use cat with heredoc to write file
	cmd := fmt.Sprintf("cat > %s && chmod %s %s", path, mode, path)

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("getting stdin pipe: %w", err)
	}

	var stderr bytes.Buffer
	session.Stderr = &stderr

	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("starting command: %w", err)
	}

	if _, err := stdin.Write(content); err != nil {
		return fmt.Errorf("writing content: %w", err)
	}
	stdin.Close()

	if err := session.Wait(); err != nil {
		return fmt.Errorf("command failed: %s", stderr.String())
	}

	return nil
}

// WriteFileWithSudo writes content to a file using sudo.
func (c *SSHClient) WriteFileWithSudo(ctx context.Context, path string, content []byte, mode string) error {
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	// Use sudo tee to write file
	cmd := fmt.Sprintf("sudo tee %s > /dev/null && sudo chmod %s %s", path, mode, path)

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("getting stdin pipe: %w", err)
	}

	var stderr bytes.Buffer
	session.Stderr = &stderr

	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("starting command: %w", err)
	}

	if _, err := stdin.Write(content); err != nil {
		return fmt.Errorf("writing content: %w", err)
	}
	stdin.Close()

	if err := session.Wait(); err != nil {
		return fmt.Errorf("command failed: %s", stderr.String())
	}

	return nil
}

// AppendToFile appends content to a file.
func (c *SSHClient) AppendToFile(ctx context.Context, path string, content string) error {
	cmd := fmt.Sprintf("echo '%s' >> %s", escapeForBash(content), path)
	_, err := c.Run(ctx, cmd)
	return err
}

// FileExists checks if a file exists.
func (c *SSHClient) FileExists(ctx context.Context, path string) (bool, error) {
	_, err := c.Run(ctx, fmt.Sprintf("test -f %s", path))
	if err != nil {
		// Check if it's a "file not found" error vs other error
		if strings.Contains(err.Error(), "exit status 1") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DirExists checks if a directory exists.
func (c *SSHClient) DirExists(ctx context.Context, path string) (bool, error) {
	_, err := c.Run(ctx, fmt.Sprintf("test -d %s", path))
	if err != nil {
		if strings.Contains(err.Error(), "exit status 1") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DownloadFile downloads a file from a URL to the remote host.
func (c *SSHClient) DownloadFile(ctx context.Context, url, destPath string) error {
	// Try curl first, then wget
	_, err := c.Run(ctx, fmt.Sprintf("curl -fsSL -o %s '%s' || wget -q -O %s '%s'",
		destPath, url, destPath, url))
	return err
}

// DownloadFileWithSudo downloads a file using sudo.
func (c *SSHClient) DownloadFileWithSudo(ctx context.Context, url, destPath string) error {
	// Download to temp, then move with sudo
	tmpPath := fmt.Sprintf("/tmp/icmpmon-download-%d", time.Now().UnixNano())
	if err := c.DownloadFile(ctx, url, tmpPath); err != nil {
		return fmt.Errorf("downloading file: %w", err)
	}
	_, err := c.RunWithSudo(ctx, fmt.Sprintf("mv %s %s", tmpPath, destPath))
	return err
}

// CopyReader copies data from a reader to a remote file using SCP.
func (c *SSHClient) CopyReader(ctx context.Context, r io.Reader, size int64, destPath, mode string) error {
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	// Use scp protocol
	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()
		fmt.Fprintf(w, "C%s %d %s\n", mode, size, destPath)
		io.Copy(w, r)
		fmt.Fprint(w, "\x00")
	}()

	if err := session.Run("scp -t " + destPath); err != nil {
		return fmt.Errorf("scp failed: %w", err)
	}

	return nil
}

// escapeForBash escapes a string for use in bash single quotes.
func escapeForBash(s string) string {
	// Replace single quotes with '\'' (end quote, escaped quote, start quote)
	return strings.ReplaceAll(s, "'", "'\\''")
}
