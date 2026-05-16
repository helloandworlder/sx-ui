package config

import (
	"path/filepath"
	"testing"
)

func TestOpenUIDefaultPaths(t *testing.T) {
	t.Setenv("OPENUI_DB_FOLDER", "")
	t.Setenv("XUI_DB_FOLDER", "")
	t.Setenv("OPENUI_LOG_FOLDER", "")
	t.Setenv("XUI_LOG_FOLDER", "")

	if got, want := GetName(), "open-ui"; got != want {
		t.Fatalf("GetName() = %q, want %q", got, want)
	}
	if got, want := GetDBFolderPath(), "/etc/open-ui"; got != want {
		t.Fatalf("GetDBFolderPath() = %q, want %q", got, want)
	}
	if got, want := GetDBPath(), "/etc/open-ui/open-ui.db"; got != want {
		t.Fatalf("GetDBPath() = %q, want %q", got, want)
	}
	if got, want := GetLogFolder(), "/var/log/open-ui"; got != want {
		t.Fatalf("GetLogFolder() = %q, want %q", got, want)
	}
}

func TestOpenUIEnvVarsOverrideLegacyXUIVars(t *testing.T) {
	openUIDB := filepath.Join(t.TempDir(), "openui-db")
	legacyDB := filepath.Join(t.TempDir(), "xui-db")
	openUILog := filepath.Join(t.TempDir(), "openui-log")
	legacyLog := filepath.Join(t.TempDir(), "xui-log")

	t.Setenv("OPENUI_DB_FOLDER", openUIDB)
	t.Setenv("XUI_DB_FOLDER", legacyDB)
	t.Setenv("OPENUI_LOG_FOLDER", openUILog)
	t.Setenv("XUI_LOG_FOLDER", legacyLog)
	t.Setenv("OPENUI_BIN_FOLDER", "/opt/open-ui/bin")
	t.Setenv("XUI_BIN_FOLDER", "/opt/x-ui/bin")
	t.Setenv("OPENUI_LOG_LEVEL", string(Warning))
	t.Setenv("XUI_LOG_LEVEL", string(Debug))

	if got := GetDBFolderPath(); got != openUIDB {
		t.Fatalf("GetDBFolderPath() = %q, want OPENUI_DB_FOLDER %q", got, openUIDB)
	}
	if got := GetLogFolder(); got != openUILog {
		t.Fatalf("GetLogFolder() = %q, want OPENUI_LOG_FOLDER %q", got, openUILog)
	}
	if got := GetBinFolderPath(); got != "/opt/open-ui/bin" {
		t.Fatalf("GetBinFolderPath() = %q, want OPENUI_BIN_FOLDER", got)
	}
	if got := GetLogLevel(); got != Warning {
		t.Fatalf("GetLogLevel() = %q, want OPENUI_LOG_LEVEL", got)
	}
}
