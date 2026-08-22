package mail

import (
	"context"
	"fmt"

	mailpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/mail/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	"github.com/google/wire"
)

// ProviderSet provides the IAM mail renderer and synchronous sender.
var ProviderSet = wire.NewSet(NewMailer, NewMailSender, NewTemplates, NewFrom, NewSender)

// From is the validated sender identity used for IAM mail.
type From struct {
	Address string
	Name    string
}

// Message is one fully rendered outbound email.
type Message struct {
	From    From
	To      string
	Subject string
	Text    []byte
	HTML    []byte
}

// Sender synchronously submits one rendered email.
type Sender interface {
	Send(context.Context, Message) error
}

// Mailer renders IAM-owned verification and password-reset messages.
type Mailer struct {
	sender    Sender
	from      From
	templates *Templates
}

// NewMailer constructs the IAM mail capability from independently testable dependencies.
func NewMailer(sender Sender, from From, templates *Templates) (*Mailer, error) {
	if sender == nil {
		return nil, fmt.Errorf("mail: sender is nil")
	}
	if templates == nil {
		return nil, fmt.Errorf("mail: templates are nil")
	}
	return &Mailer{sender: sender, from: from, templates: templates}, nil
}

// NewMailSender exposes the IAM mailer through the biz-owned capability port.
func NewMailSender(mailer *Mailer) biz.MailSender {
	return mailer
}

// SendVerification sends the shared verification message used by registration and administrator creation.
func (mailer *Mailer) SendVerification(ctx context.Context, to, link string, expiryHours int) error {
	html, err := mailer.templates.RenderVerification(link, expiryHours)
	if err != nil {
		return err
	}
	return mailer.sender.Send(ctx, Message{
		From:    mailer.from,
		To:      to,
		Subject: "验证您的邮箱",
		Text:    []byte(fmt.Sprintf("请访问以下链接验证您的邮箱（%d 小时内有效）：\n%s", expiryHours, link)),
		HTML:    html,
	})
}

// SendPasswordReset sends a password-reset message.
func (mailer *Mailer) SendPasswordReset(ctx context.Context, to, link string, expiryHours int) error {
	html, err := mailer.templates.RenderPasswordReset(link, expiryHours)
	if err != nil {
		return err
	}
	return mailer.sender.Send(ctx, Message{
		From:    mailer.from,
		To:      to,
		Subject: "重置您的密码",
		Text:    []byte(fmt.Sprintf("请访问以下链接重置您的密码（%d 小时内有效）：\n%s", expiryHours, link)),
		HTML:    html,
	})
}

// NewFrom validates and converts the shared Mail sender identity.
func NewFrom(config *mailpb.Mail) (From, error) {
	if config == nil || config.GetFrom() == nil {
		return From{}, fmt.Errorf("mail: from configuration is required")
	}
	from := From{Address: config.GetFrom().GetAddress(), Name: config.GetFrom().GetName()}
	if from.Address == "" {
		return From{}, fmt.Errorf("mail: from address is required")
	}
	return from, nil
}
