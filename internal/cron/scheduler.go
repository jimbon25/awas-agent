package cron

import (
	"awas/internal/config"
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type MessageDeliverer interface {
	DeliverMessage(platform string, chatID string, guildID string, text string) error
}

type AgentRunner func(ctx context.Context, cfg *config.Config, prompt string, ui *CronUI)

type Scheduler struct {
	store    *Store
	mgr      MessageDeliverer
	cfg      *config.Config
	runner   AgentRunner
	running  map[string]context.CancelFunc
	mu       sync.Mutex
	stopChan chan struct{}
	wg       sync.WaitGroup
}

func NewScheduler(store *Store, mgr MessageDeliverer, cfg *config.Config, runner AgentRunner) *Scheduler {
	return &Scheduler{
		store:    store,
		mgr:      mgr,
		cfg:      cfg,
		runner:   runner,
		running:  make(map[string]context.CancelFunc),
		stopChan: make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		log.Printf("[cron] Scheduler loop started.")

		for {
			select {
			case <-s.stopChan:
				return
			case <-ticker.C:
				s.runDueJobs()
			}
		}
	}()
}

func (s *Scheduler) Stop() {
	close(s.stopChan)
	s.wg.Wait()

	s.mu.Lock()
	for _, cancel := range s.running {
		cancel()
	}
	s.mu.Unlock()

	log.Printf("[cron] Scheduler loop stopped.")
}

func (s *Scheduler) RunJob(name string) (string, error) {
	job, err := s.store.GetJob(name)
	if err != nil {
		return "", fmt.Errorf("job %q not found: %w", name, err)
	}

	s.mu.Lock()
	if _, isRunning := s.running[name]; isRunning {
		s.mu.Unlock()
		return "", fmt.Errorf("job %q is already running", name)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.running[name] = cancel
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.running, name)
			cancel()
			s.mu.Unlock()
		}()
		s.executeJob(ctx, job)
	}()

	return "", nil
}

func (s *Scheduler) runDueJobs() {
	jobs, err := s.store.ListJobs()
	if err != nil {
		log.Printf("[cron] Failed to list jobs: %v", err)
		return
	}

	now := time.Now()

	for _, job := range jobs {
		if !job.Enabled {
			continue
		}

		s.mu.Lock()
		_, isRunning := s.running[job.Name]
		s.mu.Unlock()
		if isRunning {
			continue
		}

		if job.LastRun != nil && job.LastRun.Truncate(time.Minute).Equal(now.Truncate(time.Minute)) {
			continue
		}

		if MatchCron(job.Schedule, now) {
			s.mu.Lock()
			ctx, cancel := context.WithCancel(context.Background())
			s.running[job.Name] = cancel
			s.mu.Unlock()

			go func(j *CronJob) {
				defer func() {
					s.mu.Lock()
					delete(s.running, j.Name)
					s.mu.Unlock()
					cancel()
				}()

				s.executeJob(ctx, j)
			}(job)
		}
	}
}

func (s *Scheduler) executeJob(ctx context.Context, job *CronJob) {
	log.Printf("[cron] Running scheduled job: %s", job.Name)

	jobCfg := *s.cfg
	if job.Model != "" {
		jobCfg.Model = job.Model
	}
	if job.WorkDir != "" {
		jobCfg.WorkDir = job.WorkDir
	}

	runCtx, runCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer runCancel()

	cronUI := NewCronUI()

	s.runner(runCtx, &jobCfg, job.Prompt, cronUI)

	output := cronUI.GetOutput()
	if output == "" {
		output = "(No output produced by the agent)"
	}

	formattedTime := time.Now().Format("2006-01-02 15:04:05")
	finalMsg := fmt.Sprintf("⎔ **Cron: %s** (%s)\n──────────────────\n%s\n──────────────────\nSchedule: %s",
		job.Name, formattedTime, output, job.Schedule)

	err := s.mgr.DeliverMessage(job.Platform, job.ChatID, job.GuildID, finalMsg)
	if err != nil {
		log.Printf("[cron] Delivery failed for job %s: %v", job.Name, err)
	}

	now := time.Now()
	job.LastRun = &now
	job.RunCount++

	if err := s.store.SaveJob(job); err != nil {
		log.Printf("[cron] Failed to save execution stats: %v", err)
	}
}
