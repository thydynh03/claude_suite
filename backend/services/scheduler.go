package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type ScheduledJob struct {
	ID         string    `json:"id"`
	Prompt     string    `json:"prompt"`
	TargetTime time.Time `json:"target_time"`
	Repeat     bool      `json:"repeat"`
}

type SchedulerService struct {
	ctx       context.Context
	jobs      map[string]*ScheduledJob
	mu        sync.Mutex
	running   bool
	stopCh    chan struct{}
	onTrigger func(prompt string)
}

func NewSchedulerService() *SchedulerService {
	return &SchedulerService{
		jobs: make(map[string]*ScheduledJob),
	}
}

func (s *SchedulerService) SetContext(ctx context.Context) {
	s.ctx = ctx
}

func (s *SchedulerService) SetTriggerCallback(cb func(prompt string)) {
	s.onTrigger = cb
}

func (s *SchedulerService) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	go s.loop()
}

func (s *SchedulerService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()
}

func (s *SchedulerService) loop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.checkJobs()
		}
	}
}

func (s *SchedulerService) checkJobs() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, job := range s.jobs {
		if now.After(job.TargetTime) {
			// Trigger
			if s.onTrigger != nil {
				go s.onTrigger(job.Prompt)
			}
			
			// Handle repeat or delete
			if job.Repeat {
				job.TargetTime = job.TargetTime.Add(24 * time.Hour)
				if s.ctx != nil {
					runtime.EventsEmit(s.ctx, "scheduler_log", map[string]string{
						"msg": fmt.Sprintf("Repeating job %s scheduled for %s", id, job.TargetTime.Format("15:04:05")),
						"level": "INFO",
					})
				}
			} else {
				delete(s.jobs, id)
			}

			if s.ctx != nil {
				runtime.EventsEmit(s.ctx, "scheduler_updated", nil)
			}
		}
	}
}

func (s *SchedulerService) SchedulePrompt(prompt string, targetTimeStr string, repeat bool) (string, error) {
	targetTime, err := time.Parse(time.RFC3339, targetTimeStr)
	if err != nil {
		return "", fmt.Errorf("invalid time format: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := uuid.New().String()
	s.jobs[id] = &ScheduledJob{
		ID:         id,
		Prompt:     prompt,
		TargetTime: targetTime,
		Repeat:     repeat,
	}

	if s.ctx != nil {
		runtime.EventsEmit(s.ctx, "scheduler_updated", nil)
		runtime.EventsEmit(s.ctx, "scheduler_log", map[string]string{
			"msg": fmt.Sprintf("Job scheduled for %s", targetTime.Format("15:04:05")),
			"level": "SUCCESS",
		})
	}

	return id, nil
}

func (s *SchedulerService) CancelJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
	
	if s.ctx != nil {
		runtime.EventsEmit(s.ctx, "scheduler_updated", nil)
		runtime.EventsEmit(s.ctx, "scheduler_log", map[string]string{
			"msg": fmt.Sprintf("Job %s cancelled", id),
			"level": "WARN",
		})
	}
}

func (s *SchedulerService) GetJobs() []ScheduledJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	var list []ScheduledJob
	for _, j := range s.jobs {
		list = append(list, *j)
	}
	return list
}
