package config

import (
	"os"
	"strings"
	"swaves/internal/platform/logger"
)

type AppEnvironment string

const (
	envProd AppEnvironment = "prod"
	envTest AppEnvironment = "test"
	envDev  AppEnvironment = "dev"
)

var (
	SeeApiToken        = os.Getenv("SWAVES_SEE_API_TOKEN")
	ImagekitPrivateKey = os.Getenv("SWAVES_IMAGEKIT_PRIVATE_KEY")
	S3Endpoint         = os.Getenv("SWAVES_S3_ENDPOINT")
	S3AccessKeyID      = os.Getenv("SWAVES_S3_ACCESS_KEY_ID")
	S3SecretAccessKey  = os.Getenv("SWAVES_S3_SECRET_ACCESS_KEY")
)

var (
	AppEnv = readAppEnv("SWAVES_ENV")

	IsProduction    = EnvIs(envProd)
	IsNotProduction = EnvIsNot(envProd)
	IsTesting       = EnvIs(envTest)
	IsDevelopment   = EnvIs(envDev)

	TemplateReload        = EnvIsNot(envProd)
	EnableSQLLog          = EnvIs(envDev)
	SessionCookieSecure   = EnvIs(envProd)
	SessionCookieSameSite = "Lax"
)

func readAppEnv(name string) AppEnvironment {
	raw := normalizeAppEnv(os.Getenv(name))
	switch raw {
	case "", string(envProd):
		return envProd
	case string(envTest):
		return envTest
	case string(envDev):
		return envDev
	default:
		logger.Warn("invalid app environment %q, defaulting to production", raw)
		return envProd
	}
}

func EnvIs(env AppEnvironment) bool {
	return AppEnv == env
}

func EnvIsNot(env AppEnvironment) bool {
	return AppEnv != env
}

func normalizeAppEnv(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}
