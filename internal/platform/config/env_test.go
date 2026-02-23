package consts

import "testing"

func TestReadAppEnv(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want AppEnvironment
	}{
		{name: "empty defaults to prod", raw: "", want: envProd},
		{name: "prod", raw: "prod", want: envProd},
		{name: "prod with spaces", raw: " prod ", want: envProd},
		{name: "test", raw: "test", want: envTest},
		{name: "dev", raw: "dev", want: envDev},
		{name: "case insensitive", raw: "DEV", want: envDev},
		{name: "invalid defaults to prod", raw: "staging", want: envProd},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SWAVES_ENV", tc.raw)
			if got := readAppEnv("SWAVES_ENV"); got != tc.want {
				t.Fatalf("readAppEnv() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEnvIsAndEnvIsNot(t *testing.T) {
	original := AppEnv
	defer func() { AppEnv = original }()

	AppEnv = envProd
	if !EnvIs(envProd) {
		t.Fatalf("EnvIs(envProd) should be true")
	}
	if EnvIsNot(envProd) {
		t.Fatalf("EnvIsNot(envProd) should be false")
	}

	AppEnv = envDev
	if !EnvIsNot(envProd) {
		t.Fatalf("EnvIsNot(envProd) should be true when env=dev")
	}
	if EnvIs(envProd) {
		t.Fatalf("EnvIs(envProd) should be false when env=dev")
	}
}
