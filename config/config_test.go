package config

import (
	"strings"
	"testing"
)

func TestLoadReportsAllMissing(t *testing.T) {
	// t.Setenv("PORT", "not-a-number") // DATABASE_URL deliberately left unset

	_, _, err := Load("../.env")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") ||
		!strings.Contains(err.Error(), "PORT") {
		t.Fatalf("both problems should be reported, got: %v", err)
	}
}
