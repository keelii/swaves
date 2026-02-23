package md

import (
	"strings"
	"unicode"

	"github.com/yuin/goldmark/ast"
)

type UnicodeIDs struct{}

func NewUnicodeIDs() *UnicodeIDs {
	return &UnicodeIDs{}
}

func (u *UnicodeIDs) Generate(v []byte, _ ast.NodeKind) []byte {
	s := strings.TrimSpace(string(v))
	s = strings.ReplaceAll(s, " ", "-")

	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			b.WriteRune(r)
		}
	}

	id := b.String()
	if id == "" {
		id = "heading"
	}

	return []byte(id)
}

func (u *UnicodeIDs) Put(value []byte) {}
