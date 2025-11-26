package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/1Password/connect-sdk-go/connect"
	"github.com/1Password/connect-sdk-go/onepassword"
)

// OnePasswordKeyStore stores SSH keys in 1Password using the Connect API.
//
// Configuration is via environment variables:
//   - OP_CONNECT_HOST: URL of the 1Password Connect server
//   - OP_CONNECT_TOKEN: Access token for the Connect server
//   - OP_VAULT_ID: UUID of the vault to store keys in
type OnePasswordKeyStore struct {
	client    connect.Client
	vaultID   string
	logger    *slog.Logger

	// Cache to avoid repeated API calls
	mu        sync.RWMutex
	keyCache  map[string]*SSHKeyPair
}

// OnePasswordConfig holds configuration for 1Password Connect.
type OnePasswordConfig struct {
	Host    string // OP_CONNECT_HOST
	Token   string // OP_CONNECT_TOKEN
	VaultID string // OP_VAULT_ID
}

// NewOnePasswordKeyStore creates a new 1Password-backed key store.
func NewOnePasswordKeyStore(cfg OnePasswordConfig, logger *slog.Logger) (*OnePasswordKeyStore, error) {
	if cfg.Host == "" || cfg.Token == "" || cfg.VaultID == "" {
		return nil, fmt.Errorf("1Password configuration incomplete: host, token, and vault_id are required")
	}

	client := connect.NewClientWithUserAgent(cfg.Host, cfg.Token, "icmpmon-control-plane")

	return &OnePasswordKeyStore{
		client:   client,
		vaultID:  cfg.VaultID,
		logger:   logger,
		keyCache: make(map[string]*SSHKeyPair),
	}, nil
}

