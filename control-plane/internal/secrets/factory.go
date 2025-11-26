package secrets

import (
	"fmt"
	"log/slog"
	"os"
)

// Config holds configuration for the secrets backend.
type Config struct {
	// Backend specifies which backend to use: "1password", "local", or "auto"
	// "auto" (default) uses 1Password if configured, otherwise local
	Backend string

	// 1Password Service Account configuration
	// Set via environment: OP_SERVICE_ACCOUNT_TOKEN
	OnePasswordToken string

	// 1Password vault name (default: "icmp-mon keys")
	OnePasswordVault string

	// Local storage directory (default: ~/.icmpmon/keys)
	LocalKeyDir string
}

// ConfigFromEnv creates a Config from environment variables.
func ConfigFromEnv() Config {
	cfg := Config{
		Backend:          getEnv("ICMPMON_SECRETS_BACKEND", "auto"),
		OnePasswordToken: os.Getenv("OP_SERVICE_ACCOUNT_TOKEN"),
		OnePasswordVault: getEnv("OP_VAULT", "icmp-mon keys"),
		LocalKeyDir:      os.Getenv("ICMPMON_KEY_DIR"),
	}
	return cfg
}

// NewKeyStore creates a KeyStore based on configuration.
func NewKeyStore(cfg Config, logger *slog.Logger) (KeyStore, error) {
	backend := cfg.Backend
	if backend == "" {
		backend = "auto"
	}

	switch backend {
	case "1password":
		if cfg.OnePasswordToken == "" {
			return nil, fmt.Errorf("1Password backend requested but OP_SERVICE_ACCOUNT_TOKEN not set")
		}
		return NewOnePasswordCLIKeyStore(cfg.OnePasswordToken, cfg.OnePasswordVault, logger)

	case "local":
		return NewLocalKeyStore(cfg.LocalKeyDir, logger)

	case "auto":
		// Try 1Password first, fall back to local
		if cfg.OnePasswordToken != "" {
			ks, err := NewOnePasswordCLIKeyStore(cfg.OnePasswordToken, cfg.OnePasswordVault, logger)
			if err != nil {
				logger.Warn("failed to initialize 1Password, falling back to local storage",
					"error", err)
				return NewLocalKeyStore(cfg.LocalKeyDir, logger)
			}
			return ks, nil
		}
		logger.Info("OP_SERVICE_ACCOUNT_TOKEN not set, using local key storage")
		return NewLocalKeyStore(cfg.LocalKeyDir, logger)

	default:
		return nil, fmt.Errorf("unknown secrets backend: %s", backend)
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
