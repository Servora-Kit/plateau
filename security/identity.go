package security

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

var emailCaseFolder = cases.Fold()

// NormalizeEmail trims leading/trailing whitespace, applies Unicode NFC
// normalization, and Unicode case-folds raw into Plateau's canonical comparable email.
func NormalizeEmail(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	return emailCaseFolder.String(norm.NFC.String(trimmed))
}