// GetOrCreateProvisioningKey returns the control plane's SSH key pair,
// creating one if it doesn't exist.
func (ks *OnePasswordKeyStore) GetOrCreateProvisioningKey(ctx context.Context) (*SSHKeyPair, error) {
	// Check cache first
	ks.mu.RLock()
	if cached, ok := ks.keyCache[DefaultKeyName]; ok {
		ks.mu.RUnlock()
		return cached, nil
	}
	ks.mu.RUnlock()

	// Try to get existing key
	keyPair, err := ks.getKeyFromVault(ctx, DefaultKeyName)
	if err != nil {
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
	if err := ks.storeKeyInVault(ctx, keyPair); err != nil {
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
func (ks *OnePasswordKeyStore) GetPrivateKey(ctx context.Context, name string) ([]byte, error) {
	keyPair, err := ks.getKeyFromVault(ctx, name)
	if err != nil {
		return nil, err
	}
	if keyPair == nil {
		return nil, nil
	}
	return keyPair.PrivateKey, nil
}

// GetPublicKey retrieves the public key in OpenSSH format.
func (ks *OnePasswordKeyStore) GetPublicKey(ctx context.Context, name string) (string, error) {
	// Check cache first
	ks.mu.RLock()
	if cached, ok := ks.keyCache[name]; ok {
		ks.mu.RUnlock()
		return cached.PublicKey, nil
	}
	ks.mu.RUnlock()

	keyPair, err := ks.getKeyFromVault(ctx, name)
	if err != nil {
		return "", err
	}
	if keyPair == nil {
		return "", fmt.Errorf("key not found: %s", name)
	}
	return keyPair.PublicKey, nil
}

// RotateKey creates a new key pair and archives the old one.
func (ks *OnePasswordKeyStore) RotateKey(ctx context.Context) (*SSHKeyPair, error) {
	// Get the old key to archive it
	oldKey, err := ks.getKeyFromVault(ctx, DefaultKeyName)
	if err != nil {
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
		if err := ks.storeKeyInVault(ctx, oldKey); err != nil {
			ks.logger.Warn("failed to archive old key", "error", err)
			// Continue with rotation anyway
		}
	}

	// Update the main key in 1Password
	if err := ks.updateKeyInVault(ctx, newKey); err != nil {
		return nil, fmt.Errorf("updating key in 1Password: %w", err)
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
func (ks *OnePasswordKeyStore) Close() error {
	// Clear cache
	ks.mu.Lock()
	ks.keyCache = make(map[string]*SSHKeyPair)
	ks.mu.Unlock()
	return nil
}

// getKeyFromVault retrieves a key from 1Password by name.
func (ks *OnePasswordKeyStore) getKeyFromVault(ctx context.Context, name string) (*SSHKeyPair, error) {
	// List items to find the key by title
	items, err := ks.client.GetItemsByTitle(name, ks.vaultID)
	if err != nil {
		// Check if it's a "not found" error
		if isNotFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing items: %w", err)
	}

	if len(items) == 0 {
		return nil, nil
	}

	// Get the full item (including fields)
	item, err := ks.client.GetItem(items[0].ID, ks.vaultID)
	if err != nil {
		return nil, fmt.Errorf("getting item: %w", err)
	}

	return ks.itemToKeyPair(item)
}

// storeKeyInVault stores a new key in 1Password.
func (ks *OnePasswordKeyStore) storeKeyInVault(ctx context.Context, keyPair *SSHKeyPair) error {
	item := ks.keyPairToItem(keyPair)

	_, err := ks.client.CreateItem(item, ks.vaultID)
	if err != nil {
		return fmt.Errorf("creating item: %w", err)
	}

	return nil
}

// updateKeyInVault updates an existing key in 1Password.
func (ks *OnePasswordKeyStore) updateKeyInVault(ctx context.Context, keyPair *SSHKeyPair) error {
	// First, get the existing item to get its ID
	items, err := ks.client.GetItemsByTitle(keyPair.Name, ks.vaultID)
	if err != nil {
		return fmt.Errorf("finding item: %w", err)
	}

	item := ks.keyPairToItem(keyPair)

	if len(items) == 0 {
		// Item doesn't exist, create it
		_, err = ks.client.CreateItem(item, ks.vaultID)
	} else {
		// Update existing item
		item.ID = items[0].ID
		_, err = ks.client.UpdateItem(item, ks.vaultID)
	}

	if err != nil {
		return fmt.Errorf("saving item: %w", err)
	}

	return nil
}

// keyPairToItem converts an SSHKeyPair to a 1Password item.
func (ks *OnePasswordKeyStore) keyPairToItem(keyPair *SSHKeyPair) *onepassword.Item {
	// Serialize metadata as JSON for the notes field
	metadata := map[string]any{
		"key_type":    keyPair.KeyType,
		"fingerprint": keyPair.Fingerprint,
		"created_at":  keyPair.CreatedAt.Format(time.RFC3339),
	}
	if keyPair.RotatedAt != nil {
		metadata["rotated_at"] = keyPair.RotatedAt.Format(time.RFC3339)
	}
	metadataJSON, _ := json.Marshal(metadata)

	return &onepassword.Item{
		Title:    keyPair.Name,
		Category: onepassword.SSHKey,
		Vault:    onepassword.ItemVault{ID: ks.vaultID},
		Fields: []*onepassword.ItemField{
			{
				ID:      "public_key",
				Label:   "public key",
				Type:    "STRING",
				Value:   keyPair.PublicKey,
			},
			{
				ID:      "private_key",
				Label:   "private key",
				Type:    "CONCEALED",
				Value:   string(keyPair.PrivateKey),
			},
			{
				ID:      "fingerprint",
				Label:   "fingerprint",
				Type:    "STRING",
				Value:   keyPair.Fingerprint,
			},
			{
				ID:       "notesPlain",
				Label:    "notesPlain",
				Type:     "STRING",
				Value:    string(metadataJSON),
				Purpose:  "NOTES",
			},
		},
	}
}

// itemToKeyPair converts a 1Password item to an SSHKeyPair.
func (ks *OnePasswordKeyStore) itemToKeyPair(item *onepassword.Item) (*SSHKeyPair, error) {
	keyPair := &SSHKeyPair{
		ID:      item.ID,
		Name:    item.Title,
		KeyType: "ed25519", // default
	}

	for _, field := range item.Fields {
		switch field.ID {
		case "public_key":
			keyPair.PublicKey = field.Value
		case "private_key":
			keyPair.PrivateKey = []byte(field.Value)
		case "fingerprint":
			keyPair.Fingerprint = field.Value
		case "notesPlain":
			// Parse metadata from notes
			var metadata map[string]any
			if err := json.Unmarshal([]byte(field.Value), &metadata); err == nil {
				if kt, ok := metadata["key_type"].(string); ok {
					keyPair.KeyType = kt
				}
				if fp, ok := metadata["fingerprint"].(string); ok && keyPair.Fingerprint == "" {
					keyPair.Fingerprint = fp
				}
				if cat, ok := metadata["created_at"].(string); ok {
					if t, err := time.Parse(time.RFC3339, cat); err == nil {
						keyPair.CreatedAt = t
					}
				}
				if rat, ok := metadata["rotated_at"].(string); ok {
					if t, err := time.Parse(time.RFC3339, rat); err == nil {
						keyPair.RotatedAt = &t
					}
				}
			}
		}
	}

	if keyPair.CreatedAt.IsZero() {
		keyPair.CreatedAt = item.CreatedAt
	}

	return keyPair, nil
}

// isNotFoundError checks if an error is a "not found" error from 1Password.
func isNotFoundError(err error) bool {
	// The 1Password SDK returns different error types, check the message
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "not found") || contains(errStr, "404") || contains(errStr, "no items")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
