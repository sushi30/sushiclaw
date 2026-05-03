# Email Channel

## `allow_from`

The email channel uses the shared structured sender authorization path. Even
though `allow_from` is configured inside `channels.email`, inbound email
senders are matched by their canonical sender ID, not by a raw email address.

Use canonical entries like:

```json
{
  "channels": {
    "email": {
      "allow_from": ["email:user@example.com"]
    }
  }
}
```

Do not use plain addresses like `user@example.com`. They no longer match
inbound email senders.

## Why Canonical IDs

The current authorization logic is shared across structured channels such as
email, Telegram, and WhatsApp. Canonical IDs keep that shared matcher
deterministic:

- `email:user@example.com`
- `telegram:123456`
- `whatsapp:+1234567890`

This avoids channel-specific fallback behavior and makes the structured sender
path the single source of truth for authorization.

## Scope of Allowlisting

`allow_from` is channel-scoped. There is currently no global cross-channel
identity allowlist.

That means:

- `channels.email.allow_from` controls who may message the agent by email
- `channels.telegram.allow_from` controls who may message the agent on Telegram
- allowing someone on one channel does not allow them on another channel

If you want the same person allowed on multiple channels, add the appropriate
sender ID to each channel's `allow_from` list.

## Related Control

`SUSHICLAW_EXEC_ALLOWED_SENDERS` is separate from inbound allowlisting. It
controls which remote senders may use the `exec` tool and is not a general
communication allowlist.
