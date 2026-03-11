package pipeline

// Risk thresholds (MVP single-signal: vendor confidence).
const (
	ConfidenceThreshold = 0.90
	RiskLow             = 0
	RiskCritical        = 65
)

// RiskScore computes a 0-100 score from vendor confidence.
// confidence >= 0.9 → 0 (LOW, auto-approve)
// confidence <  0.9 → 65 (CRITICAL, flagged for operator)
func RiskScore(confidence float64) int {
	if confidence >= ConfidenceThreshold {
		return RiskLow
	}
	return RiskCritical
}

// IsFlagged returns true when risk score requires operator review.
func IsFlagged(score int) bool {
	return score >= RiskCritical
}
