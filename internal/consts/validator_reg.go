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

const PostUrlExtReg = `^\.[a-z]+$`

var PostUrlExtValidator = map[string]interface{}{
	"title":    "只能是.，+小写英文字母",
	"required": false,
	"pattern":  PostUrlExtReg,
}
var PostUrlExtValidatorJSON = helper.JSONStringify(PostUrlExtValidator)

const PostUrlNameReg = `^\{slug\}|\{id\}|\{title\}$`

var PostUrlNameValidator = map[string]interface{}{
	"title":    "只能是{slug}，{id}，{title}",
	"required": true,
	"pattern":  PostUrlNameReg,
}
var PostUrlNameValidatorJSON = helper.JSONStringify(PostUrlNameValidator)
