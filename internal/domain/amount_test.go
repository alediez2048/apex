package domain

import (
	"testing"
)

func TestAmount_ToDollars(t *testing.T) {
	if got := Amount(15000).ToDollars(); got != "$150.00" {
		t.Errorf("Amount(15000).ToDollars() = %q, want $150.00", got)
	}
	if got := Amount(0).ToDollars(); got != "$0.00" {
		t.Errorf("Amount(0).ToDollars() = %q, want $0.00", got)
	}
	if got := Amount(1).ToDollars(); got != "$0.01" {
		t.Errorf("Amount(1).ToDollars() = %q, want $0.01", got)
	}
}

func TestParseAmount(t *testing.T) {
	got, err := ParseAmount("150.00")
	if err != nil {
		t.Fatalf("ParseAmount(150.00): %v", err)
	}
	if got != Amount(15000) {
		t.Errorf("ParseAmount(150.00) = %v, want 15000", got)
	}
}

func TestParseAmount_FractionalCentsRejected(t *testing.T) {
	_, err := ParseAmount("150.001")
	if err == nil {
		t.Error("ParseAmount(150.001) expected error (fractional cents)")
	}
}

func TestParseAmount_EdgeCases(t *testing.T) {
	tests := []struct {
		in   string
		want Amount
	}{
		{"0", 0},
		{"0.00", 0},
		{"1.50", 150},
		{"5000.00", 500000},
	}
	for _, tt := range tests {
		got, err := ParseAmount(tt.in)
		if err != nil {
			t.Errorf("ParseAmount(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseAmount(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseAmount_Invalid(t *testing.T) {
	_, err := ParseAmount("")
	if err == nil {
		t.Error("ParseAmount(\"\") expected error")
	}
	_, err = ParseAmount("abc")
	if err == nil {
		t.Error("ParseAmount(\"abc\") expected error")
	}
}
