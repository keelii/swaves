package jsinterp

import (
	"fmt"
	"strconv"
)

type parser struct {
	tokens []Token
	pos    int
}

// Parse tokenizes src and returns the parsed program, or a non-nil error.
func Parse(src string) (prog *Program, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("parse error: %v", r)
		}
	}()
	p := &parser{tokens: tokenize(src)}
	prog = p.parseProgram()
	return
}

func (p *parser) peek() Token        { return p.tokens[p.pos] }
func (p *parser) check(tt TokenType) bool { return p.peek().Type == tt }

func (p *parser) advance() Token {
	t := p.tokens[p.pos]
	if t.Type != TEOF {
		p.pos++
	}
	return t
}

func (p *parser) eat(tt TokenType) Token {
	t := p.peek()
	if t.Type != tt {
		panic(fmt.Sprintf("line %d: expected %d, got %q", t.Line, tt, t.Text))
	}
	return p.advance()
}

func (p *parser) match(types ...TokenType) bool {
	for _, tt := range types {
		if p.check(tt) {
			return true
		}
	}
	return false
}

func (p *parser) parseProgram() *Program {
	prog := &Program{}
	for !p.check(TEOF) {
		prog.Stmts = append(prog.Stmts, p.parseStmt())
	}
	return prog
}

func (p *parser) parseStmt() Stmt {
	switch p.peek().Type {
	case TVar:
		return p.parseVarDecl()
	case TFunction:
		return p.parseFuncDecl()
	case TIf:
		return p.parseIf()
	case TWhile:
		return p.parseWhile()
	case TReturn:
		return p.parseReturn()
	case TLBrace:
		return p.parseBlock()
	default:
		return p.parseExprStmt()
	}
}

func (p *parser) parseVarDecl() *VarDecl {
	p.eat(TVar)
	name := p.eat(TIdent).Text
	var init Expr
	if p.check(TEq) {
		p.advance()
		init = p.parseExpr()
	}
	p.eatSemi()
	return &VarDecl{Name: name, Init: init}
}

func (p *parser) parseFuncDecl() *FuncDecl {
	p.eat(TFunction)
	name := p.eat(TIdent).Text
	p.eat(TLParen)
	var params []string
	for !p.check(TRParen) {
		params = append(params, p.eat(TIdent).Text)
		if !p.check(TRParen) {
			p.eat(TComma)
		}
	}
	p.eat(TRParen)
	return &FuncDecl{Name: name, Params: params, Body: p.parseBlock()}
}

func (p *parser) parseIf() *IfStmt {
	p.eat(TIf)
	p.eat(TLParen)
	cond := p.parseExpr()
	p.eat(TRParen)
	then := p.parseBlock()
	var els Stmt
	if p.check(TElse) {
		p.advance()
		if p.check(TIf) {
			els = p.parseIf()
		} else {
			els = p.parseBlock()
		}
	}
	return &IfStmt{Cond: cond, Then: then, Else: els}
}

func (p *parser) parseWhile() *WhileStmt {
	p.eat(TWhile)
	p.eat(TLParen)
	cond := p.parseExpr()
	p.eat(TRParen)
	return &WhileStmt{Cond: cond, Body: p.parseBlock()}
}

func (p *parser) parseReturn() *ReturnStmt {
	p.eat(TReturn)
	var val Expr
	if !p.check(TSemi) && !p.check(TRBrace) && !p.check(TEOF) {
		val = p.parseExpr()
	}
	p.eatSemi()
	return &ReturnStmt{Val: val}
}

func (p *parser) parseBlock() *Block {
	p.eat(TLBrace)
	b := &Block{}
	for !p.check(TRBrace) && !p.check(TEOF) {
		b.Stmts = append(b.Stmts, p.parseStmt())
	}
	p.eat(TRBrace)
	return b
}

func (p *parser) parseExprStmt() *ExprStmt {
	e := p.parseExpr()
	p.eatSemi()
	return &ExprStmt{Expr: e}
}

func (p *parser) eatSemi() {
	if p.check(TSemi) {
		p.advance()
	}
}

// Expression parsing via precedence climbing.

func (p *parser) parseExpr() Expr   { return p.parseAssign() }

func (p *parser) parseAssign() Expr {
	left := p.parseOr()
	if p.check(TEq) {
		p.advance()
		val := p.parseAssign()
		id, ok := left.(*Ident)
		if !ok {
			panic("invalid assignment target")
		}
		return &Assign{Name: id.Name, Val: val}
	}
	return left
}

func (p *parser) parseOr() Expr {
	left := p.parseAnd()
	for p.check(TPipePipe) {
		op := p.advance().Text
		left = &BinOp{Op: op, Left: left, Right: p.parseAnd()}
	}
	return left
}

func (p *parser) parseAnd() Expr {
	left := p.parseEquality()
	for p.check(TAmpAmp) {
		op := p.advance().Text
		left = &BinOp{Op: op, Left: left, Right: p.parseEquality()}
	}
	return left
}

func (p *parser) parseEquality() Expr {
	left := p.parseComparison()
	for p.match(TEqEq, TBangEq) {
		op := p.advance().Text
		left = &BinOp{Op: op, Left: left, Right: p.parseComparison()}
	}
	return left
}

func (p *parser) parseComparison() Expr {
	left := p.parseAdditive()
	for p.match(TLt, TLtEq, TGt, TGtEq) {
		op := p.advance().Text
		left = &BinOp{Op: op, Left: left, Right: p.parseAdditive()}
	}
	return left
}

func (p *parser) parseAdditive() Expr {
	left := p.parseMultiplicative()
	for p.match(TPlus, TMinus) {
		op := p.advance().Text
		left = &BinOp{Op: op, Left: left, Right: p.parseMultiplicative()}
	}
	return left
}

func (p *parser) parseMultiplicative() Expr {
	left := p.parseUnary()
	for p.match(TStar, TSlash, TPercent) {
		op := p.advance().Text
		left = &BinOp{Op: op, Left: left, Right: p.parseUnary()}
	}
	return left
}

func (p *parser) parseUnary() Expr {
	if p.match(TBang, TMinus) {
		op := p.advance().Text
		return &UnaryOp{Op: op, Expr: p.parseUnary()}
	}
	return p.parseCall()
}

func (p *parser) parseCall() Expr {
	expr := p.parsePrimary()
	if p.check(TLParen) {
		id, ok := expr.(*Ident)
		if !ok {
			panic("only named function calls are supported")
		}
		p.advance()
		var args []Expr
		for !p.check(TRParen) {
			args = append(args, p.parseExpr())
			if !p.check(TRParen) {
				p.eat(TComma)
			}
		}
		p.eat(TRParen)
		return &Call{Func: id.Name, Args: args}
	}
	return expr
}

func (p *parser) parsePrimary() Expr {
	t := p.peek()
	switch t.Type {
	case TNumber:
		p.advance()
		v, err := strconv.ParseFloat(t.Text, 64)
		if err != nil {
			panic(fmt.Sprintf("line %d: bad number %q", t.Line, t.Text))
		}
		return &NumberLit{Val: v}
	case TTrue:
		p.advance()
		return &BoolLit{Val: true}
	case TFalse:
		p.advance()
		return &BoolLit{Val: false}
	case TNull:
		p.advance()
		return &NullLit{}
	case TIdent:
		p.advance()
		return &Ident{Name: t.Text}
	case TLParen:
		p.advance()
		e := p.parseExpr()
		p.eat(TRParen)
		return e
	default:
		panic(fmt.Sprintf("line %d: unexpected token %q", t.Line, t.Text))
	}
}
