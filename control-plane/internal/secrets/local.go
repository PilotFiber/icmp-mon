package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LocalKeyStore stores SSH keys on the local filesystem.
// This is intended for development and testing only.
//
// Keys are stored in a directory with the following structure:
//
//	<base_dir>/
//	  <key_name>.json  (metadata)
//	  <key_name>.pem   (private key)
//	  <key_name>.pub   (public key)
type LocalKeyStore struct {
	baseDir string
	logger  *slog.Logger

	mu       sync.RWMutex
	keyCache map[string]*SSHKeyPair
}

// keyMetadata is the JSON structure stored alongside keys.
type keyMetadata struct {
	Name        string     `json:"name"`
	KeyType     string     `json:"key_type"`
	PublicKey   string     `json:"public_key"`
	Fingerprint string     `json:"fingerprint"`
	CreatedAt   time.Time  `json:"created_at"`
	RotatedAt   *time.Time `json:"rotated_at,omitempty"`
}

// NewLocalKeyStore creates a new local filesystem-backed key store.
// If baseDir is empty, it defaults to ~/.icmpmon/keys.
func NewLocalKeyStore(baseDir string, logger *slog.Logger) (*LocalKeyStore, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home directory: %w", err)
		}
		baseDir = filepath.Join(home, ".icmpmon", "keys")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("creating key directory: %w", err)
	}

	logger.Info("using local key store", "path", baseDir)

	return &LocalKeyStore{
		baseDir:  baseDir,
		logger:   logger,
		keyCache: make(map[string]*SSHKeyPair),
	}, nil
}

// GetOrCreateProvisioningKey returns the control plane's SSH key pair,
// creating one if it doesn't exist.
func (ks *LocalKeyStore) GetOrCreateProvisioningKey(ctx context.Context) (*SSHKeyPair, error) {
	// Check cache first
	ks.mu.RLock()
	if cached, ok := ks.keyCache[DefaultKeyName]; ok {
		ks.mu.RUnlock()
		return cached, nil
	}
	ks.mu.RUnlock()

	// Try to load from disk
	keyPair, err := ks.loadKey(DefaultKeyName)
	if err != nil {
		return nil, fmt.Errorf("loading key: %w", err)
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

	// Save to disk
	if err := ks.saveKey(keyPair); err != nil {
		return nil, fmt.Errorf("saving key: %w", err)
	}

	// Cache and return
	ks.mu.Lock()
	ks.keyCache[DefaultKeyName] = keyPair
	ks.mu.Unlock()

	ks.logger.Info("created new provisioning SSH key",
		"name", DefaultKeyName,
		"fingerprint", keyPair.Fingerprint,
		"path", ks.baseDir)

	return keyPair, nil
}

// GetPrivateKey retrieves only the private key bytes for a named key.
func (ks *LocalKeyStore) GetPrivateKey(ctx context.Context, name string) ([]byte, error) {
	keyPair, err := ks.loadKey(name)
	if err != nil {
		return nil, err
	}
	if keyPair == nil {
		return nil, nil
	}
	return keyPair.PrivateKey, nil
}

// GetPublicKey retrieves the public key in OpenSSH format.
func (ks *LocalKeyStore) GetPublicKey(ctx context.Context, name string) (string, error) {
	// Check cache first
	ks.mu.RLock()
	if cached, ok := ks.keyCache[name]; ok {
		ks.mu.RUnlock()
		return cached.PublicKey, nil
	}
	ks.mu.RUnlock()

	keyPair, err := ks.loadKey(name)
	if err != nil {
		return "", err
	}
	if keyPair == nil {
		return "", fmt.Errorf("key not found: %s", name)
	}
	return keyPair.PublicKey, nil
}

// RotateKey creates a new key pair and archives the old one.
func (ks *LocalKeyStore) RotateKey(ctx context.Context) (*SSHKeyPair, error) {
	// Get the old key to archive it
	oldKey, err := ks.loadKey(DefaultKeyName)
	if err != nil {
		return nil, fmt.Errorf("loading old key: %w", err)
	}

	// Archive old key if it exists
	if oldKey != nil {
		archiveName := fmt.Sprintf("%s-archived-%s", DefaultKeyName, time.Now().Format("20060102-150405"))
		oldKey.Name = archiveName
		if err := ks.saveKey(oldKey); err != nil {
			ks.logger.Warn("failed to archive old key", "error", err)
			// Continue with rotation anyway
		}
	}

	// Generate new key
	newKey, err := GenerateSSHKeyPair(DefaultKeyName)
	if err != nil {
		return nil, fmt.Errorf("generating new key: %w", err)
	}
	now := time.Now()
	newKey.RotatedAt = &now

	// Save new key
	if err := ks.saveKey(newKey); err != nil {
		return nil, fmt.Errorf("saving new key: %w", err)
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
func (ks *LocalKeyStore) Close() error {
	// Clear cache
	ks.mu.Lock()
	ks.keyCache = make(map[string]*SSHKeyPair)
	ks.mu.Unlock()
	return nil
}

// loadKey loads a key from disk by name.
func (ks *LocalKeyStore) loadKey(name string) (*SSHKeyPair, error) {
	metadataPath := filepath.Join(ks.baseDir, name+".json")
	privatePath := filepath.Join(ks.baseDir, name+".pem")

	// Check if metadata file exists
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		return nil, nil
	}

	// Read metadata
	metadataBytes, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("reading metadata: %w", err)
	}

	var meta keyMetadata
	if err := json.Unmarshal(metadataBytes, &meta); err != nil {
		return nil, fmt.Errorf("parsing metadata: %w", err)
	}

	// Read private key
	privateBytes, err := os.ReadFile(privatePath)
	if err != nil {
		return nil, fmt.Errorf("reading private key: %w", err)
	}

	return &SSHKeyPair{
		Name:        meta.Name,
		KeyType:     meta.KeyType,
		PublicKey:   meta.PublicKey,
		PrivateKey:  privateBytes,
		Fingerprint: meta.Fingerprint,
		CreatedAt:   meta.CreatedAt,
		RotatedAt:   meta.RotatedAt,
	}, nil
}

// saveKey saves a key to disk.
func (ks *LocalKeyStore) saveKey(keyPair *SSHKeyPair) error {
	metadataPath := filepath.Join(ks.baseDir, keyPair.Name+".json")
	privatePath := filepath.Join(ks.baseDir, keyPair.Name+".pem")
	publicPath := filepath.Join(ks.baseDir, keyPair.Name+".pub")

	// Write metadata
	meta := keyMetadata{
		Name:        keyPair.Name,
		KeyType:     keyPair.KeyType,
		PublicKey:   keyPair.PublicKey,
		Fingerprint: keyPair.Fingerprint,
		CreatedAt:   keyPair.CreatedAt,
		RotatedAt:   keyPair.RotatedAt,
	}
	metadataBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}
	if err := os.WriteFile(metadataPath, metadataBytes, 0600); err != nil {
		return fmt.Errorf("writing metadata: %w", err)
	}

	// Write private key (restrictive permissions)
	if err := os.WriteFile(privatePath, keyPair.PrivateKey, 0600); err != nil {
		return fmt.Errorf("writing private key: %w", err)
	}

	// Write public key (readable)
	if err := os.WriteFile(publicPath, []byte(keyPair.PublicKey), 0644); err != nil {
		return fmt.Errorf("writing public key: %w", err)
	}

	return nil
}
