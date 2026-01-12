package rule_1

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// brokenService demonstrates the problematic pattern
type brokenService struct {
	stage1Chan chan job          // Unbounded queue (buffered, but no real limit)
	stage2Chan chan stage1Result // Another unbounded queue
	stage3Chan chan stage1Result // Yet another unbounded queue

	// Workers keep spawning, no capacity control
	activeGoroutines int
	mu               sync.Mutex
}

func newBrokenService() *brokenService {
	return &brokenService{
		// Large buffers that hide the problem temporarily
		stage1Chan: make(chan job, 10000),
		stage2Chan: make(chan stage1Result, 10000),
		stage3Chan: make(chan stage1Result, 10000),
	}
}

func (s *brokenService) Start() {
	// Stage 1: Spawn workers that read from stage1Chan
	// Problem: Fixed number of workers, but unbounded input
	for i := 0; i < 5; i++ {
		go s.stage1Worker(i)
	}

	// Stage 2: Process results from stage 1
	for i := 0; i < 3; i++ {
		go s.stage2Worker(i)
	}

	// Stage 3: Final processing (slow)
	// This is the bottleneck that causes backpressure
	for i := 0; i < 2; i++ {
		go s.stage3Worker(i)
	}
}

// HandleRequest spawns a goroutine for EVERY request
// Problem: No limit on concurrent requests being processed
func (s *brokenService) HandleRequest(w http.ResponseWriter, r *http.Request) {
	jobID := rand.Int()
	job := job{
		ID:        jobID,
		Data:      fmt.Sprintf("request-%d", jobID),
		CreatedAt: time.Now(),
	}

	// Fire and forget - no backpressure!
	// Each request spawns a new goroutine
	go func() {
		s.trackGoroutine(1)
		defer s.trackGoroutine(-1)

		// Try to send to stage1, but if channel is full, this blocks
		// Meanwhile, more goroutines keep spawning...
		select {
		case s.stage1Chan <- job:
			// Success
		case <-time.After(30 * time.Second):
			// Timeout, but the goroutine already exists
			log.Printf("job %d timed out waiting for stage1", job.ID)
		}
	}()

	// Respond immediately, even though work isn't done
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, "job %d accepted (goroutines: %d)", jobID, s.getGoroutineCount())
}

func (s *brokenService) stage1Worker(id int) {
	for job := range s.stage1Chan {
		// Simulate some processing
		time.Sleep(time.Duration(50+rand.Intn(50)) * time.Millisecond)

		result := stage1Result{
			JobID:     job.ID,
			Processed: fmt.Sprintf("stage1-%s", job.Data),
			Stage:     1,
			CreatedAt: time.Now(),
		}

		// Send to next stage - no backpressure check
		// If stage2 is slow, this blocks, backing up stage1
		s.stage2Chan <- result
	}
}

func (s *brokenService) stage2Worker(id int) {
	for result := range s.stage2Chan {
		// Simulate processing
		time.Sleep(time.Duration(100+rand.Intn(100)) * time.Millisecond)

		result.Processed = fmt.Sprintf("stage2-%s", result.Processed)
		result.Stage = 2

		// Send to final stage
		s.stage3Chan <- result
	}
}

func (s *brokenService) stage3Worker(id int) {
	for result := range s.stage3Chan {
		// This is simulates a longer running process here, which eventually causes the bottleneck
		time.Sleep(time.Duration(200+rand.Intn(300)) * time.Millisecond)

		// Work is "done" but nobody knows how long it took
		age := time.Since(result.CreatedAt)
		if age > 5*time.Second {
			log.Printf("WARNING: job %d took %v (stage3 worker %d)",
				result.JobID, age, id)
		}
	}
}

func (s *brokenService) trackGoroutine(delta int) {
	s.mu.Lock()
	s.activeGoroutines += delta
	count := s.activeGoroutines
	s.mu.Unlock()

	if count > 100 && count%50 == 0 {
		log.Printf("WARNING: %d active goroutines spawned from requests!", count)
	}
}

func (s *brokenService) getGoroutineCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeGoroutines
}

func (s *brokenService) HealthCheck(w http.ResponseWriter, r *http.Request) {
	// Health check passes even when system is drowning
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK (goroutines: %d, stage1 queue: %d, stage2 queue: %d, stage3 queue: %d)",
		s.getGoroutineCount(),
		len(s.stage1Chan),
		len(s.stage2Chan),
		len(s.stage3Chan))
}

func RunBadDesign() {
	service := newBrokenService()
	service.Start()

	http.HandleFunc("/job", service.HandleRequest)
	http.HandleFunc("/health", service.HealthCheck)

	log.Println("Broken service starting on :8080")
	log.Println("Watch as goroutines accumulate and queues fill up...")
	log.Println("Health checks will show 'OK' even as the system drowns")

	// Simulate load
	go func() {
		time.Sleep(2 * time.Second)
		log.Println("Starting simulated load...")

		for i := 0; i < 1000; i++ {
			go func() {
				resp, err := http.Get("http://localhost:8080/job")
				if err != nil {
					return
				}
				resp.Body.Close()
			}()

			if i%100 == 0 {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	log.Fatal(http.ListenAndServe(":8080", nil))
}
