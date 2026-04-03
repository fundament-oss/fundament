package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDir_FunclConfigDir_Absolute(t *testing.T) {
	t.Setenv("FUNCTL_CONFIG_DIR", "/tmp/functl-config")
	t.Setenv("XDG_CONFIG_HOME", "")

	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "/tmp/functl-config" {
		t.Errorf("got %q, want /tmp/functl-config", dir)
	}
}

func TestConfigDir_FunclConfigDir_Relative(t *testing.T) {
	t.Setenv("FUNCTL_CONFIG_DIR", "relative/path")

	_, err := ConfigDir()
	if err == nil {
		t.Fatal("expected error for relative FUNCTL_CONFIG_DIR, got nil")
	}
}

func TestConfigDir_FunclConfigDir_Precedence(t *testing.T) {
	t.Setenv("FUNCTL_CONFIG_DIR", "/custom/dir")
	t.Setenv("XDG_CONFIG_HOME", "/xdg/config")

	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "/custom/dir" {
		t.Errorf("got %q, want /custom/dir", dir)
	}
}

func TestConfigDir_XDGConfigHome_Absolute(t *testing.T) {
	t.Setenv("FUNCTL_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "/home/user/.myconfig")

	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/home/user/.myconfig/fundament"
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
}

func TestConfigDir_XDGConfigHome_Relative(t *testing.T) {
	t.Setenv("FUNCTL_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "relative/config")

	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "fundament")
	if dir != want {
		t.Errorf("got %q, want %q (should fall through to default)", dir, want)
	}
}

func TestConfigDir_Default(t *testing.T) {
	t.Setenv("FUNCTL_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "fundament")
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
}
