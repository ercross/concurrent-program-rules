# Signals, Latches, and Queues: The Three Meanings of a Go Channel

These three lines look similar:

```go
work := make(chan struct{})    // unbuffered
work := make(chan struct{}, 1) // buffer = 1
work := make(chan struct{}, 8) // buffer > 1
```

But they describe **three different coordination models**:

* `chan struct{}` → **Rendezvous** → Synchronization point
* `chan struct{}, 1` → **Latch / Signal** → “Wake up, something changed”
* `chan struct{}, N (N > 1)` → **Queue** → Work items / backlog

Confusing this is how “simple” systems stop making progress.

Let’s ground this in a real use case: a background manager that periodically checks for new jobs and schedules workers.


## The Wrong Way: Unbuffered as a Signal

```go
workSignal := make(chan struct{})

func producer() {
    workSignal <- struct{}{} // blocks if manager is busy
}

func manager() {
    for {
        <-workSignal
        checkForWork()
    }
}
```

This looks like “notify the manager”.

But what it really means is:

> “Producer and manager must meet in time.”

If `manager()` is:

* busy checking the DB
* paused by GC
* temporarily stalled

Then `producer()` blocks.

Now your **data plane** (real work) is stalled by your **control plane** (coordination).

### Symptoms in production

* Requests hang “sometimes”
* Goroutines pile up
* CPU is low, but nothing moves
* The system feels “sticky”

You didn’t build a signal. You built a dependency chain.

This violates a core concurrency rule:

> **Never let coordination block real work.**

## The Right Way: Buffered(1) as a Latch

```go
workSignal := make(chan struct{}, 1)

func producer() {
    select {
    case workSignal <- struct{}{}:
    default:
        // already signaled
        // the default case is very important to this model
    }
}

func manager() {
    for {
        <-workSignal
        checkForWork()
    }
}
```

Now the meaning changes:

* If the buffer is empty → store a signal
* If it’s full → do nothing

This expresses:

> “Wake the manager if it’s asleep. Don’t care how many times this happens.”

### Properties

* Producers never block
* Bursts collapse into a single wake-up
* The manager can be late and still observe the event
* No unbounded buildup

This is a **latch**, not a queue.

You’re not sending work. You’re sending awareness that work may exist.



## When Buffer > 1 Becomes a Queue

```go
work := make(chan Job, 8)
```

Now the channel means:

> “Each send represents a unit of work.”

You are modeling **backlog**.

This is correct when:

* Each item must be processed
* Order may matter
* Loss is unacceptable
* Backpressure is part of the design

### Workers

```go
func worker() {
    for job := range work {
        process(job)
    }
}
```

This is a queue.

But if you use this for signals:

```go
workSignal := make(chan struct{}, 8)

workSignal <- struct{}{}
workSignal <- struct{}{}
workSignal <- struct{}{}
```

You’ve created:

* A growing pile of meaningless “wake-ups”
* A manager that wakes up N times to do the same check
* Artificial load that scales with activity

You’ve turned “something changed” into “do the same thing 8 times”.

That’s how schedulers become noisy, wasteful, and eventually unstable.



## The Mental Model

Ask one question:

> **“Do I care about how many times this happened?”**

* **No** → `chan struct{}, 1` (signal / latch)
* **Yes** → `chan T, N` (queue)
* **I need both sides to meet** → `chan T` (rendezvous)

Most coordination channels in real systems are **signals**, not queues.

And the smallest, most expressive way to encode that is:

```go
make(chan struct{}, 1)
```

That single `1` says:

> “Wake me once. I’ll figure the rest out.”

Concurrency isn’t about “making things async.”
It’s about choosing structures that make the **wrong behavior impossible**.

---

If you want, next we can:

* Add a **TL;DR section**
* Tighten this into a **blog-post style README**
* Or rewrite it into a **LinkedIn / Twitter thread** without losing depth
