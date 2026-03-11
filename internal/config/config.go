package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds application configuration from env and YAML.
type Config struct {
	Port          string
	Env           string
	DBPath        string
	Correspondents []Correspondent
	Investors     []Investor
}

// Correspondent is a firm with deposit limits and omnibus account.
type Correspondent struct {
	ID                      string `yaml:"id"`
	Name                    string `yaml:"name"`
	DepositLimitCents       int64  `yaml:"deposit_limit_cents"`
	OmnibusAccountID        string `yaml:"omnibus_account_id"`
	DefaultContributionType string `yaml:"default_contribution_type"`
}

// Investor is a test investor with API key and account mapping.
type Investor struct {
	AccountID       string `yaml:"account_id"`
	APIKey          string `yaml:"api_key"`
	CorrespondentID string `yaml:"correspondent_id"`
	Eligible        bool   `yaml:"eligible"`
	AccountType     string `yaml:"account_type"`
}

type correspondentsFile struct {
	OmnibusAccounts []string       `yaml:"omnibus_accounts"`
	Correspondents  []Correspondent `yaml:"correspondents"`
}

type investorsFile struct {
	Investors []Investor `yaml:"investors"`
}

// Load reads env and YAML files, validates, and returns Config.
func Load() (*Config, error) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	env := os.Getenv("ENV")
	if env == "" {
		env = "development"
	}
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "data/apex.db"
	}

	cfg := &Config{Port: port, Env: env, DBPath: dbPath}

	corrPath := "config/correspondents.yaml"
	corrData, err := os.ReadFile(corrPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", corrPath, err)
	}
	var corrFile correspondentsFile
	if err := yaml.Unmarshal(corrData, &corrFile); err != nil {
		return nil, fmt.Errorf("parse %s: %w", corrPath, err)
	}
	cfg.Correspondents = corrFile.Correspondents

	invPath := "config/investors.yaml"
	invData, err := os.ReadFile(invPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", invPath, err)
	}
	var invFile investorsFile
	if err := yaml.Unmarshal(invData, &invFile); err != nil {
		return nil, fmt.Errorf("parse %s: %w", invPath, err)
	}
	cfg.Investors = invFile.Investors

	allowlist := make(map[string]bool)
	for _, id := range corrFile.OmnibusAccounts {
		allowlist[id] = true
	}
	for _, c := range cfg.Correspondents {
		if c.OmnibusAccountID == "" {
			return nil, fmt.Errorf("correspondent %s: omnibus_account_id is required", c.ID)
		}
		if !allowlist[c.OmnibusAccountID] {
			return nil, fmt.Errorf("correspondent %s references non-existent omnibus account %q", c.ID, c.OmnibusAccountID)
		}
	}

	return cfg, nil
}

// InvestorByAccountID returns the investor with the given account ID, or nil.
func (c *Config) InvestorByAccountID(accountID string) *Investor {
	for i := range c.Investors {
		if c.Investors[i].AccountID == accountID {
			return &c.Investors[i]
		}
	}
	return nil
}
