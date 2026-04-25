package cron

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/logger"
	"github.com/sushi30/sushiclaw/pkg/tools/exec"
)

// Scheduler manages the lifecycle and execution of cron jobs.
type Scheduler struct {
	store   *Store
	cron    *cron.Cron
	timers  map[string]*time.Timer
	entries map[string]cron.EntryID
	bus     *bus.MessageBus
	cfg     *config.Config
	mu      sync.RWMutex
}

// NewScheduler loads existing jobs and schedules enabled ones.
func NewScheduler(cfg *config.Config, messageBus *bus.MessageBus) (*Scheduler, error) {
	storePath := cfg.WorkspacePath() + "/cron/jobs.json"
	store := NewStore(storePath)
	jobs, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("load cron jobs: %w", err)
	}

	s := &Scheduler{
		store:   store,
		cron:    cron.New(),
		timers:  make(map[string]*time.Timer),
		entries: make(map[string]cron.EntryID),
		bus:     messageBus,
		cfg:     cfg,
	}

	for _, job := range jobs {
		if job.Enabled {
			s.scheduleJob(job)
		}
	}

	return s, nil
}

// Start begins the cron runner.
func (s *Scheduler) Start() {
	s.cron.Start()
}

// Stop halts the cron runner and cancels pending timers.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cron.Stop()
	for _, t := range s.timers {
		t.Stop()
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
	jobs = append(jobs, job)
	if err := s.store.Save(jobs); err != nil {
		return err
	}
	if job.Enabled {
		s.scheduleJob(job)
	}
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
	s.unscheduleJob(name)
	return nil
}

// EnableJob enables and schedules a job.
func (s *Scheduler) EnableJob(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.store.Load()
	if err != nil {
		return err
	}
	for i := range jobs {
		if jobs[i].Name == name {
			if jobs[i].Enabled {
				return nil
			}
			jobs[i].Enabled = true
			if err := s.store.Save(jobs); err != nil {
				return err
			}
			s.scheduleJob(jobs[i])
			return nil
		}
	}
	return fmt.Errorf("job %q not found", name)
}

// DisableJob disables and unschedules a job.
func (s *Scheduler) DisableJob(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.store.Load()
	if err != nil {
		return err
	}
	for i := range jobs {
		if jobs[i].Name == name {
			if !jobs[i].Enabled {
				return nil
			}
			jobs[i].Enabled = false
			if err := s.store.Save(jobs); err != nil {
				return err
			}
			s.unscheduleJob(name)
			return nil
		}
	}
	return fmt.Errorf("job %q not found", name)
}

// ListJobs returns all persisted jobs.
func (s *Scheduler) ListJobs() ([]Job, error) {
	return s.store.Load()
}

func (s *Scheduler) scheduleJob(job Job) {
	if !job.Enabled {
		return
	}

	// Priority: at_seconds > every_seconds > cron_expr
	switch {
	case job.AtSeconds != nil:
		runAt := job.CreatedAt.Add(time.Duration(*job.AtSeconds) * time.Second)
		now := time.Now()
		if now.After(runAt) {
			go s.executeJob(job)
			_ = s.RemoveJob(job.Name)
			return
		}
		d := runAt.Sub(now)
		s.timers[job.Name] = time.AfterFunc(d, func() {
			s.executeJob(job)
			_ = s.RemoveJob(job.Name)
		})
	case job.EverySeconds != nil:
		schedule := cron.Every(time.Duration(*job.EverySeconds) * time.Second)
		id := s.cron.Schedule(schedule, cron.FuncJob(func() {
			s.executeJob(job)
		}))
		s.entries[job.Name] = id
	case job.CronExpr != "":
		id, err := s.cron.AddFunc(job.CronExpr, func() {
			s.executeJob(job)
		})
		if err != nil {
			logger.ErrorCF("cron", "Invalid cron expression", map[string]any{
				"job":   job.Name,
				"expr":  job.CronExpr,
				"error": err.Error(),
			})
			return
		}
		s.entries[job.Name] = id
	}
}

func (s *Scheduler) unscheduleJob(name string) {
	if timer, ok := s.timers[name]; ok {
		timer.Stop()
		delete(s.timers, name)
	}
	if entryID, ok := s.entries[name]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, name)
	}
}

func (s *Scheduler) executeJob(job Job) {
	// Reload to respect disable/remove that happened after scheduling.
	jobs, err := s.store.Load()
	if err != nil {
		logger.ErrorCF("cron", "Failed to reload job", map[string]any{"error": err.Error()})
		return
	}
	var current *Job
	for i := range jobs {
		if jobs[i].Name == job.Name {
			current = &jobs[i]
			break
		}
	}
	if current == nil || !current.Enabled {
		return
	}

	logger.InfoCF("cron", "Executing job", map[string]any{
		"name":    current.Name,
		"channel": current.Channel,
		"chat_id": current.ChatID,
	})

	ctx := context.Background()

	if current.Command != "" {
		s.executeCommandJob(ctx, *current)
		return
	}

	if current.Deliver {
		s.deliverMessage(ctx, *current)
		return
	}

	s.agentTurn(ctx, *current)
}

func (s *Scheduler) agentTurn(ctx context.Context, job Job) {
	msg := bus.InboundMessage{
		Context: bus.InboundContext{
			Channel:  job.Channel,
			ChatID:   job.ChatID,
			SenderID: job.SenderID,
		},
		Sender: bus.SenderInfo{
			CanonicalID: job.SenderID,
		},
		Content: job.Message,
	}
	if err := s.bus.PublishInbound(ctx, msg); err != nil {
		logger.ErrorCF("cron", "Failed to publish inbound cron job", map[string]any{
			"job":   job.Name,
			"error": err.Error(),
		})
	}
}

func (s *Scheduler) deliverMessage(ctx context.Context, job Job) {
	msg := bus.OutboundMessage{
		Channel: job.Channel,
		ChatID:  job.ChatID,
		Context: bus.NewOutboundContext(job.Channel, job.ChatID, ""),
		Content: job.Message,
	}
	if err := s.bus.PublishOutbound(ctx, msg); err != nil {
		logger.ErrorCF("cron", "Failed to publish outbound cron job", map[string]any{
			"job":   job.Name,
			"error": err.Error(),
		})
	}
}

func (s *Scheduler) executeCommandJob(ctx context.Context, job Job) {
	if !s.cfg.Tools.IsToolEnabled("exec") {
		logger.WarnCF("cron", "Exec tool disabled, skipping command job", map[string]any{
			"job": job.Name,
		})
		return
	}

	timeout := time.Duration(s.cfg.Tools.Cron.ExecTimeoutMinutes) * time.Minute
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	execTool := exec.NewExecTool(s.cfg.WorkspacePath(), s.cfg.Agents.Defaults.RestrictToWorkspace, true)
	output, err := execTool.Run(execCtx, job.Command)

	content := output
	if err != nil {
		content = fmt.Sprintf("Error: %v\nOutput: %s", err, output)
	}

	msg := bus.OutboundMessage{
		Channel: job.Channel,
		ChatID:  job.ChatID,
		Context: bus.NewOutboundContext(job.Channel, job.ChatID, ""),
		Content: content,
	}
	if err := s.bus.PublishOutbound(ctx, msg); err != nil {
		logger.ErrorCF("cron", "Failed to publish command job output", map[string]any{
			"job":   job.Name,
			"error": err.Error(),
		})
	}
}
