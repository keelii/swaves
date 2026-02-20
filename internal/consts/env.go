package consts

import "os"

var (
	SeeApiToken        = os.Getenv("SWV_SEE_API_TOKEN")
	ImagekitPrivateKey = os.Getenv("SWV_IMAGEKIT_PRIVATE_KEY")
	S3AccessKeyID      = os.Getenv("SWAVES_S3_ACCESS_KEY_ID")
	S3SecretAccessKey  = os.Getenv("SWAVES_S3_SECRET_ACCESS_KEY")
)
