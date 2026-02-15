package consts

import (
	"swaves/helper"
)

const PostUrlPrefixReg = `^/\{datetime\}|/[a-z]*$`

var PostUrlPrefixValidator = map[string]interface{}{
	"title":    "只能是{datetime}，/，/+小写英文字母",
	"required": true,
	"pattern":  PostUrlPrefixReg,
}
var PostUrlPrefixValidatorJSON = helper.JSONStringify(PostUrlPrefixValidator)

const UrlPrefixReg = `^/[a-z]*$`

var UrlPrefixValidator = map[string]interface{}{
	"title":    "只能是/，/+小写英文字母",
	"required": true,
	"pattern":  UrlPrefixReg,
}
var UrlPrefixValidatorJSON = helper.JSONStringify(UrlPrefixValidator)

const UrlFileNamePrefixReg = `^/[a-z]+[0-9]*.?[a-z]+$`

var UrlFileNamePrefixValidator = map[string]interface{}{
	"title":    "只能是/，/+小写英文字母",
	"required": true,
	"pattern":  UrlFileNamePrefixReg,
}
var UrlFileNamePrefixValidatorJSON = helper.JSONStringify(UrlFileNamePrefixValidator)
