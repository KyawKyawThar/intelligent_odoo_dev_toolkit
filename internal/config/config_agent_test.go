package config_test

// TestLoadConfigReadsAgentVarsFromEnv guards against the class of bug where a
// new Odoo/agent env var is added to the Config struct but not registered with
// viper via mustBindEnv.  When the agent runs as a system service (launchd /
// systemd) there is no .env file present — all values come from OS env vars
// that loadSystemConfig pushes via os.Setenv.  If a key is not bound, viper
// silently ignores it and the field stays at its zero value.
//
// This test simulates that exact path: no config files on disk, values set
// only through os.Setenv, then calls LoadConfig and asserts every agent-
// critical field is populated.

import (
	"testing"

	"Intelligent_Dev_ToolKit_Odoo/internal/config"
)

func TestLoadConfigReadsAgentVarsFromEnv(t *testing.T) {
	// Agent-critical vars that must survive the os.Setenv → LoadConfig path.
	vars := map[string]string{
		"ODOO_URL":                 "http://odoo.example.com:8069",
		"PG_ODOO_DB":               "myodoo",
		"ODOO_ADMIN_USER":          "admin",
		"ODOO_ADMIN_PASSWORD":      "secret",
		"AGENT_CLOUD_URL":          "wss://api.example.com/api/v1/agent/ws",
		"AGENT_REGISTRATION_TOKEN": "reg_testtoken",
	}

	// Set each var in the OS environment, then clean up after the test.
	for k, v := range vars {
		t.Setenv(k, v)
	}

	// Use a temp dir that has no .env file — mirrors the system-service context.
	dir := t.TempDir()

	cfg, err := config.LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	checks := []struct {
		field string
		got   string
		want  string
	}{
		{"OdooURL", cfg.OdooURL, vars["ODOO_URL"]},
		{"OdooDB", cfg.OdooDB, vars["PG_ODOO_DB"]},
		{"OdooUser", cfg.OdooUser, vars["ODOO_ADMIN_USER"]},
		{"OdooPassword", cfg.OdooPassword, vars["ODOO_ADMIN_PASSWORD"]},
		{"AgentCloudURL", cfg.AgentCloudURL, vars["AGENT_CLOUD_URL"]},
		{"AgentRegistrationToken", cfg.AgentRegistrationToken, vars["AGENT_REGISTRATION_TOKEN"]},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("cfg.%s = %q, want %q — add the env var key to mustBindEnv in LoadConfig", c.field, c.got, c.want)
		}
	}

}
