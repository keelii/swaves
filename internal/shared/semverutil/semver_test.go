package semverutil

import "testing"

func TestIsStable(t *testing.T) {
	if !IsStable("v1.2.3") {
		t.Fatal("expected v1.2.3 to be stable")
	}
	if IsStable("v1.2.3-rc.1") {
		t.Fatal("expected prerelease version not to be stable")
	}
	if IsStable("dev") {
		t.Fatal("expected dev not to be stable")
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{left: "v1.2.3", right: "v1.2.3", want: 0},
		{left: "v1.2.3", right: "v1.2.4", want: -1},
		{left: "v1.3.0", right: "v1.2.9", want: 1},
		{left: "v1.2.3-rc.1", right: "v1.2.3", want: -1},
		{left: "v1.2.3-rc.2", right: "v1.2.3-rc.10", want: -1},
		{left: "v1.2.3-alpha.beta", right: "v1.2.3-alpha.1", want: 1},
	}

	for _, tt := range tests {
		got, err := Compare(tt.left, tt.right)
		if err != nil {
			t.Fatalf("Compare(%q, %q) failed: %v", tt.left, tt.right, err)
		}
		if got != tt.want {
			t.Fatalf("Compare(%q, %q) = %d, want %d", tt.left, tt.right, got, tt.want)
		}
	}
}
