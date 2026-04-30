package conversationlock

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/resend/resend-go/v3"
)

type ResendSender struct {
	client *resend.Client
	from   string
}

func NewResendSender(apiKey, from string) (*ResendSender, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("resend api key is required")
	}
	if strings.TrimSpace(from) == "" {
		return nil, fmt.Errorf("from email is required")
	}
	return &ResendSender{
		client: resend.NewClient(apiKey),
		from:   strings.TrimSpace(from),
	}, nil
}

func (s *ResendSender) SendUnlockCode(ctx context.Context, to, code string, expiresIn time.Duration) error {
	minutes := int(expiresIn.Minutes())
	if minutes <= 0 {
		minutes = 1
	}
	params := &resend.SendEmailRequest{
		From:    s.from,
		To:      []string{strings.TrimSpace(to)},
		Subject: "Your sushiclaw unlock code",
		Text: fmt.Sprintf(
			"Your sushiclaw unlock code is %s. It expires in %d minutes. If you did not request this code, ignore this email.",
			code,
			minutes,
		),
		Html: fmt.Sprintf(
			"<p>Your sushiclaw unlock code is <strong>%s</strong>.</p><p>It expires in %d minutes. If you did not request this code, ignore this email.</p>",
			code,
			minutes,
		),
	}
	if _, err := s.client.Emails.SendWithContext(ctx, params); err != nil {
		return fmt.Errorf("send unlock email: %w", err)
	}
	return nil
}
