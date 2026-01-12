package rule_1

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// job represents a unit of work
type job struct {
	ID        int
	Data      string
	CreatedAt time.Time
	ctx       context.Context
}

// stage1Result represents intermediate processing result
type stage1Result struct {
	JobID     int
	Processed string
	Stage     int
	CreatedAt time.Time
}

// workCoordinator owns the entire flow of work
// It has a clear capacity limit and enforces backpressure
type workCoordinator struct {
	// Fixed-size semaphore controls max concurrent work
	workSemaphore chan struct{}

	// Worker pools with fixed sizes
	stage1Workers int
	stage2Workers int
	stage3Workers int

	// Channels sized to match worker capacity (small buffers)
	stage1Chan chan job
	stage2Chan chan stage1Result
	stage3Chan chan stage1Result

	// Metrics
	mu            sync.Mutex
	activeJobs    int
	rejectedJobs  int64
	completedJobs int64
	totalLatency  time.Duration

	ctx    context.Context
	cancel context.CancelFunc
}

func newWorkCoordinator(maxConcurrent int) *workCoordinator {
	ctx, cancel := context.WithCancel(context.Background())

	return &workCoordinator{
		// Semaphore enforces global concurrency limit
		workSemaphore: make(chan struct{}, maxConcurrent),

		// Worker counts define capacity
		stage1Workers: 5,
		stage2Workers: 3,
		stage3Workers: 2,

		// Small buffers - just enough to smooth flow
		// Not to hide backpressure
		stage1Chan: make(chan job, 5),
		stage2Chan: make(chan stage1Result, 3),
		stage3Chan: make(chan stage1Result, 2),

		ctx:    ctx,
		cancel: cancel,
	}
}

func (wc *workCoordinator) Start() {
	// Start fixed worker pools
	for i := 0; i < wc.stage1Workers; i++ {
		go wc.stage1Worker(i)
	}

	for i := 0; i < wc.stage2Workers; i++ {
		go wc.stage2Worker(i)
	}

	for i := 0; i < wc.stage3Workers; i++ {
		go wc.stage3Worker(i)
	}

	log.Printf("Started coordinator: max_concurrent=%d, workers=[%d,%d,%d]",
		cap(wc.workSemaphore), wc.stage1Workers, wc.stage2Workers, wc.stage3Workers)
}

func (wc *workCoordinator) Stop() {
	wc.cancel()
}

// SubmitJob enforces backpressure at admission
// If system is at capacity, it rejects immediately
func (wc *workCoordinator) SubmitJob(ctx context.Context, data string) error {
	// Try to acquire semaphore - this is the backpressure point
	select {
	case wc.workSemaphore <- struct{}{}:
		// Got permission to proceed
	case <-ctx.Done():
		wc.recordRejection()
		return errors.New("request cancelled")
	case <-time.After(100 * time.Millisecond):
		// Fast rejection instead of accumulating goroutines
		wc.recordRejection()
		return errors.New("system at capacity - try again later")
	}

	job := job{
		ID:        rand.Int(),
		Data:      data,
		CreatedAt: time.Now(),
		ctx:       ctx,
	}

	wc.trackJobStart()

	// Submit to pipeline with timeout
	// If stage1 is backed up, we find out quickly
	select {
	case wc.stage1Chan <- job:
		return nil
	case <-time.After(1 * time.Second):
		// Release semaphore - we couldn't submit
		<-wc.workSemaphore
		wc.trackJobEnd(time.Since(job.CreatedAt), false)
		return errors.New("stage1 backed up - system degraded")
	case <-ctx.Done():
		<-wc.workSemaphore
		wc.trackJobEnd(time.Since(job.CreatedAt), false)
		return errors.New("request cancelled")
	}
}

func (wc *workCoordinator) stage1Worker(id int) {
	for {
		select {
		case <-wc.ctx.Done():
			return
		case job := <-wc.stage1Chan:
			wc.processStage1(job)
		}
	}
}

func (wc *workCoordinator) processStage1(job job) {
	// Simulate processing
	time.Sleep(time.Duration(50+rand.Intn(50)) * time.Millisecond)

	result := stage1Result{
		JobID:     job.ID,
		Processed: fmt.Sprintf("stage1-%s", job.Data),
		Stage:     1,
		CreatedAt: job.CreatedAt,
	}

	// Blocking send - if stage2 is slow, we wait
	// This naturally creates backpressure up the chain
	select {
	case wc.stage2Chan <- result:
		// Sent successfully
	case <-job.ctx.Done():
		// job cancelled, release semaphore
		<-wc.workSemaphore
		wc.trackJobEnd(time.Since(job.CreatedAt), false)
	case <-wc.ctx.Done():
		return
	}
}

