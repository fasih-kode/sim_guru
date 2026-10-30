package config

import (
	"encoding/hex"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Environment, Address, DatabaseURL                          string
	TeacherPhotoDirectory, StaffPhotoDirectory, RepositoryRoot string
	TrustedProxyCIDRs                                          []netip.Prefix
	CSRFKey                                                    []byte
}

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
		port = "18743"
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
	key, err := hex.DecodeString(get("CSRF_AUTH_KEY"))
	if err != nil || len(key) != 32 {
		return Config{}, errors.New("CSRF_AUTH_KEY wajib berupa 64 digit heksadesimal acak")
	}
	teacherPhotoDirectory, staffPhotoDirectory, repositoryRoot := get("TEACHER_PHOTO_DIR"), get("STAFF_PHOTO_DIR"), get("APP_REPOSITORY_ROOT")
	if (teacherPhotoDirectory != "" || staffPhotoDirectory != "") && (!filepath.IsAbs(repositoryRoot) || (teacherPhotoDirectory != "" && !filepath.IsAbs(teacherPhotoDirectory)) || (staffPhotoDirectory != "" && !filepath.IsAbs(staffPhotoDirectory))) {
		return Config{}, errors.New("APP_REPOSITORY_ROOT dan direktori foto yang diaktifkan wajib berupa path absolut")
	}
	trustedProxyCIDRs, err := parseTrustedProxyCIDRs(get("APP_TRUSTED_PROXY_CIDRS"))
	if err != nil {
		return Config{}, err
	}
	return Config{Environment: env, Address: net.JoinHostPort(host, port), DatabaseURL: raw, CSRFKey: key,
		TeacherPhotoDirectory: teacherPhotoDirectory, StaffPhotoDirectory: staffPhotoDirectory, RepositoryRoot: repositoryRoot, TrustedProxyCIDRs: trustedProxyCIDRs}, nil
}

func parseTrustedProxyCIDRs(value string) ([]netip.Prefix, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	prefixes := make([]netip.Prefix, 0)
	for _, item := range strings.Split(value, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err != nil {
			return nil, errors.New("APP_TRUSTED_PROXY_CIDRS harus berisi CIDR yang valid")
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}
