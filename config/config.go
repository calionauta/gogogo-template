package config

import (
	"log"
	"os"
	"strconv"

	"github.com/calionauta/gogogo-fullstack-template/internal/secrets"
)

type Config struct {
	// AppName is used as the scope for the secrets file
	// (~/.secrets/<AppName>.env.age) and the project name in logs.
	// Derived from APP_NAME env or the binary name; never empty.
	AppName string

	Host     string
	Port     int
	LogLevel string
	Dev      bool

	DBPath        string
	DataDir       string
	EncryptionKey string

	// AdminToken, when non-empty, unlocks the admin endpoints (e.g. the
	// Todo "clear all" via token). Loaded from the age-decrypted
	// secrets file, NOT from the host environment directly. This is the
	// canonical example of "use a real secret in the demo app" — see
	// README's "Admin unlock" section.
	AdminToken string

	NATS struct {
		Enabled  bool
		StoreDir string
	}

	Workflow struct {
		Enabled    bool
		ExecutorID string
	}

	GoAI GoAIConfig
}

// GoAIConfig holds the LLM client settings. Currently just an
// APIKey + a model; expanded in internal/llm as more knobs
// (GOAI_BASE_URL, GOAI_MODEL, etc.) are read from env.
type GoAIConfig struct {
	APIKey string
}

// Load builds the Config. Order matters: secrets must be decrypted
// BEFORE reading the rest of the env so admin/LLM/NATS values can
// come from the encrypted file.
func Load() *Config {
	appName := os.Getenv("APP_NAME")
	if appName == "" {
		appName = defaultAppName()
	}

	// Decrypt ~/.secrets/<appName>.env.age into the process env. Silent
	// skip when AGE_SECRET_KEY or the secrets file is missing.
	secrets.Load(appName)

	dev := os.Getenv("ENVIRONMENT") != "production"

	port := 8080
	if p := os.Getenv("PORT"); p != "" {
		parsed, err := strconv.Atoi(p)
		if err != nil {
			log.Printf("config: invalid PORT=%q, using %d: %v", p, port, err)
		} else {
			port = parsed
		}
	}

	cfg := &Config{
		AppName:       appName,
		Host:          getEnv("HOST", "0.0.0.0"),
		Port:          port,
		LogLevel:      getEnv("LOG_LEVEL", "INFO"),
		Dev:           dev,
		DBPath:        getEnv("DATABASE_PATH", "data/app.db"),
		DataDir:       getEnv("DATA_DIR", "data"),
		EncryptionKey: os.Getenv("ENCRYPTION_KEY"),
		AdminToken:    os.Getenv("ADMIN_UNLOCK_TOKEN"),
		GoAI: GoAIConfig{
			APIKey: os.Getenv("GOAI_API_KEY"),
		},
	}

	cfg.NATS.Enabled = envBool("NATS_ENABLED", defaultNATSEnabled())
	cfg.NATS.StoreDir = getEnv("NATS_STORE_DIR", "data/nats")

	cfg.Workflow.Enabled = envBool("WORKFLOW_ENABLED", defaultWorkflowEnabled())
	cfg.Workflow.ExecutorID = getEnv("WORKFLOW_EXECUTOR_ID", "local")

	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envBool reads a boolean env var, falling back to def when unset or
// unparseable. This lets a build tag supply the default (e.g. -tags
// jetstream implies NATS on) while still allowing an explicit override
// via the env var (NATS_ENABLED=false).
func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// defaultAppName falls back to the binary name when APP_NAME is unset
// so the secrets file scope tracks whatever the project owner actually
// compiled. Uses os.Args[0] (binary path) trimmed to base name; if that
// fails (e.g. tests), it returns a hard-coded stable name.
func defaultAppName() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return "gogogo-fullstack-template"
	}
	base := exe
	for i := len(exe) - 1; i >= 0; i-- {
		if exe[i] == '/' {
			base = exe[i+1:]
			break
		}
	}
	if base == "" {
		return "gogogo-fullstack-template"
	}
	return base
}
