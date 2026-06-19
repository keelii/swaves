package jsinterp

import (
	"strings"
	"testing"
)

func run(t *testing.T, src string) string {
	t.Helper()
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf strings.Builder
	interp := New()
	interp.Output = func(s string) { buf.WriteString(s + "\n") }
	if err := interp.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return strings.TrimSpace(buf.String())
}

func TestArith(t *testing.T) {
	if got := run(t, `print(2 + 3 * 4);`); got != "14" {
		t.Errorf("got %q, want 14", got)
	}
}

func TestUnary(t *testing.T) {
	if got := run(t, `print(-5 + 3);`); got != "-2" {
		t.Errorf("got %q, want -2", got)
	}
	if got := run(t, `print(!false);`); got != "true" {
		t.Errorf("got %q, want true", got)
	}
}

func TestComparison(t *testing.T) {
	cases := []struct{ src, want string }{
		{`print(1 < 2);`, "true"},
		{`print(2 <= 2);`, "true"},
		{`print(3 > 4);`, "false"},
		{`print(1 == 1);`, "true"},
		{`print(1 != 2);`, "true"},
	}
	for _, c := range cases {
		if got := run(t, c.src); got != c.want {
			t.Errorf("%s: got %q, want %q", c.src, got, c.want)
		}
	}
}

func TestVar(t *testing.T) {
	src := `var x = 10; var y = 20; print(x + y);`
	if got := run(t, src); got != "30" {
		t.Errorf("got %q, want 30", got)
	}
}

func TestAssign(t *testing.T) {
	src := `var x = 1; x = x + 1; print(x);`
	if got := run(t, src); got != "2" {
		t.Errorf("got %q, want 2", got)
	}
}

func TestIf(t *testing.T) {
	src := `
var x = 5;
if (x > 3) { print(1); } else { print(0); }
`
	if got := run(t, src); got != "1" {
		t.Errorf("got %q, want 1", got)
	}
}

func TestElseIf(t *testing.T) {
	src := `
var x = 5;
if (x > 10) { print(2); } else if (x > 3) { print(1); } else { print(0); }
`
	if got := run(t, src); got != "1" {
		t.Errorf("got %q, want 1", got)
	}
}

func TestWhile(t *testing.T) {
	src := `
var i = 0;
var s = 0;
while (i < 10) {
    i = i + 1;
    s = s + i;
}
print(s);`
	if got := run(t, src); got != "55" {
		t.Errorf("got %q, want 55", got)
	}
}

func TestFuncBasic(t *testing.T) {
	src := `
function add(a, b) { return a + b; }
print(add(3, 4));
`
	if got := run(t, src); got != "7" {
		t.Errorf("got %q, want 7", got)
	}
}

func TestFib(t *testing.T) {
	src := `
function fib(n) {
    if (n <= 1) { return n; }
    return fib(n - 1) + fib(n - 2);
}
print(fib(10));
`
	if got := run(t, src); got != "55" {
		t.Errorf("got %q, want 55", got)
	}
}

func TestFactorial(t *testing.T) {
	src := `
function fact(n) {
    if (n <= 1) { return 1; }
    return n * fact(n - 1);
}
print(fact(10));
`
	if got := run(t, src); got != "3.6288e+06" {
		t.Errorf("got %q, want 3.6288e+06", got)
	}
}

func TestLogical(t *testing.T) {
	if got := run(t, `print(true && false);`); got != "false" {
		t.Errorf("got %q", got)
	}
	if got := run(t, `print(false || true);`); got != "true" {
		t.Errorf("got %q", got)
	}
}

func TestNull(t *testing.T) {
	if got := run(t, `print(null);`); got != "null" {
		t.Errorf("got %q", got)
	}
	if got := run(t, `print(null == null);`); got != "true" {
		t.Errorf("got %q", got)
	}
}

func TestMultiPrint(t *testing.T) {
	src := `print(1, 2, 3);`
	if got := run(t, src); got != "1 2 3" {
		t.Errorf("got %q, want \"1 2 3\"", got)
	}
}

func TestLocalScopeIsolated(t *testing.T) {
	// local vars in a function should not leak to global
	src := `
var x = 1;
function f() { var x = 99; return x; }
print(f());
print(x);
`
	if got := run(t, src); got != "99\n1" {
		t.Errorf("got %q", got)
	}
}
