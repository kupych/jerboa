package email

import (
	"fmt"
	"html"
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

func (m *Mailer) SendPlain(to, subject, body string) error {
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

	return m.send(to, subject, msg)
}

func (m *Mailer) SendHTML(to, subject, htmlBody string) error {
	if !m.Enabled() {
		slog.Warn("email not configured, skipping", "to", to, "subject", subject)
		return nil
	}

	msg := strings.Join([]string{
		"From: " + m.from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=utf-8",
		"",
		htmlBody,
	}, "\r\n")

	return m.send(to, subject, msg)
}

func (m *Mailer) send(to, subject, msg string) error {
	auth := smtp.PlainAuth("", m.user, m.pass, m.host)
	addr := fmt.Sprintf("%s:%s", m.host, m.port)

	if err := smtp.SendMail(addr, auth, m.from, []string{to}, []byte(msg)); err != nil {
		slog.Error("send email", "to", to, "error", err)
		return err
	}

	slog.Info("email sent", "to", to, "subject", subject)
	return nil
}

func (m *Mailer) SendMagicLoginEmail(to, loginURL string) {
	subject := "Your Jerboa login link"
	safeURL := html.EscapeString(loginURL)
	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width"></head>
<body style="margin:0;padding:0;background:#131313;font-family:'Courier New',monospace;">
  <div style="max-width:480px;margin:0 auto;padding:40px 24px;">
    <div style="border-bottom:1px solid #222;padding-bottom:16px;margin-bottom:32px;">
      <span style="font-size:11px;font-weight:700;letter-spacing:0.25em;color:#555;text-transform:uppercase;">JERBOA · SYS·MSG·01</span>
    </div>

    <p style="font-size:14px;color:#ccc;line-height:1.6;margin:0 0 24px;">
      Click the link below to sign in to Jerboa. This link expires in 15 minutes.
    </p>

    <a href="%s" style="display:inline-block;background:#c8a864;color:#131313;font-size:11px;font-weight:700;letter-spacing:0.15em;text-transform:uppercase;text-decoration:none;padding:12px 28px;margin:0 0 32px;">SIGN IN</a>

    <p style="font-size:11px;color:#555;line-height:1.5;margin:0 0 32px;">
      If you didn't request this, you can safely ignore it.
    </p>

    <div style="border-top:1px solid #222;padding-top:16px;margin-top:32px;">
      <span style="font-size:8px;font-weight:600;letter-spacing:0.25em;color:rgba(255,255,255,0.1);text-transform:uppercase;">whether.network</span>
    </div>
  </div>
</body>
</html>`, safeURL)

	go m.SendHTML(to, subject, htmlBody)
}

func (m *Mailer) SendInvite(to, bandName, inviteURL string) {
	safeBand := html.EscapeString(bandName)
	safeURL := html.EscapeString(inviteURL)
	subject := fmt.Sprintf("You're invited to %s on Jerboa", bandName)
	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width"></head>
<body style="margin:0;padding:0;background:#131313;font-family:'Courier New',monospace;">
  <div style="max-width:480px;margin:0 auto;padding:40px 24px;">
    <div style="border-bottom:1px solid #222;padding-bottom:16px;margin-bottom:32px;">
      <span style="font-size:11px;font-weight:700;letter-spacing:0.25em;color:#555;text-transform:uppercase;">JERBOA · SYS·MSG·01</span>
    </div>

    <p style="font-size:14px;color:#ccc;line-height:1.6;margin:0 0 24px;">
      You've been invited to join <strong style="color:#fff;">%s</strong> on Jerboa.
    </p>

    <p style="font-size:12px;color:#888;line-height:1.6;margin:0 0 24px;">
      Jerboa is a private space for bands to share recordings, track ideas, and collaborate on music together. Think of it as your band's shared notebook &mdash; upload takes, leave comments, build setlists.
    </p>

    <a href="%s" style="display:inline-block;background:#c8a864;color:#131313;font-size:11px;font-weight:700;letter-spacing:0.15em;text-transform:uppercase;text-decoration:none;padding:12px 28px;margin:0 0 12px;">ACCEPT INVITE</a>

    <p style="font-size:10px;color:#555;line-height:1.5;margin:0 0 32px;">
      This link is also your login &mdash; bookmark it to sign in anytime.
    </p>

    <div style="border-top:1px solid #222;padding-top:16px;margin-top:32px;">
      <span style="font-size:8px;font-weight:600;letter-spacing:0.25em;color:rgba(255,255,255,0.1);text-transform:uppercase;">whether.network</span>
    </div>
  </div>
</body>
</html>`, safeBand, safeURL)

	// Send in background, don't block the request
	go m.SendHTML(to, subject, htmlBody)
}
