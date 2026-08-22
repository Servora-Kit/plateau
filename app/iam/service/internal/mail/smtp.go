package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	stdmail "net/mail"
	"strings"

	mailpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/mail/v1"
	gomail "github.com/wneessen/go-mail"
)

type smtpSender struct {
	host string
	opts []gomail.Option
}

// NewSender creates the required synchronous SMTP sender from shared Mail config.
func NewSender(config *mailpb.Mail) (Sender, error) {
	if config == nil || config.GetSmtp() == nil {
		return nil, fmt.Errorf("mail: SMTP configuration is required")
	}
	smtp := config.GetSmtp()
	host := strings.TrimSpace(smtp.GetHost())
	if host == "" {
		return nil, fmt.Errorf("mail: SMTP host is required")
	}
	port := int(smtp.GetPort())
	if port == 0 {
		port = 587
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("mail: SMTP port is invalid")
	}
	if (smtp.GetUsername() == "") != (smtp.GetPassword() == "") {
		return nil, fmt.Errorf("mail: SMTP username and password must be configured together")
	}

	opts := []gomail.Option{gomail.WithPort(port)}
	if smtp.GetUsername() != "" {
		opts = append(opts,
			gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
			gomail.WithUsername(smtp.GetUsername()),
			gomail.WithPassword(smtp.GetPassword()),
		)
	}
	if smtp.GetTls() {
		opts = append(opts, gomail.WithTLSPortPolicy(gomail.TLSMandatory))
	} else {
		opts = append(opts, gomail.WithTLSPortPolicy(gomail.TLSOpportunistic))
	}
	if smtp.GetSkipVerifySsl() {
		opts = append(opts, gomail.WithTLSConfig(&tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true, //nolint:gosec // explicitly limited to configured development SMTP
		}))
	}
	if timeout := smtp.GetSendTimeout(); timeout != nil {
		duration := timeout.AsDuration()
		if duration <= 0 {
			return nil, fmt.Errorf("mail: SMTP send timeout must be positive")
		}
		opts = append(opts, gomail.WithTimeout(duration))
	}
	return &smtpSender{host: host, opts: opts}, nil
}

func (sender *smtpSender) Send(ctx context.Context, email Message) error {
	if ctx == nil {
		return fmt.Errorf("mail: context is nil")
	}
	if email.To == "" {
		return fmt.Errorf("mail: recipient is required")
	}
	message := gomail.NewMsg()
	from := (&stdmail.Address{Name: email.From.Name, Address: email.From.Address}).String()
	if err := message.From(from); err != nil {
		return fmt.Errorf("mail: set From header: %w", err)
	}
	if err := message.To(email.To); err != nil {
		return fmt.Errorf("mail: set To header: %w", err)
	}
	message.Subject(email.Subject)
	if len(email.HTML) > 0 {
		message.SetBodyString(gomail.TypeTextHTML, string(email.HTML))
		if len(email.Text) > 0 {
			message.AddAlternativeString(gomail.TypeTextPlain, string(email.Text))
		}
	} else if len(email.Text) > 0 {
		message.SetBodyString(gomail.TypeTextPlain, string(email.Text))
	}

	client, err := gomail.NewClient(sender.host, sender.opts...)
	if err != nil {
		return fmt.Errorf("mail: create SMTP client: %w", err)
	}
	if err := client.DialAndSendWithContext(ctx, message); err != nil {
		return fmt.Errorf("mail: send: %w", err)
	}
	return nil
}

var _ Sender = (*smtpSender)(nil)
