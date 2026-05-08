package server

import "testing"

func TestParseInitOptionsEmpty(t *testing.T) {
	cfg := parseInitOptions(nil)
	if cfg.Format.Indentation != 0 {
		t.Errorf("Indentation=%d want 0", cfg.Format.Indentation)
	}
	if cfg.Format.NormalizeStrings {
		t.Errorf("NormalizeStrings=true want false")
	}
}

func TestParseInitOptionsDetectString(t *testing.T) {
	raw := map[string]any{
		"format": map[string]any{
			"indentation":      "detect",
			"normalizeStrings": false,
		},
	}
	cfg := parseInitOptions(raw)
	if cfg.Format.Indentation != 0 {
		t.Errorf("Indentation=%d want 0 (detect)", cfg.Format.Indentation)
	}
}

func TestParseInitOptionsFixedIndent(t *testing.T) {
	raw := map[string]any{
		"format": map[string]any{
			"indentation": float64(4),
		},
	}
	cfg := parseInitOptions(raw)
	if cfg.Format.Indentation != 4 {
		t.Errorf("Indentation=%d want 4", cfg.Format.Indentation)
	}
}

func TestParseInitOptionsNormalizeStrings(t *testing.T) {
	raw := map[string]any{
		"format": map[string]any{
			"normalizeStrings": true,
		},
	}
	cfg := parseInitOptions(raw)
	if !cfg.Format.NormalizeStrings {
		t.Errorf("NormalizeStrings=false want true")
	}
}

func TestParseInitOptionsNegativeIndentFallsBackToDetect(t *testing.T) {
	raw := map[string]any{
		"format": map[string]any{
			"indentation": float64(-1),
		},
	}
	cfg := parseInitOptions(raw)
	if cfg.Format.Indentation != 0 {
		t.Errorf("Indentation=%d want 0 (negative coerced to detect)", cfg.Format.Indentation)
	}
}

func TestParseInitOptionsMalformedDoesNotPanic(t *testing.T) {
	// A garbage payload should silently degrade to defaults, not crash
	// the initialize handler.
	cfg := parseInitOptions("not-an-object")
	if cfg.Format.Indentation != 0 || cfg.Format.NormalizeStrings {
		t.Errorf("expected defaults, got %+v", cfg)
	}
}
