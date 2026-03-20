package email

import (
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"

	"jerboa/internal/config"
)

type Mailer struct {
	host string
	port string
	user string
	pass string
	from string
}

func NewMailer(cfg *config.Config) *Mailer {
	return &Mailer{
		host: cfg.SMTPHost,
		port: cfg.SMTPPort,
		user: cfg.SMTPUser,
		pass: cfg.SMTPPass,
		from: cfg.SMTPFrom,
	}
}

func (m *Mailer) Enabled() bool {
	return m.host != "" && m.from != ""
}

func (m *Mailer) Send(to, subject, body string) error {
	if !m.Enabled() {
		slog.Warn("email not configured, skipping", "to", to, "subject", subject)
		return nil
	}

	msg := strings.Join([]string{
		"From: " + m.from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		body,
	}, "\r\n")

	auth := smtp.PlainAuth("", m.user, m.pass, m.host)
	addr := fmt.Sprintf("%s:%s", m.host, m.port)

	if err := smtp.SendMail(addr, auth, m.from, []string{to}, []byte(msg)); err != nil {
		slog.Error("send email", "to", to, "error", err)
		return err
	}

	slog.Info("email sent", "to", to, "subject", subject)
	return nil
}

func (m *Mailer) SendInvite(to, bandName, inviteURL string) {
	subject := fmt.Sprintf("You're invited to %s on Jerboa", bandName)
	body := fmt.Sprintf(`Hey!

You've been invited to join %s on Jerboa.

Click the link below to accept:
%s

See you there.`, bandName, inviteURL)

	// Send in background, don't block the request
	go m.Send(to, subject, body)
}
