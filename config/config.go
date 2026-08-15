package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	server Servercfg
	db     DBcfg
	smtp   SMTPcfg
}

type Servercfg struct {
	port int
	host string
}
type DBcfg struct {
	user string
	pass string
	host string
}
type SMTPcfg struct {
	server string
	passwd string
}
type loader struct {
	errs  []error
	warns []string
}

func (l *loader) fail(format string, args ...any) {
	l.errs = append(l.errs, fmt.Errorf(format, args...))
}
func (l *loader) warn(format string, args ...any) {
	l.warns = append(l.warns, fmt.Sprintf(format, args...))
}

// ANCHOR Checkers
func (l *loader) optIP(key string, def net.IP) net.IP {
	v, exists := os.LookupEnv(key)
	if !exists || v == "" {
		l.warn("Optional value %s not set, falling back to default value: %s", key, def)
		return def
	}
	ip := net.ParseIP(v)
	if ip == nil {
		l.fail("%s=%q is not a valid IP address", key, v)
		return def
	}
	return ip
}

func (l *loader) requireIP(key string) net.IP {
	v, exists := os.LookupEnv(key)
	if !exists || v == "" {
		l.fail("%s required, but not set", key)
		return nil
	}
	ip := net.ParseIP(v)
	if ip == nil {
		l.fail("%s=%q is not a valid IP address", key, v)
		return nil
	}
	return ip
}

func (l *loader) requireString(key string) string {
	v, exists := os.LookupEnv(key)
	if !exists || v == "" {
		l.fail("%s required, but not set", key)
		return ""
	}
	return v
}

func (l *loader) optString(key string, def string) string {
	v, exists := os.LookupEnv(key)
	if !exists || v == "" {
		l.warn("%s Optional, but not set. Defaulting to %s", key, def)
		return def
	}
	return v
}

func (l *loader) intInRange(key string, def int, lo int, hi int) int { //inclusive of low/hi values
	v, exists := os.LookupEnv(key)
	if !exists || v == "" {
		l.warn(" %s not set, using default %d", key, def)
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64) // parse from v, base 10, size 64 bits
	if err != nil {
		l.fail("Invalid integer: %s=%q.", key, v)
		return def
	}
	i := int(n)
	if !(i <= hi && i >= lo) {
		l.fail("%s=%d not in valid range [%d, %d]", key, i, lo, hi)
		return def
	}

	return i
}

func (l *loader) boolean(key string, def bool) bool {
	v, exists := os.LookupEnv(key)
	if !exists || v == "" {
		l.warn("Bool %s not set, using default %s", key, strconv.FormatBool(def))
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.fail("%s=%q is not a valid boolean", key, v)
		return def
	}
	return b
}

func (l *loader) duration(key string, def time.Duration) time.Duration {
	v, exists := os.LookupEnv(key)
	if !exists || v == "" {
		l.warn("%s not set, using default %s", key, def)
		return def
	}
	t, err := time.ParseDuration(v)
	if err != nil {
		l.fail("%s=%q is not a valid duration", key, v)
		return def
	}
	return t

}

// ANCHOR Loader
func Load(ENVPATH string) (Config, []string, error) {
	l := &loader{}
	err := godotenv.Load(ENVPATH)
	if err != nil {
		l.fail("Error loading .env from path %s", ENVPATH)
	}
	cfg := Config{
		server: Servercfg{
			port: l.intInRange("PORT", 8080, 1, 65535),
			host: l.optString("HOST", "127.0.0.1"),
		},
		db: DBcfg{
			user: l.requireString("DB_USER"),
			pass: l.requireString("DB_PASS"),
			host: l.requireString("DB_HOST"),
		},
		smtp: SMTPcfg{
			server: l.requireString("SMTP_SERVER"),
			passwd: l.requireString("SMTP_PASS"),
		},
	}
	if err := errors.Join(l.errs...); err != nil {
		return Config{}, l.warns, err
	}

	return cfg, l.warns, nil
}

// ANCHOR Getters
func (c Config) ServerPort() int    { return c.server.port }
func (c Config) ServerHost() string { return c.server.host }

func (c Config) DBUser() string { return c.db.user }
func (c Config) DBPass() string { return c.db.pass }
func (c Config) DBIP() string   { return c.db.host }

func (c Config) SMTPServer() string { return c.smtp.server }
func (c Config) SMTPPass() string   { return c.smtp.passwd }
