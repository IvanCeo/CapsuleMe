package workerpool

import (
	telegram "capsule-me/internal/delivery/telegram"
	"context"
	"log/slog"
	"sync"
)

type WorkerPool struct {
	jobs    chan telegram.Job
	workers int
	log     *slog.Logger
}

func NewWorkerPool(workers, queueSize int, log *slog.Logger) *WorkerPool {
	return &WorkerPool{
		jobs:    make(chan telegram.Job, queueSize),
		workers: workers,
		log:     log,
	}
}

func (p *WorkerPool) Start(ctx context.Context, handle func(context.Context, telegram.Job)) {
	var wg sync.WaitGroup

	for i := 0; i < p.workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			defer func() {
				if r := recover(); r != nil {
					p.log.Error("worker panic recovered", "workerID", workerID, "panic", r)
				}
			}()

			for {
				select {
				case <-ctx.Done():
					p.log.Info("worker stopping", "workerID", workerID)
					return
				case job, ok := <-p.jobs:
					if !ok {
						p.log.Info("job channel closed", "workerID", workerID)
						return
					}

					handle(ctx, job)
				}
			}
		}(i)
	}

	go func() {
		<-ctx.Done()
		close(p.jobs)
		wg.Wait()
		p.log.Info("worker pool stopped")
	}()
}

func (p *WorkerPool) Submit(job telegram.Job) bool {
	select {
	case p.jobs <- job: // кладет job в канал ворекерпула (очередь)
		return true
	default:
		return false
	}
}
