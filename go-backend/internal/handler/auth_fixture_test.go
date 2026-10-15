package handler

import (
	"piguard/go-backend/internal/auth"
	"testing"
)

// Existing business fixtures opt into local development mode; auth cases belong to V1.
func disabledTestAuth(t *testing.T) *auth.Manager {
	t.Helper()
	m, err := auth.New(auth.Config{Disabled: true, SessionTTLMinutes: 30}, "127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}
