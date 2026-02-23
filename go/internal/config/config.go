package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CredentialsPath returns the path to the credentials file,
// respecting $XDG_CONFIG_HOME with fallback to ~/.config/wrds-dl/credentials.
func CredentialsPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "wrds-dl", "credentials")
}

// LoadCredentials reads PGUSER, PGPASSWORD, and PGDATABASE from the credentials file.
func LoadCredentials() (user, password, database string, err error) {
	f, err := os.Open(CredentialsPath())
	if err != nil {
		return "", "", "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "PGUSER":
			user = strings.TrimSpace(val)
		case "PGPASSWORD":
			password = strings.TrimSpace(val)
		case "PGDATABASE":
			database = strings.TrimSpace(val)
		}
	}
	return user, password, database, scanner.Err()
}

// SaveCredentials writes PGUSER, PGPASSWORD, and PGDATABASE to the credentials file.
// It creates the parent directories with 0700 and the file with 0600 permissions.
func SaveCredentials(user, password, database string) error {
	path := CredentialsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	content := fmt.Sprintf("PGUSER=%s\nPGPASSWORD=%s\nPGDATABASE=%s\n", user, password, database)
	return os.WriteFile(path, []byte(content), 0600)
}

// ApplyCredentials loads credentials from the config file and sets
// environment variables for any values not already set.
func ApplyCredentials() {
	user, password, database, err := LoadCredentials()
	if err != nil {
		return // file doesn't exist or unreadable — silently skip
	}
	if os.Getenv("PGUSER") == "" && user != "" {
		os.Setenv("PGUSER", user)
	}
	if os.Getenv("PGPASSWORD") == "" && password != "" {
		os.Setenv("PGPASSWORD", password)
	}
	if os.Getenv("PGDATABASE") == "" && database != "" {
		os.Setenv("PGDATABASE", database)
	}
}
