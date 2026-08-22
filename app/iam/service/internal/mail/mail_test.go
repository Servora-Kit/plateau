package mail

import (
	"context"
	"errors"
	"strings"
	"testing"

	mailpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/mail/v1"
)

type fakeSender struct {
	message Message
	err     error
}

func (sender *fakeSender) Send(_ context.Context, message Message) error {
	sender.message = message
	return sender.err
}

func TestMailerRendersVerificationAndPasswordReset(t *testing.T) {
	templates, err := NewTemplates()
	if err != nil {
		t.Fatalf("NewTemplates() error = %v", err)
	}
	sender := new(fakeSender)
	mailer, err := NewMailer(sender, From{Address: "no-reply@plateau.local", Name: "Plateau IAM"}, templates)
	if err != nil {
		t.Fatalf("NewMailer() error = %v", err)
	}

	if err := mailer.SendVerification(t.Context(), "person@example.com", "https://iam.example/verify#token", 24); err != nil {
		t.Fatalf("SendVerification() error = %v", err)
	}
	if sender.message.Subject != "验证您的邮箱" || sender.message.To != "person@example.com" {
		t.Fatalf("verification message = %#v", sender.message)
	}
	verificationHTML := string(sender.message.HTML)
	if !strings.Contains(verificationHTML, "https://iam.example/verify#token") || !strings.Contains(verificationHTML, "data:image/png;base64,") {
		t.Fatal("verification message omitted link or embedded logo")
	}

	if err := mailer.SendPasswordReset(t.Context(), "person@example.com", "https://iam.example/reset#token", 1); err != nil {
		t.Fatalf("SendPasswordReset() error = %v", err)
	}
	if sender.message.Subject != "重置您的密码" || !strings.Contains(string(sender.message.HTML), "https://iam.example/reset#token") {
		t.Fatalf("password reset message = %#v", sender.message)
	}
}

func TestMailerPropagatesSynchronousSenderFailure(t *testing.T) {
	templates, err := NewTemplates()
	if err != nil {
		t.Fatalf("NewTemplates() error = %v", err)
	}
	sendErr := errors.New("SMTP unavailable")
	mailer, err := NewMailer(&fakeSender{err: sendErr}, From{Address: "no-reply@plateau.local"}, templates)
	if err != nil {
		t.Fatalf("NewMailer() error = %v", err)
	}
	if err := mailer.SendVerification(t.Context(), "person@example.com", "https://iam.example/verify#token", 24); !errors.Is(err, sendErr) {
		t.Fatalf("SendVerification() error = %v, want SMTP error", err)
	}
}

func TestMailConfigurationIsRequired(t *testing.T) {
	if _, err := NewSender(nil); err == nil {
		t.Fatal("NewSender(nil) error = nil")
	}
	if _, err := NewFrom(nil); err == nil {
		t.Fatal("NewFrom(nil) error = nil")
	}
	config := &mailpb.Mail{
		Smtp: &mailpb.SMTP{Host: "localhost"},
		From: &mailpb.MailFrom{Address: "no-reply@plateau.local", Name: "Plateau IAM"},
	}
	if _, err := NewSender(config); err != nil {
		t.Fatalf("NewSender(valid) error = %v", err)
	}
	if from, err := NewFrom(config); err != nil || from.Address != "no-reply@plateau.local" {
		t.Fatalf("NewFrom(valid) = %#v, %v", from, err)
	}
}
