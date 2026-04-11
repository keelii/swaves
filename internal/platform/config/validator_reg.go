package config

import (
	"encoding/json"
	"regexp"
)

func jsonStringify(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

var PostUrlPrefixRegexp = regexp.MustCompile(`^(\{datetime\}|[a-z]*)$`)

var PostUrlPrefixValidator = map[string]interface{}{
	"title":   "只能是{datetime}或小写英文字母",
	"pattern": PostUrlPrefixRegexp.String(),
}
var PostUrlPrefixValidatorJSON = jsonStringify(PostUrlPrefixValidator)

var UrlPrefixRegexp = regexp.MustCompile(`^[a-z]*$`)

var UrlPrefixValidator = map[string]interface{}{
	"title":   "只能是小写英文字母",
	"pattern": UrlPrefixRegexp.String(),
}
var UrlPrefixValidatorJSON = jsonStringify(UrlPrefixValidator)

var DashPathRegexp = regexp.MustCompile(`^/?([a-z]+(?:/[a-z]+)*)?$`)

var DashPathValidator = map[string]interface{}{
	"title":   "只能是 / 和小写英文字母路径",
	"pattern": DashPathRegexp.String(),
}

var DashPathValidatorJSON = jsonStringify(DashPathValidator)

var UrlFileNamePrefixRegexp = regexp.MustCompile(`^[a-z]+[0-9]*\.?[a-z]+$`)

var UrlFileNamePrefixValidator = map[string]interface{}{
	"title":    "只能是文件名",
	"required": true,
	"pattern":  UrlFileNamePrefixRegexp.String(),
}
var UrlFileNamePrefixValidatorJSON = jsonStringify(UrlFileNamePrefixValidator)

var PostUrlExtRegexp = regexp.MustCompile(`^\.[a-z]+$`)

var PostUrlExtValidator = map[string]interface{}{
	"title":    "只能是.，+小写英文字母",
	"required": false,
	"pattern":  PostUrlExtRegexp.String(),
}
var PostUrlExtValidatorJSON = jsonStringify(PostUrlExtValidator)

//var PostUrlNameRegexp = regexp.MustCompile(`^\{slug\}|\{id\}|\{title\}$`)
//
//var PostUrlNameValidator = map[string]interface{}{
//	"title":    "只能是{slug}，{id}，{title}",
//	"required": true,
//	"pattern":  PostUrlNameRegexp.String(),
//}
//var PostUrlNameValidatorJSON = jsonStringify(PostUrlNameValidator)

var DashPasswordValidator = CondProduction(map[string]interface{}{
	"minlength": 6,
}, map[string]interface{}{})
var DashPasswordValidatorJSON = jsonStringify(DashPasswordValidator)

const (
	DashNavWidthMin  = 100
	DashNavWidthMax  = 480
	DashNavWidthStep = 5
)

var DashNavWidthValidator = map[string]interface{}{
	"min":  DashNavWidthMin,
	"max":  DashNavWidthMax,
	"step": DashNavWidthStep,
}

var DashNavWidthValidatorJSON = jsonStringify(DashNavWidthValidator)
