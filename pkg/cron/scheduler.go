package cron

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/logger"
	"github.com/sushi30/sushiclaw/pkg/tools"
	"github.com/sushi30/sushiclaw/pkg/tools/exec"
)

const (
	defaultTimezone               = "UTC"
	startupInterruptedError       = "cron: job interrupted by gateway restart"
	defaultStartupCatchupLimit    = 5
	defaultStartupCatchupStagger  = 5 * time.Second
	defaultSchedulerMaxTimerDelay = time.Minute
)

// AgentRunner executes cron agent turns synchronously so the scheduler can
// persist success/failure instead of only enqueueing an inbound message.
type AgentRunner interface {
	RunCronAgentTurn(ctx context.Context, msg bus.InboundMessage) (AgentRunResult, error)
}

type AgentRunResult struct {
	Response      string
	ToolCalls     int
	ResponseBytes int
}

// Scheduler manages the lifecycle and execution of cron jobs.
type Scheduler struct {
	store       *Store
	timer       *time.Timer
	bus         *bus.MessageBus
	cfg         *config.Config
	agentRunner AgentRunner
	started     bool
	stopped     bool
	mu          sync.Mutex
}

// NewScheduler loads existing jobs and prepares the scheduler.
func NewScheduler(cfg *config.Config, messageBus *bus.MessageBus) (*Scheduler, error) {
	storePath := cfg.WorkspacePath() + "/cron/jobs.json"
	store := NewStore(storePath)
	jobs, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("load cron jobs: %w", err)
	}

	s := &Scheduler{
		store: store,
		bus:   messageBus,
		cfg:   cfg,
	}
	if changed := s.recomputeNextRunsLocked(jobs, time.Now(), true); changed {
		if err := s.store.Save(jobs); err != nil {
			return nil, fmt.Errorf("persist cron state: %w", err)
		}
	}
	return s, nil
}

func (s *Scheduler) SetAgentRunner(r AgentRunner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentRunner = r
}

// Start begins the cron runner.
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.stopped = false
	s.mu.Unlock()

	s.markInterruptedRuns()
	s.runMissedJobs()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.armTimerLocked(time.Now())
}

// Stop halts the cron runner and cancels pending timers.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

// AddJob persists a new job and schedules it if enabled.
func (s *Scheduler) AddJob(job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.store.Load()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.Name == job.Name {
			return fmt.Errorf("job %q already exists", job.Name)
		}
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	if job.Timezone == "" && job.CronExpr != "" {
		job.Timezone = s.defaultTimezone()
	}
	if job.Enabled {
		next, err := s.computeNextRun(job, time.Now())
		if err != nil {
			job.State.LastStatus = StatusError
			job.State.LastError = "schedule error: " + err.Error()
			job.State.ConsecutiveErrors++
		} else {
			job.State.NextRunAt = next
		}
	}
	jobs = append(jobs, job)
	if err := s.store.Save(jobs); err != nil {
		return err
	}
	s.armTimerLocked(time.Now())
	return nil
}

