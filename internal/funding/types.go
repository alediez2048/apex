package funding

import "github.com/alediez2048/apex/internal/config"

// Context is the resolved funding context after auth and business rules pass.
type Context struct {
	Investor       config.Investor
	Correspondent  config.Correspondent
	OmnibusID      string
	ContributionType string // default for IRA etc.
}
