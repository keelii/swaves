package consts

import (
	"regexp"
	"swaves/helper"
)

var PostUrlPrefixRegexp = regexp.MustCompile(`^(\{datetime\}|[a-z]*)$`)

var PostUrlPrefixValidator = map[string]interface{}{
	"title":   "只能是{datetime}或小写英文字母",
	"pattern": PostUrlPrefixRegexp.String(),
}
var PostUrlPrefixValidatorJSON = helper.JSONStringify(PostUrlPrefixValidator)

var UrlPrefixRegexp = regexp.MustCompile(`^[a-z]*$`)

var UrlPrefixValidator = map[string]interface{}{
	"title":   "只能是小写英文字母",
	"pattern": UrlPrefixRegexp.String(),
}
var UrlPrefixValidatorJSON = helper.JSONStringify(UrlPrefixValidator)

var UrlFileNamePrefixRegexp = regexp.MustCompile(`^[a-z]+[0-9]*\.?[a-z]+$`)

var UrlFileNamePrefixValidator = map[string]interface{}{
	"title":    "只能是文件名",
	"required": true,
	"pattern":  UrlFileNamePrefixRegexp.String(),
}
var UrlFileNamePrefixValidatorJSON = helper.JSONStringify(UrlFileNamePrefixValidator)

var PostUrlExtRegexp = regexp.MustCompile(`^\.[a-z]+$`)

var PostUrlExtValidator = map[string]interface{}{
	"title":    "只能是.，+小写英文字母",
	"required": false,
	"pattern":  PostUrlExtRegexp.String(),
}
var PostUrlExtValidatorJSON = helper.JSONStringify(PostUrlExtValidator)

//var PostUrlNameRegexp = regexp.MustCompile(`^\{slug\}|\{id\}|\{title\}$`)
//
//var PostUrlNameValidator = map[string]interface{}{
//	"title":    "只能是{slug}，{id}，{title}",
//	"required": true,
//	"pattern":  PostUrlNameRegexp.String(),
//}
//var PostUrlNameValidatorJSON = helper.JSONStringify(PostUrlNameValidator)
