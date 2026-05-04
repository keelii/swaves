package config

import "testing"

func withAppEnv(t *testing.T, env AppEnvironment) {
	t.Helper()

	original := AppEnv
	AppEnv = env
	t.Cleanup(func() {
		AppEnv = original
	})
}

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
	withAppEnv(t, envProd)
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

func TestShouldEnsureDefaultSettings(t *testing.T) {
	t.Run("prod never ensures", func(t *testing.T) {
		withAppEnv(t, envProd)
		t.Setenv("SWAVES_ENSURE_DEFAULT_SETTINGS", "true")
		if ShouldEnsureDefaultSettings() {
			t.Fatal("ShouldEnsureDefaultSettings should be false in prod")
		}
	})

	t.Run("dev defaults to false", func(t *testing.T) {
		withAppEnv(t, envDev)
		t.Setenv("SWAVES_ENSURE_DEFAULT_SETTINGS", "")
		if ShouldEnsureDefaultSettings() {
			t.Fatal("ShouldEnsureDefaultSettings should default to false in dev")
		}
	})

	t.Run("dev explicit true", func(t *testing.T) {
		withAppEnv(t, envDev)
		t.Setenv("SWAVES_ENSURE_DEFAULT_SETTINGS", "true")
		if !ShouldEnsureDefaultSettings() {
			t.Fatal("ShouldEnsureDefaultSettings should be true when explicitly enabled in dev")
		}
	})

	t.Run("dev explicit false-like value", func(t *testing.T) {
		withAppEnv(t, envDev)
		t.Setenv("SWAVES_ENSURE_DEFAULT_SETTINGS", "false")
		if ShouldEnsureDefaultSettings() {
			t.Fatal("ShouldEnsureDefaultSettings should be false for false-like values")
		}
	})
}

func TestShouldEnableSQLLog(t *testing.T) {
	tests := []struct {
		name string
		env  AppEnvironment
		want bool
	}{
		{name: "prod disables sql log", env: envProd, want: false},
		{name: "test disables sql log", env: envTest, want: false},
		{name: "dev enables sql log", env: envDev, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldEnableSQLLog(tc.env); got != tc.want {
				t.Fatalf("shouldEnableSQLLog(%q) = %v, want %v", tc.env, got, tc.want)
			}
		})
	}
}

func TestReadPositiveEnvInt(t *testing.T) {
	tests := []struct {
		name string
		raw  *string
		want int
	}{
		{name: "missing uses default", want: 42},
		{name: "valid", raw: strPtr("2048"), want: 2048},
		{name: "valid with spaces", raw: strPtr(" 4096 "), want: 4096},
		{name: "invalid uses default", raw: strPtr("large"), want: 42},
		{name: "zero uses default", raw: strPtr("0"), want: 42},
		{name: "negative uses default", raw: strPtr("-1"), want: 42},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.raw != nil {
				t.Setenv("SWAVES_BODY_LIMIT_TEST", *tc.raw)
			}
			if got := readPositiveEnvInt("SWAVES_BODY_LIMIT_TEST", 42); got != tc.want {
				t.Fatalf("readPositiveEnvInt() = %d, want %d", got, tc.want)
			}
		})
	}
}

func strPtr(value string) *string {
	return &value
}
