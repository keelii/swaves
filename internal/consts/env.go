package consts

import (
	"os"
)

var (
	SeeApiToken        = os.Getenv("SWAVES_SEE_API_TOKEN")
	ImagekitPrivateKey = os.Getenv("SWAVES_IMAGEKIT_PRIVATE_KEY")
	S3Endpoint         = os.Getenv("SWAVES_S3_ENDPOINT")
	S3AccessKeyID      = os.Getenv("SWAVES_S3_ACCESS_KEY_ID")
	S3SecretAccessKey  = os.Getenv("SWAVES_S3_SECRET_ACCESS_KEY")
)
