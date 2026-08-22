package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/assets"
)

const (
	verifyTemplatePath = "mailTemplate/verify_email.html"
	resetTemplatePath  = "mailTemplate/reset_password.html"
	logoPath           = "mailTemplate/logo.png"
)

// Templates holds parsed immutable IAM mail templates.
type Templates struct {
	verification  *template.Template
	passwordReset *template.Template
	logoDataURI   string
}

type templateData struct {
	Link        string
	ExpiryHours int
	LogoDataURI template.URL
}

// NewTemplates parses embedded assets once during service startup.
func NewTemplates() (*Templates, error) {
	verification, err := template.ParseFS(assets.MailFS, verifyTemplatePath)
	if err != nil {
		return nil, fmt.Errorf("mail: parse verification template: %w", err)
	}
	passwordReset, err := template.ParseFS(assets.MailFS, resetTemplatePath)
	if err != nil {
		return nil, fmt.Errorf("mail: parse password-reset template: %w", err)
	}
	logo, err := assets.MailFS.ReadFile(logoPath)
	if err != nil {
		return nil, fmt.Errorf("mail: read embedded logo: %w", err)
	}
	return &Templates{
		verification:  verification,
		passwordReset: passwordReset,
		logoDataURI:   "data:image/png;base64," + base64.StdEncoding.EncodeToString(logo),
	}, nil
}

// RenderVerification renders the shared registration and administrator-created User message.
func (templates *Templates) RenderVerification(link string, expiryHours int) ([]byte, error) {
	return templates.render(templates.verification, link, expiryHours)
}

// RenderPasswordReset renders the password-reset message.
func (templates *Templates) RenderPasswordReset(link string, expiryHours int) ([]byte, error) {
	return templates.render(templates.passwordReset, link, expiryHours)
}

func (templates *Templates) render(source *template.Template, link string, expiryHours int) ([]byte, error) {
	if link == "" {
		return nil, fmt.Errorf("mail: link is empty")
	}
	if expiryHours <= 0 {
		return nil, fmt.Errorf("mail: expiry hours must be positive")
	}
	var rendered bytes.Buffer
	if err := source.Execute(&rendered, templateData{
		Link:        link,
		ExpiryHours: expiryHours,
		LogoDataURI: template.URL(templates.logoDataURI),
	}); err != nil {
		return nil, fmt.Errorf("mail: render template: %w", err)
	}
	return rendered.Bytes(), nil
}
