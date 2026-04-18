# Release Notes

## Email channel config migration (fix/email4)

### What changed

The email channel is wired manually outside picoclaw's channel registry.
This means email config must live under the top-level `email_channel` key,
**not** inside the `channels` map.

If your config has `channels.email`, startup will fail with:

```
error loading config: channel "email" has unknown type "email"
```

### Required config migration

Open `~/.picoclaw/config.json` (or `$SUSHICLAW_CONFIG`) and move the email
block from `channels` to a top-level `email_channel` key.

**Before:**

```json
{
  "channels": {
    "whatsapp": { ... },
    "email": {
      "enabled": true,
      "smtp_host": "smtp.gmail.com",
      "smtp_port": 587,
      "smtp_from": "env://SMTP_USER",
      "smtp_user": "env://SMTP_USER",
      "smtp_password": "env://SMTP_PASSWORD",
      "imap_host": "imap.gmail.com",
      "imap_port": 993,
      "imap_user": "env://SMTP_USER",
      "imap_password": "env://SMTP_PASSWORD",
      "poll_interval_secs": 10,
      "allow_from": ["you@example.com"]
    }
  }
}
```

**After:**

```json
{
  "channels": {
    "whatsapp": { ... }
  },
  "email_channel": {
    "enabled": true,
    "smtp_host": "smtp.gmail.com",
    "smtp_port": 587,
    "smtp_from": "env://SMTP_USER",
    "smtp_user": "env://SMTP_USER",
    "smtp_password": "env://SMTP_PASSWORD",
    "imap_host": "imap.gmail.com",
    "imap_port": 993,
    "imap_user": "env://SMTP_USER",
    "imap_password": "env://SMTP_PASSWORD",
    "poll_interval_secs": 10,
    "allow_from": ["you@example.com"]
  }
}
```

### Reference: full email_channel options

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Enable/disable the email channel |
| `smtp_host` | string | SMTP server hostname |
| `smtp_port` | int | SMTP port (typically 587 for TLS, 465 for SSL) |
| `smtp_from` | string | Sender address (supports `env://VAR`) |
| `smtp_user` | string | SMTP auth username (supports `env://VAR`) |
| `smtp_password` | string | SMTP auth password (supports `env://VAR`) |
| `default_subject` | string | Subject for new outbound emails |
| `imap_host` | string | IMAP server hostname |
| `imap_port` | int | IMAP port (typically 993) |
| `imap_user` | string | IMAP username (supports `env://VAR`) |
| `imap_password` | string | IMAP password (supports `env://VAR`) |
| `poll_interval_secs` | int | How often to poll for new mail (seconds) |
| `allow_from` | []string | Allowlist of sender addresses that can message the agent |
| `reasoning_channel_id` | string | Channel ID for extended reasoning responses |
