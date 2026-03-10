package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// Amount is money in integer cents (int64) to avoid floating-point rounding errors.
type Amount int64

// ToDollars returns a formatted string e.g. "$150.00" for 15000 cents.
func (a Amount) ToDollars() string {
	cents := int64(a)
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	dollars := cents / 100
	c := cents % 100
	return fmt.Sprintf("%s$%d.%02d", sign, dollars, c)
}

// ParseAmount parses a decimal string like "150.00" into cents. Rejects fractional cents (e.g. "150.001").
// Accepts negative strings for display/reversal math; deposit submission should reject negative amounts at the API boundary.
func ParseAmount(s string) (Amount, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("amount: empty string")
	}
	negative := strings.HasPrefix(s, "-")
	if negative {
		s = strings.TrimSpace(s[1:])
	}
	parts := strings.SplitN(s, ".", 2)
	dollarsStr := parts[0]
	centsStr := "00"
	if len(parts) == 2 {
		centsStr = parts[1]
		if len(centsStr) > 2 {
			return 0, fmt.Errorf("amount: fractional cents not allowed (e.g. 150.001)")
		}
		for len(centsStr) < 2 {
			centsStr += "0"
		}
	}
	combined := dollarsStr + centsStr
	cents, err := strconv.ParseInt(combined, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount: invalid format: %w", err)
	}
	if negative {
		cents = -cents
	}
	return Amount(cents), nil
}