func (wc *workCoordinator) stage2Worker(id int) {
	for {
		select {
		case <-wc.ctx.Done():
			return
		case result := <-wc.stage2Chan:
			wc.processStage2(result)
		}
	}
}

func (wc *workCoordinator) processStage2(result stage1Result) {
	// Simulate processing
	time.Sleep(time.Duration(100+rand.Intn(100)) * time.Millisecond)

	result.Processed = fmt.Sprintf("stage2-%s", result.Processed)
	result.Stage = 2

	select {
	case wc.stage3Chan <- result:
		// Sent successfully
	case <-wc.ctx.Done():
		return
	}
}

func (wc *workCoordinator) stage3Worker(id int) {
	for {
		select {
		case <-wc.ctx.Done():
			return
		case result := <-wc.stage3Chan:
			wc.processStage3(result)
		}
	}
}

func (wc *workCoordinator) processStage3(result stage1Result) {
	// Intentionally slow - the bottleneck
	time.Sleep(time.Duration(200+rand.Intn(300)) * time.Millisecond)

	// job complete - release semaphore
	<-wc.workSemaphore

	latency := time.Since(result.CreatedAt)
	wc.trackJobEnd(latency, true)

	if latency > 2*time.Second {
		log.Printf("job %d completed in %v (slow but controlled)", result.JobID, latency)
	}
}

func (wc *workCoordinator) trackJobStart() {
	wc.mu.Lock()
	wc.activeJobs++
	wc.mu.Unlock()
}

func (wc *workCoordinator) trackJobEnd(latency time.Duration, completed bool) {
	wc.mu.Lock()
	wc.activeJobs--
	if completed {
		wc.completedJobs++
		wc.totalLatency += latency
	}
	wc.mu.Unlock()
}

func (wc *workCoordinator) recordRejection() {
	wc.mu.Lock()
	wc.rejectedJobs++
	wc.mu.Unlock()
}

func (wc *workCoordinator) GetMetrics() (active int, completed int64, rejected int64, avgLatency time.Duration) {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	active = wc.activeJobs
	completed = wc.completedJobs
	rejected = wc.rejectedJobs

	if wc.completedJobs > 0 {
		avgLatency = wc.totalLatency / time.Duration(wc.completedJobs)
	}

	return
}

// FixedService uses the coordinator to enforce backpressure
type FixedService struct {
	coordinator *workCoordinator
}

func NewFixedService() *FixedService {
	return &FixedService{
		coordinator: newWorkCoordinator(50), // Max 50 concurrent jobs
	}
}

func (s *FixedService) Start() {
	s.coordinator.Start()
}

func (s *FixedService) HandleRequest(w http.ResponseWriter, r *http.Request) {
	// Submit with timeout context
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := s.coordinator.SubmitJob(ctx, fmt.Sprintf("request-%d", rand.Int()))

	if err != nil {
		// System pushed back - return 503
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "System at capacity: %v", err)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, "job accepted")
}

func (s *FixedService) HealthCheck(w http.ResponseWriter, r *http.Request) {
	active, completed, rejected, avgLatency := s.coordinator.GetMetrics()

	// Health check reflects actual system state
	status := http.StatusOK
	if rejected > completed {
		status = http.StatusServiceUnavailable
	}

	w.WriteHeader(status)
	fmt.Fprintf(w, "active=%d completed=%d rejected=%d avg_latency=%v",
		active, completed, rejected, avgLatency)
}

func RunGoodDesign() {
	service := NewFixedService()
	service.Start()

	http.HandleFunc("/job", service.HandleRequest)
	http.HandleFunc("/health", service.HealthCheck)

	log.Println("Fixed service starting on :8080")
	log.Println("System enforces backpressure - will reject when at capacity")
	log.Println("No unbounded goroutine growth or hidden queue buildup")

	// Simulate load
	go func() {
		time.Sleep(2 * time.Second)
		log.Println("Starting simulated load...")

		var accepted, rejected int

		for i := 0; i < 1000; i++ {
			go func() {
				resp, err := http.Get("http://localhost:8080/job")
				if err != nil {
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode == http.StatusAccepted {
					accepted++
				} else {
					rejected++
				}
			}()

			if i%100 == 0 {
				time.Sleep(100 * time.Millisecond)
				log.Printf("Load test progress: %d requests (accepted: %d, rejected: %d)",
					i, accepted, rejected)
			}
		}
	}()

	log.Fatal(http.ListenAndServe(":8080", nil))
}
