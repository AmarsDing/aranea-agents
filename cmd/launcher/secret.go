package main

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// legacyAuthSecret is the hardcoded dev secret historically used by the
// portable package. Kept for upgrades: existing installs may have JWT/PAT
// issued under it, and rotating would invalidate every session.
const legacyAuthSecret = "aranea-portable-dev-secret-32chars!!"

// authSecretFileName lives under configs/ and holds the per-install random
// KRATOS_AUTH_SECRET generated at first run.
const authSecretFileName = "auth.secret"

// resolveAuthSecret picks the KRATOS_AUTH_SECRET for this install:
//  1. configs/auth.secret (persisted, ≥32 chars) — always wins;
//  2. fresh install (no wizard state, no bundled PG data) → generate a random
//     64-char hex secret and persist it;
//  3. otherwise (upgrade with possible issued tokens) → legacy dev secret.
func resolveAuthSecret(root string, log func(string, ...any)) string {
	path := filepath.Join(root, "configs", authSecretFileName)
	if b, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(b)); len(s) >= 32 {
			return s
		}
		log("auth.secret too short (<32 chars); regenerating")
	}
	if isFreshInstall(root) {
		s, err := generateAuthSecret()
		if err != nil {
			log("random secret generation failed (%v); falling back to legacy secret", err)
			return legacyAuthSecret
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			if err := os.WriteFile(path, []byte(s+"\n"), 0o600); err != nil {
				log("persist auth.secret failed: %v", err)
			} else {
				log("generated per-install KRATOS_AUTH_SECRET -> configs\\auth.secret")
			}
		}
		return s
	}
	return legacyAuthSecret
}

// isFreshInstall reports whether this launcher run belongs to a never-started
// install: no setup wizard state and no initialized bundled PG cluster.
func isFreshInstall(root string) bool {
	if setupDone(root) {
		return false
	}
	if _, err := os.Stat(filepath.Join(root, "postgres", "data", "PG_VERSION")); err == nil {
		return false
	}
	return true
}

func generateAuthSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
