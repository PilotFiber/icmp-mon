// Package secrets provides secure storage for sensitive data like SSH keys.
//
// This package defines a KeyStore interface for managing SSH keys used during
// agent enrollment. The primary implementation uses 1Password Connect for
// production environments, with a local file-based fallback for development.
package secrets

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHKeyPair represents an SSH key pair with metadata.
type SSHKeyPair struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	KeyType     string    `json:"key_type"`     // "ed25519"
	PublicKey   string    `json:"public_key"`   // OpenSSH format (ssh-ed25519 AAAA...)
	PrivateKey  []byte    `json:"-"`            // PEM encoded, never serialized to JSON
	Fingerprint string    `json:"fingerprint"`  // SHA256 fingerprint
	CreatedAt   time.Time `json:"created_at"`
	RotatedAt   *time.Time `json:"rotated_at,omitempty"`
}

// KeyStore provides secure storage and retrieval of SSH keys.
type KeyStore interface {
	// GetOrCreateProvisioningKey returns the control plane's SSH key pair,
	// creating one if it doesn't exist. The key is used for agent enrollment.
	GetOrCreateProvisioningKey(ctx context.Context) (*SSHKeyPair, error)

	// GetPrivateKey retrieves only the private key bytes for a named key.
	// Returns nil if the key doesn't exist.
	GetPrivateKey(ctx context.Context, name string) ([]byte, error)

	// RotateKey creates a new key pair, archives the old one, and returns the new key.
	// The old key remains valid for a grace period to allow in-flight enrollments.
	RotateKey(ctx context.Context) (*SSHKeyPair, error)

	// GetPublicKey retrieves the public key in OpenSSH format.
	GetPublicKey(ctx context.Context, name string) (string, error)

	// Close releases any resources held by the key store.
	Close() error
}

// DefaultKeyName is the name of the default provisioning key.
const DefaultKeyName = "icmpmon-provisioning"

// GenerateSSHKeyPair generates a new Ed25519 SSH key pair.
func GenerateSSHKeyPair(name string) (*SSHKeyPair, error) {
	// Generate Ed25519 key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating ed25519 key: %w", err)
	}

	// Convert to SSH format
	sshPubKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return nil, fmt.Errorf("converting to ssh public key: %w", err)
	}

	// Encode private key to OpenSSH format
	privKeyPEM, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return nil, fmt.Errorf("marshaling private key: %w", err)
	}

	// Get fingerprint
	fingerprint := ssh.FingerprintSHA256(sshPubKey)

	// Format public key in authorized_keys format
	pubKeyStr := string(ssh.MarshalAuthorizedKey(sshPubKey))

	return &SSHKeyPair{
		Name:        name,
		KeyType:     "ed25519",
		PublicKey:   pubKeyStr,
		PrivateKey:  pem.EncodeToMemory(privKeyPEM),
		Fingerprint: fingerprint,
		CreatedAt:   time.Now(),
	}, nil
}

// ParsePrivateKey parses a PEM-encoded private key and returns an ssh.Signer.
func ParsePrivateKey(pemBytes []byte) (ssh.Signer, error) {
	signer, err := ssh.ParsePrivateKey(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}
	return signer, nil
}
