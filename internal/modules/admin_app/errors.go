package admin_app

import (
	"errors"
	"fmt"
)

func errSlugInvalid(code string, slug string) error {
	if slug == "" {
		return errors.New(fmt.Sprintf("[%s] slug 格式不合法，仅允许小写字母、数字、连字符、下划线，且以字母或数字开头", code))
	}
	return errors.New(fmt.Sprintf("[%s] slug 格式不合法: %s，仅允许小写字母、数字、连字符、下划线，且以字母或数字开头", code, slug))
}
