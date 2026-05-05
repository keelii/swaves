package jsinterp

import (
	"fmt"
	"math"
	"strings"
)

// Value is float64, bool, or nil (null).
type Value = interface{}

// returnSignal is used to unwind the call stack on a return statement.
type returnSignal struct{ val Value }

// Interpreter runs a parsed program.
type Interpreter struct {
	globals map[string]Value
	funcs   map[string]*FuncDecl
	// Output is called once per print() with the formatted line (no newline).
	Output func(string)
}

// New returns an Interpreter that writes to stdout.
func New() *Interpreter {
	interp := &Interpreter{
		globals: make(map[string]Value),
		funcs:   make(map[string]*FuncDecl),
	}
	interp.Output = func(s string) { fmt.Println(s) }
	return interp
}

// Run executes prog and returns any runtime error.
func (interp *Interpreter) Run(prog *Program) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(returnSignal); ok {
				return // top-level return is a no-op
			}
			err = fmt.Errorf("%v", r)
		}
	}()
	for _, s := range prog.Stmts {
		interp.execStmt(s, nil)
	}
	return
}

// env: nil = global scope; non-nil = function-local scope.
func (interp *Interpreter) execStmt(s Stmt, env map[string]Value) {
	switch n := s.(type) {
	case *VarDecl:
		var v Value
		if n.Init != nil {
			v = interp.evalExpr(n.Init, env)
		}
		interp.setVar(n.Name, v, env)
	case *FuncDecl:
		interp.funcs[n.Name] = n
	case *Block:
		for _, stmt := range n.Stmts {
			interp.execStmt(stmt, env)
		}
	case *IfStmt:
		if isTruthy(interp.evalExpr(n.Cond, env)) {
			interp.execBlock(n.Then, env)
		} else if n.Else != nil {
			interp.execStmt(n.Else, env)
		}
	case *WhileStmt:
		for isTruthy(interp.evalExpr(n.Cond, env)) {
			interp.execBlock(n.Body, env)
		}
	case *ReturnStmt:
		var v Value
		if n.Val != nil {
			v = interp.evalExpr(n.Val, env)
		}
		panic(returnSignal{v})
	case *ExprStmt:
		interp.evalExpr(n.Expr, env)
	default:
		panic(fmt.Sprintf("unknown stmt type %T", s))
	}
}

func (interp *Interpreter) execBlock(b *Block, env map[string]Value) {
	for _, s := range b.Stmts {
		interp.execStmt(s, env)
	}
}

func (interp *Interpreter) setVar(name string, v Value, env map[string]Value) {
	if env != nil {
		env[name] = v
	} else {
		interp.globals[name] = v
	}
}

func (interp *Interpreter) getVar(name string, env map[string]Value) Value {
	if env != nil {
		if v, ok := env[name]; ok {
			return v
		}
	}
	if v, ok := interp.globals[name]; ok {
		return v
	}
	panic(fmt.Sprintf("undefined variable %q", name))
}

func (interp *Interpreter) assignVar(name string, v Value, env map[string]Value) {
	if env != nil {
		if _, ok := env[name]; ok {
			env[name] = v
			return
		}
	}
	if _, ok := interp.globals[name]; ok {
		interp.globals[name] = v
		return
	}
	panic(fmt.Sprintf("undefined variable %q", name))
}

func (interp *Interpreter) evalExpr(e Expr, env map[string]Value) Value {
	switch n := e.(type) {
	case *NumberLit:
		return n.Val
	case *BoolLit:
		return n.Val
	case *NullLit:
		return nil
	case *Ident:
		return interp.getVar(n.Name, env)
	case *Assign:
		v := interp.evalExpr(n.Val, env)
		interp.assignVar(n.Name, v, env)
		return v
	case *BinOp:
		return interp.evalBinOp(n, env)
	case *UnaryOp:
		v := interp.evalExpr(n.Expr, env)
		switch n.Op {
		case "!":
			return !isTruthy(v)
		case "-":
			return -toNumber(v)
		}
	case *Call:
		return interp.callFunc(n, env)
	}
	panic(fmt.Sprintf("unknown expr type %T", e))
}

func (interp *Interpreter) evalBinOp(n *BinOp, env map[string]Value) Value {
	// Short-circuit operators.
	if n.Op == "&&" {
		left := interp.evalExpr(n.Left, env)
		if !isTruthy(left) {
			return left
		}
		return interp.evalExpr(n.Right, env)
	}
	if n.Op == "||" {
		left := interp.evalExpr(n.Left, env)
		if isTruthy(left) {
			return left
		}
		return interp.evalExpr(n.Right, env)
	}

	left := interp.evalExpr(n.Left, env)
	right := interp.evalExpr(n.Right, env)
	switch n.Op {
	case "+":
		return toNumber(left) + toNumber(right)
	case "-":
		return toNumber(left) - toNumber(right)
	case "*":
		return toNumber(left) * toNumber(right)
	case "/":
		r := toNumber(right)
		if r == 0 {
			return math.Inf(1)
		}
		return toNumber(left) / r
	case "%":
		return math.Mod(toNumber(left), toNumber(right))
	case "==":
		return valEqual(left, right)
	case "!=":
		return !valEqual(left, right)
	case "<":
		return toNumber(left) < toNumber(right)
	case "<=":
		return toNumber(left) <= toNumber(right)
	case ">":
		return toNumber(left) > toNumber(right)
	case ">=":
		return toNumber(left) >= toNumber(right)
	}
	panic(fmt.Sprintf("unknown operator %q", n.Op))
}

func (interp *Interpreter) callFunc(n *Call, env map[string]Value) (result Value) {
	// Built-in: print(args...)
	if n.Func == "print" {
		parts := make([]string, len(n.Args))
		for i, a := range n.Args {
			parts[i] = FormatVal(interp.evalExpr(a, env))
		}
		interp.Output(strings.Join(parts, " "))
		return nil
	}

	fd, ok := interp.funcs[n.Func]
	if !ok {
		panic(fmt.Sprintf("undefined function %q", n.Func))
	}
	if len(n.Args) != len(fd.Params) {
		panic(fmt.Sprintf("function %q: expected %d args, got %d", n.Func, len(fd.Params), len(n.Args)))
	}

	// Evaluate arguments in the caller's scope, bind to fresh local env.
	local := make(map[string]Value, len(fd.Params))
	for i, p := range fd.Params {
		local[p] = interp.evalExpr(n.Args[i], env)
	}

	defer func() {
		if r := recover(); r != nil {
			if ret, ok := r.(returnSignal); ok {
				result = ret.val
			} else {
				panic(r)
			}
		}
	}()
	interp.execBlock(fd.Body, local)
	return nil
}

// isTruthy follows JS truthiness: null, false, 0 are falsy.
func isTruthy(v Value) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	if n, ok := v.(float64); ok {
		return n != 0
	}
	return true
}

func toNumber(v Value) float64 {
	if v == nil {
		return 0
	}
	if n, ok := v.(float64); ok {
		return n
	}
	if b, ok := v.(bool); ok {
		if b {
			return 1
		}
		return 0
	}
	panic(fmt.Sprintf("cannot convert %v to number", v))
}

func valEqual(a, b Value) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a == b
}

// FormatVal returns the printable representation of a value.
func FormatVal(v Value) string {
	if v == nil {
		return "null"
	}
	if b, ok := v.(bool); ok {
		if b {
			return "true"
		}
		return "false"
	}
	if n, ok := v.(float64); ok {
		return fmt.Sprintf("%g", n)
	}
	return fmt.Sprintf("%v", v)
}
