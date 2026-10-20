package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct{ Environment, Address, DatabaseURL string }

// Load reads optional local configuration without overriding exported variables.
func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, errors.New("tidak dapat membaca .env")
	}
	return Parse(os.Getenv)
}

func Parse(get func(string) string) (Config, error) {
	env := get("APP_ENV")
	if env == "" {
		env = "development"
	}
	if env != "development" && env != "test" && env != "production" {
		return Config{}, errors.New("APP_ENV harus development, test, atau production")
	}
	host := get("APP_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	if net.ParseIP(host) == nil && host != "localhost" {
		return Config{}, errors.New("APP_HOST harus alamat IP atau localhost")
	}
	port := get("APP_PORT")
	if port == "" {
		port = "8080"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return Config{}, errors.New("APP_PORT harus 1–65535")
	}
	raw := get("DATABASE_URL")
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.Path == "" || u.Path == "/" {
		return Config{}, errors.New("DATABASE_URL harus URL PostgreSQL dengan host dan nama database")
	}
	if env == "production" && u.Query().Get("sslmode") != "verify-full" {
		return Config{}, errors.New("production membutuhkan sslmode=verify-full")
	}
	return Config{Environment: env, Address: net.JoinHostPort(host, port), DatabaseURL: raw}, nil
}
