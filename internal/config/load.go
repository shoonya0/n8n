package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	name     string = "config"
	fileType string = "yaml"
	path     string = "./configs"
)

func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("../../configs")

	if env := v.GetString("app.env"); env != "" {
		v.SetConfigName("config." + env)
		_ = v.MergeInConfig()
		v.SetConfigName("config")
	}

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		var nf viper.ConfigFileNotFoundError
		if errors.As(err, &nf) {
			return nil, fmt.Errorf("config: read error: %w", err)
		}
	}

	setDefaults(v)

	var c Config

	if err := v.UnmarshalExact(&c); err != nil {
		fmt.Println(err)
		return nil, fmt.Errorf("config: unmarshal error: %w", err)
	}

	if c.App.Env == "" {
		c.App.Env = "development"
	}

	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("config: validate error: %w", err)
	}

	return &c, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.env", "development")
	v.SetDefault("app.logLevel", "info")
	v.SetDefault("app.logFormat", "text")
	v.SetDefault("app.service", "n8n-server")

	v.SetDefault("db.maxIdleConns", 5)
	v.SetDefault("db.maxOpenConns", 25)
	v.SetDefault("db.connMaxLifetime", 30*time.Minute)

	v.SetDefault("jwt.accessTTL", "15m")
	v.SetDefault("jwt.refreshTTL", "720h")
}

func (c *Config) validate() error {
	var errs []string

	if c.App.Env == "" {
		errs = append(errs, "app.env is required")
	}

	if c.App.LogLevel == "" {
		errs = append(errs, "app.logLevel is required")
	}

	if c.App.LogFormat == "" {
		errs = append(errs, "app.logFormat is required")
	}

	if c.App.Service == "" {
		errs = append(errs, "app.service is required")
	}

	if c.DB.DNS == "" {
		errs = append(errs, "db.dns is required")
	}

	if c.DB.MaxIdleConns == 0 {
		errs = append(errs, "db.maxIdleConns is required")
	}

	if c.DB.MaxOpenConns == 0 {
		errs = append(errs, "db.maxOpenConns is required")
	}

	if c.DB.ConnMaxLifetime == 0 {
		errs = append(errs, "db.connMaxLifetime is required")
	}

	if c.JWT.AccessTTL == 0 {
		errs = append(errs, "jwt.accessTTL is required")
	}

	if c.JWT.RefreshTTL == 0 {
		errs = append(errs, "jwt.refreshTTL is required")
	}

	if c.JWT.Issuer == "" {
		errs = append(errs, "jwt.issuer is required")
	}

	if c.JWT.Audience == "" {
		errs = append(errs, "jwt.audience is required")
	}

	if len(errs) > 0 {
		return fmt.Errorf("config: validate error: %s", strings.Join(errs, ", "))
	}

	return nil
}
