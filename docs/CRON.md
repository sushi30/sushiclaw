# Cron Jobs

sushiclaw can schedule future agent turns, direct channel messages, or optional shell commands through
the `cron` tool. The scheduler runs inside `sushiclaw gateway` and stores jobs in the configured
workspace at `cron/jobs.json`.

## Enable The Tool

Enable cron in `config.json`:

```json
{
  "tools": {
    "cron": {
      "enabled": true,
      "allow_command": false,
      "exec_timeout_minutes": 5,
      "timezone": "Europe/Amsterdam"
    }
  }
}
```

`allow_command` controls whether cron jobs may run shell commands. Keep it disabled unless the
configured workspace and allowed senders are trusted.

## How Jobs Run

Cron jobs support three schedule types:

- `at_seconds`: run once after a delay.
- `every_seconds`: run repeatedly at an interval.
- `cron_expr`: run on a standard cron expression.

`cron_expr` uses local wall-clock time in the job `timezone`. If no timezone is supplied when the
job is created, the scheduler uses `tools.cron.timezone`, then falls back to `UTC`. Do not convert
requested local times to UTC before writing the cron expression.

Jobs can run in three modes:

- Agent turn: sends the saved prompt back through the agent, so the model can reason and use tools.
- Direct delivery: sends the saved message directly to the original channel without an agent turn.
- Command: runs a shell command and sends the command output back to the original channel.

The agent fills in the channel, chat ID, and sender ID from the conversation where the job is
created.

The scheduler persists execution state in `cron/jobs.json`: next run, running marker, last run
status, last error, duration, and consecutive errors. On gateway restart it marks stale running jobs
as interrupted and catches up a bounded number of missed jobs.

## Example Prompts

Ask the agent naturally from Telegram, WhatsApp, email, or chat:

```text
Remind me in 10 minutes to check the rice.
```

```text
Every morning at 9, ask me what the most important task is today.
```

```text
Schedule a direct message every 30 minutes that says "drink water".
```

```text
At 18:00 on weekdays, run an agent check-in: summarize my open priorities and ask what changed.
```

```text
Create a cron job named weekly-review for Mondays at 08:30 that asks me to review last week's notes.
```

```text
List my cron jobs.
```

```text
Disable the weekly-review cron job.
```

```text
Remove the weekly-review cron job.
```

```text
Run the weekly-review cron job now.
```

```text
Show cron status.
```

If command jobs are enabled:

```text
Every hour, run `git -C /home/imri/workspace status --short` and send me the output.
```

## Slash Commands

Use `/list cron` to list scheduled cron jobs without asking the agent to use the tool:

```text
/list cron
```

The response shows each job name, whether it is disabled, how it runs, and its schedule.
It also includes next run, last status, and last error when available.

## CLI

Cron jobs can also be managed from the binary:

```bash
sushiclaw cron add --name daily-check --message "What should I focus on today?" --cron "0 9 * * *" --channel telegram --chat-id 12345
sushiclaw cron list
sushiclaw cron remove daily-check
```

CLI-created jobs are persisted immediately. Restart a running gateway after adding jobs through the
CLI so the scheduler loads them.