// RemoveJob deletes a job and unschedules it.
func (s *Scheduler) RemoveJob(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.store.Load()
	if err != nil {
		return err
	}
	found := false
	for i, j := range jobs {
		if j.Name == name {
			jobs = append(jobs[:i], jobs[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("job %q not found", name)
	}
	if err := s.store.Save(jobs); err != nil {
		return err
	}
	s.armTimerLocked(time.Now())
	return nil
}

// EnableJob enables and schedules a job.
func (s *Scheduler) EnableJob(name string) error {
	return s.updateJob(name, func(job *Job) error {
		job.Enabled = true
		next, err := s.computeNextRun(*job, time.Now())
		if err != nil {
			return err
		}
		job.State.NextRunAt = next
		return nil
	})
}

// DisableJob disables and unschedules a job.
func (s *Scheduler) DisableJob(name string) error {
	return s.updateJob(name, func(job *Job) error {
		job.Enabled = false
		job.State.NextRunAt = nil
		job.State.RunningAt = nil
		return nil
	})
}

func (s *Scheduler) updateJob(name string, fn func(*Job) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.store.Load()
	if err != nil {
		return err
	}
	for i := range jobs {
		if jobs[i].Name != name {
			continue
		}
		if err := fn(&jobs[i]); err != nil {
			return err
		}
		if err := s.store.Save(jobs); err != nil {
			return err
		}
		s.armTimerLocked(time.Now())
		return nil
	}
	return fmt.Errorf("job %q not found", name)
}

// ListJobs returns all persisted jobs.
func (s *Scheduler) ListJobs() ([]Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs, err := s.store.Load()
	if err != nil {
		return nil, err
	}
	if changed := s.recomputeNextRunsLocked(jobs, time.Now(), false); changed {
		if err := s.store.Save(jobs); err != nil {
			return nil, err
		}
	}
	return jobs, nil
}

func (s *Scheduler) Status() (string, error) {
	jobs, err := s.ListJobs()
	if err != nil {
		return "", err
	}
	next := nextWake(jobs)
	if next == nil {
		return fmt.Sprintf("Cron scheduler enabled. Jobs: %d. No next run scheduled.", len(jobs)), nil
	}
	return fmt.Sprintf("Cron scheduler enabled. Jobs: %d. Next run: %s.", len(jobs), next.UTC().Format(time.RFC3339)), nil
}

func (s *Scheduler) RunJob(name string, force bool) error {
	job, ok, err := s.claimJob(name, time.Now(), force)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("job %q is not due", name)
	}
	go s.executeClaimedJob(job)
	return nil
}

func (s *Scheduler) tick() {
	now := time.Now()
	for {
		job, ok, err := s.claimNextDueJob(now)
		if err != nil {
			logger.ErrorCF("cron", "Failed to claim due job", map[string]any{"error": err.Error()})
			break
		}
		if !ok {
			break
		}
		go s.executeClaimedJob(job)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.armTimerLocked(time.Now())
}

func (s *Scheduler) claimNextDueJob(now time.Time) (Job, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.store.Load()
	if err != nil {
		return Job{}, false, err
	}
	s.recomputeNextRunsLocked(jobs, now, false)
	for i := range jobs {
		if !isDue(jobs[i], now, false) || jobs[i].State.RunningAt != nil {
			continue
		}
		runningAt := now.UTC()
		jobs[i].State.RunningAt = &runningAt
		jobs[i].State.LastError = ""
		job := jobs[i]
		if err := s.store.Save(jobs); err != nil {
			return Job{}, false, err
		}
		return job, true, nil
	}
	if err := s.store.Save(jobs); err != nil {
		return Job{}, false, err
	}
	return Job{}, false, nil
}

func (s *Scheduler) claimJob(name string, now time.Time, force bool) (Job, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.store.Load()
	if err != nil {
		return Job{}, false, err
	}
	s.recomputeNextRunsLocked(jobs, now, false)
	for i := range jobs {
		if jobs[i].Name != name {
			continue
		}
		if jobs[i].State.RunningAt != nil {
			return Job{}, false, fmt.Errorf("job %q is already running", name)
		}
		if !isDue(jobs[i], now, force) {
			return Job{}, false, nil
		}
		runningAt := now.UTC()
		jobs[i].State.RunningAt = &runningAt
		jobs[i].State.LastError = ""
		job := jobs[i]
		if err := s.store.Save(jobs); err != nil {
			return Job{}, false, err
		}
		return job, true, nil
	}
	return Job{}, false, fmt.Errorf("job %q not found", name)
}

func (s *Scheduler) executeClaimedJob(job Job) {
	start := time.Now()
	err := s.runJob(context.Background(), job)
	status := StatusOK
	errText := ""
	if err != nil {
		status = StatusError
		errText = err.Error()
		logger.ErrorCF("cron", "Cron job failed", map[string]any{"job": job.Name, "error": errText})
		s.notifyFailure(context.Background(), job, errText)
	}
	s.finishJob(job.Name, status, errText, start, time.Now())
}

// executeJob is retained for focused tests and manual execution paths.
func (s *Scheduler) executeJob(job Job) {
	start := time.Now()
	err := s.runJob(context.Background(), job)
	status := StatusOK
	errText := ""
	if err != nil {
		status = StatusError
		errText = err.Error()
	}
	s.finishJob(job.Name, status, errText, start, time.Now())
}

func (s *Scheduler) runJob(ctx context.Context, job Job) error {
	target := job.targetContext()
	logger.InfoCF("cron", "Executing job", map[string]any{
		"name":    job.Name,
		"channel": target.Channel,
		"chat_id": target.ChatID,
	})
	if target.Channel == "" || target.ChatID == "" {
		return fmt.Errorf("missing routing context")
	}
	if job.Command != "" {
		return s.executeCommandJob(ctx, job)
	}
	if job.Deliver {
		return s.deliverMessage(ctx, job)
	}
	return s.agentTurn(ctx, job)
}

func (s *Scheduler) finishJob(name, status, errText string, startedAt, endedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.store.Load()
	if err != nil {
		logger.ErrorCF("cron", "Failed to persist job result", map[string]any{"job": name, "error": err.Error()})
		return
	}
	now := endedAt.UTC()
	for i := range jobs {
		if jobs[i].Name != name {
			continue
		}
		jobs[i].State.RunningAt = nil
		jobs[i].State.LastRunAt = &now
		jobs[i].State.LastStatus = status
		jobs[i].State.LastError = errText
		jobs[i].State.LastDurationMS = endedAt.Sub(startedAt).Milliseconds()
		if status == StatusOK {
			jobs[i].State.ConsecutiveErrors = 0
		} else {
			jobs[i].State.ConsecutiveErrors++
		}
		if jobs[i].AtSeconds != nil && status == StatusOK {
			jobs[i].Enabled = false
			jobs[i].State.NextRunAt = nil
		} else if jobs[i].Enabled {
			next, err := s.computeNextRun(jobs[i], endedAt)
			if err != nil {
				jobs[i].State.LastStatus = StatusError
				jobs[i].State.LastError = "schedule error: " + err.Error()
				jobs[i].State.ConsecutiveErrors++
				jobs[i].State.NextRunAt = nil
			} else {
				jobs[i].State.NextRunAt = next
			}
		}
		break
	}
	if err := s.store.Save(jobs); err != nil {
		logger.ErrorCF("cron", "Failed to save job result", map[string]any{"job": name, "error": err.Error()})
		return
	}
	s.armTimerLocked(time.Now())
}

func (s *Scheduler) agentTurn(ctx context.Context, job Job) error {
	target := job.targetContext()
	msg := bus.InboundMessage{
		Context: target,
		Sender: bus.SenderInfo{
			CanonicalID: target.SenderID,
		},
		Content:    job.Message,
		SessionKey: cronSessionKey(job),
	}
	if s.agentRunner != nil {
		_, err := s.agentRunner.RunCronAgentTurn(ctx, msg)
		return err
	}
	return s.bus.PublishInbound(ctx, msg)
}

func (s *Scheduler) deliverMessage(ctx context.Context, job Job) error {
	target := job.targetContext()
	msg := bus.OutboundMessage{
		Channel: target.Channel,
		ChatID:  target.ChatID,
		Context: target,
		Content: job.Message,
	}
	msg = bus.MarkSystemOutboundMessage(msg)
	return s.bus.PublishOutbound(ctx, msg)
}

func (s *Scheduler) executeCommandJob(ctx context.Context, job Job) error {
	if !s.cfg.Tools.IsToolEnabled("exec") {
		return fmt.Errorf("exec tool disabled")
	}

	timeout := time.Duration(s.cfg.Tools.Cron.ExecTimeoutMinutes) * time.Minute
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	execTool := tools.WithDebugLogging(exec.NewExecTool(s.cfg.WorkspacePath(), s.cfg.Agents.Defaults.RestrictToWorkspace, true))
	output, err := execTool.Run(execCtx, job.Command)

	content := output
	if err != nil {
		content = fmt.Sprintf("Error: %v\nOutput: %s", err, output)
	}

	target := job.targetContext()
	msg := bus.OutboundMessage{
		Channel:    target.Channel,
		ChatID:     target.ChatID,
		Context:    target,
		Content:    content,
		SessionKey: target.Channel + ":" + target.ChatID,
	}
	msg = bus.MarkSystemOutboundMessage(msg)
	if publishErr := s.bus.PublishOutbound(ctx, msg); publishErr != nil {
		return publishErr
	}
	return err
}

func (s *Scheduler) notifyFailure(ctx context.Context, job Job, errText string) {
	target := job.targetContext()
	if target.Channel == "" || target.ChatID == "" {
		return
	}
	msg := bus.MarkSystemOutboundMessage(bus.OutboundMessage{
		Channel: target.Channel,
		ChatID:  target.ChatID,
		Context: target,
		Content: fmt.Sprintf("Cron job %q failed: %s", job.Name, errText),
	})
	if err := s.bus.PublishOutbound(ctx, msg); err != nil {
		logger.ErrorCF("cron", "Failed to publish cron failure notification", map[string]any{"job": job.Name, "error": err.Error()})
	}
}

func (s *Scheduler) markInterruptedRuns() {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.store.Load()
	if err != nil {
		logger.ErrorCF("cron", "Failed to load jobs on startup", map[string]any{"error": err.Error()})
		return
	}
	now := time.Now().UTC()
	changed := false
	for i := range jobs {
		if jobs[i].State.RunningAt == nil {
			continue
		}
		jobs[i].State.LastRunAt = jobs[i].State.RunningAt
		jobs[i].State.RunningAt = nil
		jobs[i].State.LastStatus = StatusError
		jobs[i].State.LastError = startupInterruptedError
		jobs[i].State.ConsecutiveErrors++
		changed = true
	}
	if s.recomputeNextRunsLocked(jobs, now, false) {
		changed = true
	}
	if changed {
		if err := s.store.Save(jobs); err != nil {
			logger.ErrorCF("cron", "Failed to save startup cron state", map[string]any{"error": err.Error()})
		}
	}
}

func (s *Scheduler) runMissedJobs() {
	for i := 0; i < defaultStartupCatchupLimit; i++ {
		job, ok, err := s.claimNextDueJob(time.Now())
		if err != nil {
			logger.ErrorCF("cron", "Failed startup catch-up claim", map[string]any{"error": err.Error()})
			return
		}
		if !ok {
			return
		}
		delay := time.Duration(i) * defaultStartupCatchupStagger
		time.AfterFunc(delay, func() {
			s.executeClaimedJob(job)
		})
	}
}

func (s *Scheduler) recomputeNextRunsLocked(jobs []Job, now time.Time, recomputeExisting bool) bool {
	changed := false
	for i := range jobs {
		if !jobs[i].Enabled {
			if jobs[i].State.NextRunAt != nil {
				jobs[i].State.NextRunAt = nil
				changed = true
			}
			continue
		}
		if jobs[i].CronExpr != "" && jobs[i].Timezone == "" {
			jobs[i].Timezone = defaultTimezone
			changed = true
		}
		if jobs[i].State.RunningAt != nil {
			continue
		}
		if jobs[i].State.NextRunAt != nil && !recomputeExisting {
			continue
		}
		next, err := s.computeNextRun(jobs[i], now)
		if err != nil {
			jobs[i].State.LastStatus = StatusError
			jobs[i].State.LastError = "schedule error: " + err.Error()
			jobs[i].State.ConsecutiveErrors++
			jobs[i].State.NextRunAt = nil
			changed = true
			continue
		}
		if !sameTimePtr(jobs[i].State.NextRunAt, next) {
			jobs[i].State.NextRunAt = next
			changed = true
		}
	}
	return changed
}

func (s *Scheduler) computeNextRun(job Job, now time.Time) (*time.Time, error) {
	switch {
	case job.AtSeconds != nil:
		if job.State.LastStatus == StatusOK && job.State.LastRunAt != nil {
			return nil, nil
		}
		runAt := job.CreatedAt.Add(time.Duration(*job.AtSeconds) * time.Second).UTC()
		return &runAt, nil
	case job.EverySeconds != nil:
		every := time.Duration(*job.EverySeconds) * time.Second
		if every <= 0 {
			return nil, fmt.Errorf("every_seconds must be positive")
		}
		anchor := job.CreatedAt
		if anchor.IsZero() {
			anchor = now
		}
		if job.State.LastRunAt != nil {
			next := job.State.LastRunAt.Add(every).UTC()
			if next.After(now) {
				return &next, nil
			}
		}
		elapsed := now.Sub(anchor)
		if elapsed < 0 {
			next := anchor.UTC()
			return &next, nil
		}
		steps := int64(math.Floor(float64(elapsed)/float64(every))) + 1
		next := anchor.Add(time.Duration(steps) * every).UTC()
		return &next, nil
	case job.CronExpr != "":
		loc, err := time.LoadLocation(resolveTimezone(job.Timezone))
		if err != nil {
			return nil, err
		}
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		schedule, err := parser.Parse(job.CronExpr)
		if err != nil {
			return nil, err
		}
		next := schedule.Next(now.In(loc)).UTC()
		if next.IsZero() {
			return nil, fmt.Errorf("no future run")
		}
		return &next, nil
	default:
		return nil, fmt.Errorf("missing schedule")
	}
}

func (s *Scheduler) armTimerLocked(now time.Time) {
	if s.stopped || !s.started {
		return
	}
	jobs, err := s.store.Load()
	if err != nil {
		logger.ErrorCF("cron", "Failed to load jobs for timer", map[string]any{"error": err.Error()})
		return
	}
	if s.recomputeNextRunsLocked(jobs, now, false) {
		if err := s.store.Save(jobs); err != nil {
			logger.ErrorCF("cron", "Failed to persist next runs", map[string]any{"error": err.Error()})
			return
		}
	}
	next := nextWake(jobs)
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if next == nil {
		return
	}
	delay := time.Until(*next)
	if delay < 0 {
		delay = 0
	}
	if delay > defaultSchedulerMaxTimerDelay {
		delay = defaultSchedulerMaxTimerDelay
	}
	s.timer = time.AfterFunc(delay, s.tick)
}

func (s *Scheduler) defaultTimezone() string {
	if s.cfg != nil && s.cfg.Tools.Cron.Timezone != "" {
		return s.cfg.Tools.Cron.Timezone
	}
	return defaultTimezone
}

func resolveTimezone(tz string) string {
	if tz == "" {
		return defaultTimezone
	}
	return tz
}

func isDue(job Job, now time.Time, force bool) bool {
	if force {
		return job.Enabled
	}
	if !job.Enabled || job.State.NextRunAt == nil {
		return false
	}
	return !job.State.NextRunAt.After(now)
}

func nextWake(jobs []Job) *time.Time {
	var next *time.Time
	for i := range jobs {
		if !jobs[i].Enabled || jobs[i].State.RunningAt != nil || jobs[i].State.NextRunAt == nil {
			continue
		}
		if next == nil || jobs[i].State.NextRunAt.Before(*next) {
			t := *jobs[i].State.NextRunAt
			next = &t
		}
	}
	return next
}

func sameTimePtr(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func (j Job) targetContext() bus.InboundContext {
	ctx := j.Context
	if ctx.Channel == "" {
		ctx.Channel = j.Channel
	}
	if ctx.ChatID == "" {
		ctx.ChatID = j.ChatID
	}
	if ctx.SenderID == "" {
		ctx.SenderID = j.SenderID
	}
	return bus.NormalizeInboundMessage(bus.InboundMessage{Context: ctx}).Context
}

func cronSessionKey(job Job) string {
	target := job.targetContext()
	return fmt.Sprintf("cron:%s:%s:%s", job.Name, target.Channel, target.ChatID)
}
