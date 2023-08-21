package peopledb

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2/log"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/leorcvargas/rinha-2023-q3/internal/app/domain/people"
)

var (
	MaxWorker = 1
	MaxQueue  = 1
)

type JobQueue chan Job

// Job represents the job to be run
type Job struct {
	Payload *people.Person
}

// A buffered channel that we can send work requests on.

func NewJobQueue() JobQueue {
	return make(JobQueue, MaxQueue)
}

// Worker represents the worker that executes the job
type Worker struct {
	WorkerPool chan chan Job
	JobChannel chan Job
	quit       chan bool
	db         *pgxpool.Pool
}

func NewWorker(workerPool chan chan Job, db *pgxpool.Pool) Worker {
	return Worker{
		WorkerPool: workerPool,
		JobChannel: make(chan Job),
		quit:       make(chan bool),
		db:         db,
	}
}

// Start method starts the run loop for the worker, listening for a quit channel in
// case we need to stop it
func (w Worker) Start() {
	dataCh := make(chan Job)
	insertCh := make(chan []Job)

	go func() {
		for {
			// register the current worker into the worker queue.
			w.WorkerPool <- w.JobChannel

			select {
			case job := <-w.JobChannel:
				dataCh <- job

			case <-w.quit:
				// we have received a signal to stop
				return
			}
		}
	}()

	go func() {
		batch := make([]Job, 0, 10000)
		tick := time.Tick(5 * time.Second)

		for {
			select {
			case data := <-dataCh:
				batch = append(batch, data)

			case <-tick:
				log.Infof("Insert tick, current batch length is %d", len(batch))
				if len(batch) > 0 {
					insertCh <- batch
					batch = make([]Job, 0, 10000)
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case batch := <-insertCh:
				_, err := w.db.CopyFrom(
					context.Background(),
					pgx.Identifier{"people"},
					[]string{"id", "nickname", "name", "birthdate", "stack", "search"},
					pgx.CopyFromSlice(len(batch), func(i int) ([]interface{}, error) {
						return []interface{}{
							batch[i].Payload.ID,
							batch[i].Payload.Nickname,
							batch[i].Payload.Name,
							batch[i].Payload.Birthdate,
							batch[i].Payload.StackStr(),
							batch[i].Payload.SearchStr(),
						}, nil
					}))

				if err != nil {
					log.Errorf("Error on insert batch: %v", err)
				}
			}
		}
	}()
}

// Stop signals the worker to stop listening for work requests.
func (w Worker) Stop() {
	go func() {
		w.quit <- true
	}()
}

type Dispatcher struct {
	maxWorkers int
	// A pool of workers channels that are registered with the dispatcher
	WorkerPool chan chan Job
	jobQueue   chan Job
	db         *pgxpool.Pool
}

func NewDispatcher(db *pgxpool.Pool, jobQueue JobQueue) *Dispatcher {
	maxWorkers := MaxWorker

	pool := make(chan chan Job, maxWorkers)

	return &Dispatcher{
		WorkerPool: pool,
		maxWorkers: maxWorkers,
		jobQueue:   jobQueue,
		db:         db,
	}
}

func (d *Dispatcher) Run() {
	// starting n number of workers
	for i := 0; i < d.maxWorkers; i++ {
		worker := NewWorker(d.WorkerPool, d.db)
		worker.Start()
	}

	go d.dispatch()
}

func (d *Dispatcher) dispatch() {
	for {
		select {
		case job := <-d.jobQueue:
			// a job request has been received
			go func(job Job) {
				// try to obtain a worker job channel that is available.
				// this will block until a worker is idle
				jobChannel := <-d.WorkerPool

				// dispatch the job to the worker job channel
				jobChannel <- job
			}(job)
		}
	}
}
