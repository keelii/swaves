package consts

import (
	"os"
	"strings"
)

var (
	SeeApiToken        = os.Getenv("SWAVES_SEE_API_TOKEN")
	ImagekitPrivateKey = os.Getenv("SWAVES_IMAGEKIT_PRIVATE_KEY")
	S3Endpoint         = os.Getenv("SWAVES_S3_ENDPOINT")
	S3AccessKeyID      = os.Getenv("SWAVES_S3_ACCESS_KEY_ID")
	S3SecretAccessKey  = os.Getenv("SWAVES_S3_SECRET_ACCESS_KEY")
	TemplateReload     = readBoolEnv("SWAVES_TEMPLATE_RELOAD")
)

func readBoolEnv(name string) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
