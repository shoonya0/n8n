package config

import "time"

type Config struct {
	App AppConfig
	DB  DBConfig
	JWT JWTConfig
}

type AppConfig struct {
	Env       string // development | staging | production
	LogLevel  string // debug | info | warn | error
	LogFormat string // json | text | "" (auto: text in development, json otherwise)
	Service   string // "buckspe-backend"
}

type DBConfig struct {
	DNS             string
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
}

type JWTConfig struct {
	Alg            string // RS256
	PrivateKeyFile string
	PublicKeyFile  string
	AccessTTL      time.Duration
	RefreshTTL     time.Duration
	Issuer         string
	Audience       string
}
