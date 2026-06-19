package jsinterp

// Stmt is implemented by all statement nodes.
type Stmt interface{ stmtNode() }

// Expr is implemented by all expression nodes.
type Expr interface{ exprNode() }

// --- Statements ---

// Program is the top-level node.
type Program struct{ Stmts []Stmt }

// VarDecl: var name [= init];
type VarDecl struct {
	Name string
	Init Expr // nil if no initializer
}

// FuncDecl: function name(params) { body }
type FuncDecl struct {
	Name   string
	Params []string
	Body   *Block
}

// Block: { stmts }
type Block struct{ Stmts []Stmt }

// IfStmt: if (cond) then [else els]
type IfStmt struct {
	Cond Expr
	Then *Block
	Else Stmt // *Block, *IfStmt, or nil
}

// WhileStmt: while (cond) body
type WhileStmt struct {
	Cond Expr
	Body *Block
}

// ReturnStmt: return [val];
type ReturnStmt struct{ Val Expr }

// ExprStmt: expr;
type ExprStmt struct{ Expr Expr }

// --- Expressions ---

// NumberLit: 123 / 3.14
type NumberLit struct{ Val float64 }

// BoolLit: true / false
type BoolLit struct{ Val bool }

// NullLit: null
type NullLit struct{}

// Ident: variable reference
type Ident struct{ Name string }

// Assign: name = val
type Assign struct {
	Name string
	Val  Expr
}

// BinOp: left op right
type BinOp struct {
	Op          string
	Left, Right Expr
}

// UnaryOp: op expr
type UnaryOp struct {
	Op   string
	Expr Expr
}

// Call: name(args)
type Call struct {
	Func string
	Args []Expr
}

// marker methods
func (*VarDecl) stmtNode()    {}
func (*FuncDecl) stmtNode()   {}
func (*Block) stmtNode()      {}
func (*IfStmt) stmtNode()     {}
func (*WhileStmt) stmtNode()  {}
func (*ReturnStmt) stmtNode() {}
func (*ExprStmt) stmtNode()   {}

func (*NumberLit) exprNode() {}
func (*BoolLit) exprNode()   {}
func (*NullLit) exprNode()   {}
func (*Ident) exprNode()     {}
func (*Assign) exprNode()    {}
func (*BinOp) exprNode()     {}
func (*UnaryOp) exprNode()   {}
func (*Call) exprNode()      {}
