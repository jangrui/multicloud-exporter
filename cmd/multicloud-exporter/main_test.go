package main

import (
	"multicloud-exporter/internal/config"
	"testing"
)

func adminAuthEnabled(cfg *config.Config) (bool, []config.BasicAuth) {
	if cfg.Server != nil && cfg.Server.AdminAuthEnabled && len(cfg.Server.AdminAuth) > 0 {
		return true, cfg.Server.AdminAuth
	}
	if cfg.ServerConf != nil && cfg.ServerConf.AdminAuthEnabled && len(cfg.ServerConf.AdminAuth) > 0 {
		return true, cfg.ServerConf.AdminAuth
	}
	return false, nil
}

func checkBasicAuth(pairs []config.BasicAuth, u, p string) bool {
	for _, pair := range pairs {
		if u == pair.Username && p == pair.Password {
			return true
		}
	}
	return false
}

func TestAdminAuthEnabled(t *testing.T) {
	cfg := &config.Config{Server: &config.ServerConf{AdminAuthEnabled: false}}
	if ok, _ := adminAuthEnabled(cfg); ok {
		t.Fatalf("disabled")
	}
	cfg.Server.AdminAuthEnabled = true
	cfg.Server.AdminAuth = []config.BasicAuth{{Username: "u", Password: "p"}}
	if ok, pairs := adminAuthEnabled(cfg); !ok || len(pairs) != 1 {
		t.Fatalf("enabled")
	}
}

func TestCheckBasicAuth(t *testing.T) {
	pairs := []config.BasicAuth{{Username: "a", Password: "b"}}
	if !checkBasicAuth(pairs, "a", "b") {
		t.Fatalf("good")
	}
	if checkBasicAuth(pairs, "a", "x") {
		t.Fatalf("bad")
	}
}
