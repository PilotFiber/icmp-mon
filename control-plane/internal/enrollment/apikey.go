package enrollment

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// GenerateAgentAPIKey generates a new API key for an agent.
// Returns the plaintext key and its bcrypt hash.
func GenerateAgentAPIKey(agentID string) (plaintext string, hash string, err error) {
	// Generate 32 random bytes
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", fmt.Errorf("generating random bytes: %w", err)
	}

	// Format: icmpmon_<agent_prefix>_<base64>
	// Use first 6 chars of agent ID as prefix for easy identification
	prefix := agentID
	if len(prefix) > 6 {
		prefix = prefix[:6]
	}

	encoded := base64.URLEncoding.EncodeToString(randomBytes)
	plaintext = fmt.Sprintf("icmpmon_%s_%s", prefix, encoded)

	// Hash with bcrypt for storage
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", "", fmt.Errorf("hashing API key: %w", err)
	}

	return plaintext, string(hashBytes), nil
}

// VerifyAPIKey compares a plaintext API key against a bcrypt hash.
func VerifyAPIKey(plaintext, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext))
	return err == nil
}
