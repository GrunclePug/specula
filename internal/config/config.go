package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all the configuration parameters for the application.
type Config struct {
	Addr         string
	PingInterval time.Duration
	PingTimeout  time.Duration
	Targets      []string
	DBPath       string
	PingCount    int
	JSONLog      bool

	// InfluxDB settings
	InfluxURL    string
	InfluxToken  string
	InfluxOrg    string
	InfluxBucket string
}

// Load loads the configuration from the environment and optional .env file.
func Load() (*Config, error) {
	// Attempt to load .env file. We don't fail if it's missing,
	// as environment variables can be set directly.
	_ = godotenv.Load()

	addr := getEnv("SPECULA_ADDR", ":8080")
	intervalStr := getEnv("SPECULA_PING_INTERVAL", "5s")
	timeoutStr := getEnv("SPECULA_PING_TIMEOUT", "2s")
	targetsStr := getEnv("SPECULA_TARGETS", "google.com,github.com,cloudflare.com,8.8.8.8")
	dbPath := getEnv("SPECULA_DB_PATH", "specula.db")
	pingCountStr := getEnv("SPECULA_PING_COUNT", "5")
	jsonLogStr := getEnv("SPECULA_JSON_LOG", "false")

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		interval = 5 * time.Second
	}

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		timeout = 2 * time.Second
	}

	pingCount, err := strconv.Atoi(pingCountStr)
	if err != nil || pingCount <= 0 {
		pingCount = 5
	}

	jsonLog, _ := strconv.ParseBool(jsonLogStr)

	var targets []string
	for _, t := range strings.Split(targetsStr, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			targets = append(targets, t)
		}
	}

	return &Config{
		Addr:         addr,
		PingInterval: interval,
		PingTimeout:  timeout,
		Targets:      targets,
		DBPath:       dbPath,
		PingCount:    pingCount,
		JSONLog:      jsonLog,
		InfluxURL:    getEnv("INFLUX_URL", ""),
		InfluxToken:  getEnv("INFLUX_TOKEN", ""),
		InfluxOrg:    getEnv("INFLUX_ORG", ""),
		InfluxBucket: getEnv("INFLUX_BUCKET", ""),
	}, nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

