package assets

import "embed"

// MailFS contains IAM-owned templates and images compiled into the service binary.
//
//go:embed mailTemplate/*.html mailTemplate/logo.png
var MailFS embed.FS
