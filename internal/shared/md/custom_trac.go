package md

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

// 自定义主题，基于 trac，但去掉背景色
var SwavesTrac = styles.Register(chroma.MustNewStyle("swaves-trac",
	chroma.StyleEntries{
		chroma.Error:              "#a61717",
		chroma.Background:         "", // 不设置背景
		chroma.Keyword:            "bold",
		chroma.KeywordType:        "#445588",
		chroma.NameAttribute:      "#008080",
		chroma.NameBuiltin:        "#999999",
		chroma.NameClass:          "bold #445588",
		chroma.NameConstant:       "#008080",
		chroma.NameEntity:         "#800080",
		chroma.NameException:      "bold #990000",
		chroma.NameFunction:       "bold #990000",
		chroma.NameNamespace:      "#555555",
		chroma.NameTag:            "#000080",
		chroma.NameVariable:       "#008080",
		chroma.LiteralString:      "#bb8844",
		chroma.LiteralStringRegex: "#808000",
		chroma.LiteralNumber:      "#009999",
		chroma.Operator:           "bold",
		chroma.Comment:            "italic #999988",
		chroma.CommentSpecial:     "bold #999999",
		chroma.CommentPreproc:     "bold noitalic #999999",
		chroma.GenericDeleted:     "#000000",
		chroma.GenericEmph:        "italic",
		chroma.GenericError:       "#aa0000",
		chroma.GenericHeading:     "#999999",
		chroma.GenericInserted:    "#000000",
		chroma.GenericOutput:      "#888888",
		chroma.GenericPrompt:      "#555555",
		chroma.GenericStrong:      "bold",
		chroma.GenericSubheading:  "#aaaaaa",
		chroma.GenericTraceback:   "#aa0000",
		chroma.GenericUnderline:   "underline",
		chroma.TextWhitespace:     "#bbbbbb",
	},
))
