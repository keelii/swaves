package jsinterp

import (
	"fmt"
	"unicode"
)

type TokenType int

const (
	TNumber   TokenType = iota // 123, 3.14
	TTrue                      // true
	TFalse                     // false
	TNull                      // null
	TIdent                     // identifier
	TVar                       // var
	TIf                        // if
	TElse                      // else
	TWhile                     // while
	TFunction                  // function
	TReturn                    // return
	TPlus                      // +
	TMinus                     // -
	TStar                      // *
	TSlash                     // /
	TPercent                   // %
	TBang                      // !
	TEq                        // =
	TEqEq                      // ==
	TBangEq                    // !=
	TLt                        // <
	TLtEq                      // <=
	TGt                        // >
	TGtEq                      // >=
	TAmpAmp                    // &&
	TPipePipe                  // ||
	TLParen                    // (
	TRParen                    // )
	TLBrace                    // {
	TRBrace                    // }
	TSemi                      // ;
	TComma                     // ,
	TEOF
)

var keywords = map[string]TokenType{
	"var":      TVar,
	"if":       TIf,
	"else":     TElse,
	"while":    TWhile,
	"function": TFunction,
	"return":   TReturn,
	"true":     TTrue,
	"false":    TFalse,
	"null":     TNull,
}

// Token is a single lexical unit.
type Token struct {
	Type TokenType
	Text string
	Line int
}

type lexer struct {
	src  []rune
	pos  int
	line int
}

func newLexer(src string) *lexer {
	return &lexer{src: []rune(src), line: 1}
}

func (l *lexer) peek() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *lexer) next() rune {
	ch := l.src[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
	}
	return ch
}

func (l *lexer) skipWS() {
	for l.pos < len(l.src) {
		ch := l.peek()
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			l.next()
		} else if ch == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
			for l.pos < len(l.src) && l.peek() != '\n' {
				l.next()
			}
		} else {
			break
		}
	}
}

func (l *lexer) readNumber() Token {
	start, line := l.pos, l.line
	for l.pos < len(l.src) && (unicode.IsDigit(l.peek()) || l.peek() == '.') {
		l.next()
	}
	return Token{TNumber, string(l.src[start:l.pos]), line}
}

func (l *lexer) readIdent() Token {
	start, line := l.pos, l.line
	for l.pos < len(l.src) && (unicode.IsLetter(l.peek()) || unicode.IsDigit(l.peek()) || l.peek() == '_') {
		l.next()
	}
	text := string(l.src[start:l.pos])
	tt, ok := keywords[text]
	if !ok {
		tt = TIdent
	}
	return Token{tt, text, line}
}

func (l *lexer) nextToken() Token {
	l.skipWS()
	if l.pos >= len(l.src) {
		return Token{TEOF, "", l.line}
	}
	line := l.line
	ch := l.next()
	switch ch {
	case '+':
		return Token{TPlus, "+", line}
	case '-':
		return Token{TMinus, "-", line}
	case '*':
		return Token{TStar, "*", line}
	case '/':
		return Token{TSlash, "/", line}
	case '%':
		return Token{TPercent, "%", line}
	case '(':
		return Token{TLParen, "(", line}
	case ')':
		return Token{TRParen, ")", line}
	case '{':
		return Token{TLBrace, "{", line}
	case '}':
		return Token{TRBrace, "}", line}
	case ';':
		return Token{TSemi, ";", line}
	case ',':
		return Token{TComma, ",", line}
	case '!':
		if l.peek() == '=' {
			l.next()
			return Token{TBangEq, "!=", line}
		}
		return Token{TBang, "!", line}
	case '=':
		if l.peek() == '=' {
			l.next()
			return Token{TEqEq, "==", line}
		}
		return Token{TEq, "=", line}
	case '<':
		if l.peek() == '=' {
			l.next()
			return Token{TLtEq, "<=", line}
		}
		return Token{TLt, "<", line}
	case '>':
		if l.peek() == '=' {
			l.next()
			return Token{TGtEq, ">=", line}
		}
		return Token{TGt, ">", line}
	case '&':
		if l.peek() == '&' {
			l.next()
			return Token{TAmpAmp, "&&", line}
		}
		panic(fmt.Sprintf("line %d: unexpected char '&'", line))
	case '|':
		if l.peek() == '|' {
			l.next()
			return Token{TPipePipe, "||", line}
		}
		panic(fmt.Sprintf("line %d: unexpected char '|'", line))
	default:
		if unicode.IsDigit(ch) {
			l.pos--
			return l.readNumber()
		}
		if unicode.IsLetter(ch) || ch == '_' {
			l.pos--
			return l.readIdent()
		}
		panic(fmt.Sprintf("line %d: unexpected char %q", line, ch))
	}
}

func tokenize(src string) []Token {
	l := newLexer(src)
	var tokens []Token
	for {
		t := l.nextToken()
		tokens = append(tokens, t)
		if t.Type == TEOF {
			break
		}
	}
	return tokens
}
