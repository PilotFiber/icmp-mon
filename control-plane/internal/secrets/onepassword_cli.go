package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// OnePasswordCLIKeyStore uses the 1Password CLI with Service Account authentication.
// This is the recommended approach for using 1Password Service Accounts in Go.
//
// Prerequisites:
//   - 1Password CLI (op) must be installed: https://developer.1password.com/docs/cli/
//   - Service Account token must be set: OP_SERVICE_ACCOUNT_TOKEN
type OnePasswordCLIKeyStore struct {
	token   string
	vault   string
	logger  *slog.Logger

	mu       sync.RWMutex
	keyCache map[string]*SSHKeyPair
}

// opItem represents a 1Password item from the CLI.
type opItem struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Category  string    `json:"category"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Fields    []opField `json:"fields"`
}

type opField struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Label   string `json:"label"`
	Value   string `json:"value"`
	Purpose string `json:"purpose,omitempty"`
}

// NewOnePasswordCLIKeyStore creates a new key store using the 1Password CLI.
func NewOnePasswordCLIKeyStore(token, vault string, logger *slog.Logger) (*OnePasswordCLIKeyStore, error) {
	if token == "" {
		return nil, fmt.Errorf("1Password service account token is required")
	}

	ks := &OnePasswordCLIKeyStore{
		token:    token,
		vault:    vault,
		logger:   logger,
		keyCache: make(map[string]*SSHKeyPair),
	}

	// Verify CLI is installed and token works
	if err := ks.verifyAccess(); err != nil {
		return nil, fmt.Errorf("verifying 1Password access: %w", err)
	}

	logger.Info("initialized 1Password key store", "vault", vault)
	return ks, nil
}

// verifyAccess checks that the CLI is installed and the token is valid.
func (ks *OnePasswordCLIKeyStore) verifyAccess() error {
	// Check if op CLI is installed
	if _, err := exec.LookPath("op"); err != nil {
		return fmt.Errorf("1Password CLI (op) not found in PATH - install from https://developer.1password.com/docs/cli/")
	}

	// Test authentication by listing vaults
	_, err := ks.runOP("vault", "list", "--format=json")
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	return nil
}

// runOP executes an op CLI command with the service account token.
func (ks *OnePasswordCLIKeyStore) runOP(args ...string) ([]byte, error) {
	cmd := exec.Command("op", args...)
	cmd.Env = append(cmd.Environ(), "OP_SERVICE_ACCOUNT_TOKEN="+ks.token)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// GetOrCreateProvisioningKey returns the control plane's SSH key pair,
// creating one if it doesn't exist.
func (ks *OnePasswordCLIKeyStore) GetOrCreateProvisioningKey(ctx context.Context) (*SSHKeyPair, error) {
	// Check cache first
	ks.mu.RLock()
	if cached, ok := ks.keyCache[DefaultKeyName]; ok {
		ks.mu.RUnlock()
		return cached, nil
	}
	ks.mu.RUnlock()

	// Try to get existing key
	keyPair, err := ks.getKey(DefaultKeyName)
	if err != nil && !isItemNotFound(err) {
		return nil, fmt.Errorf("checking for existing key: %w", err)
	}

	if keyPair != nil {
		// Cache and return existing key
		ks.mu.Lock()
		ks.keyCache[DefaultKeyName] = keyPair
		ks.mu.Unlock()
		return keyPair, nil
	}

	// Key doesn't exist, create new one
	ks.logger.Info("creating new provisioning SSH key", "name", DefaultKeyName)

	keyPair, err = GenerateSSHKeyPair(DefaultKeyName)
	if err != nil {
		return nil, fmt.Errorf("generating key pair: %w", err)
	}

	// Store in 1Password
	if err := ks.createKey(keyPair); err != nil {
		return nil, fmt.Errorf("storing key in 1Password: %w", err)
	}

	// Cache and return
	ks.mu.Lock()
	ks.keyCache[DefaultKeyName] = keyPair
	ks.mu.Unlock()

	ks.logger.Info("created new provisioning SSH key",
		"name", DefaultKeyName,
		"fingerprint", keyPair.Fingerprint)

	return keyPair, nil
}

// GetPrivateKey retrieves only the private key bytes for a named key.
func (ks *OnePasswordCLIKeyStore) GetPrivateKey(ctx context.Context, name string) ([]byte, error) {
	keyPair, err := ks.getKey(name)
	if err != nil {
		if isItemNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if keyPair == nil {
		return nil, nil
	}
	return keyPair.PrivateKey, nil
}

// GetPublicKey retrieves the public key in OpenSSH format.
func (ks *OnePasswordCLIKeyStore) GetPublicKey(ctx context.Context, name string) (string, error) {
	// Check cache first
	ks.mu.RLock()
	if cached, ok := ks.keyCache[name]; ok {
		ks.mu.RUnlock()
		return cached.PublicKey, nil
	}
	ks.mu.RUnlock()

	keyPair, err := ks.getKey(name)
	if err != nil {
		return "", err
	}
	if keyPair == nil {
		return "", fmt.Errorf("key not found: %s", name)
	}
	return keyPair.PublicKey, nil
}

// RotateKey creates a new key pair and archives the old one.
func (ks *OnePasswordCLIKeyStore) RotateKey(ctx context.Context) (*SSHKeyPair, error) {
	// Get the old key to archive it
	oldKey, err := ks.getKey(DefaultKeyName)
	if err != nil && !isItemNotFound(err) {
		return nil, fmt.Errorf("getting old key: %w", err)
	}

	// Generate new key
	newKey, err := GenerateSSHKeyPair(DefaultKeyName)
	if err != nil {
		return nil, fmt.Errorf("generating new key: %w", err)
	}
	now := time.Now()
	newKey.RotatedAt = &now

	// Archive old key if it exists
	if oldKey != nil {
		archiveName := fmt.Sprintf("%s-archived-%s", DefaultKeyName, time.Now().Format("20060102-150405"))
		oldKey.Name = archiveName
		if err := ks.createKey(oldKey); err != nil {
			ks.logger.Warn("failed to archive old key", "error", err)
		}
		// Delete the old key item
		ks.deleteKey(DefaultKeyName)
	}

	// Create the new key
	if err := ks.createKey(newKey); err != nil {
		return nil, fmt.Errorf("storing new key: %w", err)
	}

	// Update cache
	ks.mu.Lock()
	ks.keyCache[DefaultKeyName] = newKey
	ks.mu.Unlock()

	ks.logger.Info("rotated provisioning SSH key",
		"fingerprint", newKey.Fingerprint)

	return newKey, nil
}

// Close releases any resources.
func (ks *OnePasswordCLIKeyStore) Close() error {
	ks.mu.Lock()
	ks.keyCache = make(map[string]*SSHKeyPair)
	ks.mu.Unlock()
	return nil
}

// getKey retrieves a key from 1Password by name.
func (ks *OnePasswordCLIKeyStore) getKey(name string) (*SSHKeyPair, error) {
	// Get item by title
	output, err := ks.runOP("item", "get", name, "--vault="+ks.vault, "--format=json")
	if err != nil {
		return nil, err
	}

	var item opItem
	if err := json.Unmarshal(output, &item); err != nil {
		return nil, fmt.Errorf("parsing item: %w", err)
	}

	return ks.itemToKeyPair(&item)
}

// createKey creates a new key in 1Password.
func (ks *OnePasswordCLIKeyStore) createKey(keyPair *SSHKeyPair) error {
	// Create item with fields
	// op item create --category="Secure Note" --title="name" --vault="vault" 'field=value'

	metadata := map[string]any{
		"key_type":    keyPair.KeyType,
		"fingerprint": keyPair.Fingerprint,
		"created_at":  keyPair.CreatedAt.Format(time.RFC3339),
	}
	if keyPair.RotatedAt != nil {
		metadata["rotated_at"] = keyPair.RotatedAt.Format(time.RFC3339)
	}
	metadataJSON, _ := json.Marshal(metadata)

	args := []string{
		"item", "create",
		"--category=Secure Note",
		"--title=" + keyPair.Name,
		"--vault=" + ks.vault,
		"public_key[text]=" + strings.TrimSpace(keyPair.PublicKey),
		"private_key[concealed]=" + string(keyPair.PrivateKey),
		"fingerprint[text]=" + keyPair.Fingerprint,
		"metadata[text]=" + string(metadataJSON),
	}

	_, err := ks.runOP(args...)
	return err
}

// deleteKey deletes a key from 1Password.
func (ks *OnePasswordCLIKeyStore) deleteKey(name string) error {
	_, err := ks.runOP("item", "delete", name, "--vault="+ks.vault)
	return err
}

// itemToKeyPair converts a 1Password item to an SSHKeyPair.
func (ks *OnePasswordCLIKeyStore) itemToKeyPair(item *opItem) (*SSHKeyPair, error) {
	keyPair := &SSHKeyPair{
		ID:        item.ID,
		Name:      item.Title,
		KeyType:   "ed25519",
		CreatedAt: item.CreatedAt,
	}

	for _, field := range item.Fields {
		switch field.Label {
		case "public_key", "public key":
			keyPair.PublicKey = field.Value
		case "private_key", "private key":
			keyPair.PrivateKey = []byte(field.Value)
		case "fingerprint":
			keyPair.Fingerprint = field.Value
		case "metadata":
			var meta map[string]any
			if err := json.Unmarshal([]byte(field.Value), &meta); err == nil {
				if kt, ok := meta["key_type"].(string); ok {
					keyPair.KeyType = kt
				}
				if cat, ok := meta["created_at"].(string); ok {
					if t, err := time.Parse(time.RFC3339, cat); err == nil {
						keyPair.CreatedAt = t
					}
				}
				if rat, ok := meta["rotated_at"].(string); ok {
					if t, err := time.Parse(time.RFC3339, rat); err == nil {
						keyPair.RotatedAt = &t
					}
				}
			}
		}
	}

	return keyPair, nil
}

// isItemNotFound checks if an error indicates the item was not found.
func isItemNotFound(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "no item") ||
		strings.Contains(errStr, "doesn't exist")
}
