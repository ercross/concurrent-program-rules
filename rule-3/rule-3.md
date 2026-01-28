# Buffered Channels Do Not Remove Blocking
### They Only Delay It


This is the **third lesson** in the **Rules for Designing Concurrent Systems** series.

The previous lessons focused on **ownership**, **backpressure**, and **signaling**.  
This one targets a belief that feels almost mathematically true when you first learn concurrency:

> If blocking is bad, then buffers must be good.

Add a buffer.  
Absorb the spike.  
Smooth the system.

And in small tests, that intuition holds.

In production, it is often the beginning of a **slow and expensive failure**.


## The System That “Worked”

The system that triggered this lesson was already “well designed”, or at least that’s what we thought.

It used worker pools.  
It had channels between stages.  
It avoided unbounded goroutine creation.

And when it started to stall under load, the fix seemed obvious.

Producers were blocking.  
Consumers were slower.

The answer, everyone agreed, was to add buffering.

So we did.

A channel that used to block immediately could now absorb hundreds of messages.  
Latency dropped.  
Alerts cleared.

The deployment looked like a success.

For a while.


## When It Started to Fail Again

Then the system started to behave strangely.

- Memory usage crept upward
- Latency became bimodal
- Some requests completed instantly
- Others waited far longer than before

Restarting the service caused a huge burst of work, followed by another slow decay.

The system felt **elastic**, but **unpredictable**.


## The Usual Suspects

When buffers fail, teams usually blame the numbers:

- “The buffer size is still too small.”
- “We need more consumers.”
- “The GC is probably kicking in.”
- “Traffic patterns must have changed.”
- “This worked before. Maybe it’s something else.”

So we increased the buffer again.  
And again.

Each increase bought time.  
Each increase made the failure harder to see.

What we were really doing was **postponing the inevitable**.


## What Buffers Actually Do

A buffer does **not** remove blocking.  
It just moves it somewhere else in time.

When producers are faster than consumers, one of three things must happen:

- Producers block
- Memory grows
- Work is dropped

Buffers choose the second option, **until they cannot**.

In our case, the buffer filled slowly and quietly.  
By the time it was full, the system was already under stress.

When blocking finally occurred, it happened at the worst possible moment:
under peak load, with maximum work already in flight.

The buffer had not solved the mismatch.  
It had **hidden it**.


## Buffers Destroy Backpressure

Before buffering, producers felt pressure immediately.  
After buffering, they ran freely, generating work the system could not sustain.

Downstream stages paid the price later, **all at once**.

At this point, the pattern should feel familiar.

In the first lesson, we established that concurrency is not about “making things async.”  
It is about **controlling backpressure and ownership**.

This failure is the same mistake, wearing a different costume.


## The Real Problem

The real problem was not buffer size.

It was the **absence of flow control**.

We had added capacity without adding ownership.  
No part of the system was responsible for deciding how fast work was allowed to enter.

The buffer quietly accepted everything until it could not.

> Buffers without ownership are just delayed failures.


## Buffers as Batching, Not Shock Absorbers

The turning point came when we stopped treating buffers as shock absorbers  
and started treating them as **deliberate batching mechanisms**.

A shock absorber exists to hide impact.  
It smooths external force so the rest of the system does not feel it.

That is exactly how buffers are misused in broken systems: to hide overload.

Batching is different. It is intentional.

It says:

- We will process work in groups of this size
- We understand the cost of each batch
- We know how long a batch should take
- We know what happens when a batch cannot be accepted

When buffers represent batching, their size is meaningful.  
It corresponds to a unit of work the system can reason about, observe, and control.

When buffers are shock absorbers, their size is arbitrary.

Bigger just means **“fail later.”**


## How to Think About Buffer Size

There is no universally correct buffer size.  
But there is a correct way to choose one.

Start by answering:

- What is the downstream throughput in steady state?
- What is the maximum acceptable latency for work in this stage?
- How much work can we afford to have in flight at once?
- What happens when the buffer fills?

A useful mental model:

> **Buffer size should represent time, not just count.**

If a consumer can process 100 items per second  
and you are comfortable with at most 200ms of queued delay,

your buffer should be on the order of **20 items**, not 2,000.

Anything larger is not smoothing variance.  
It is hiding overload.

If you cannot explain what a full buffer means operationally, the buffer is too large.

And if a full buffer does not trigger a visible response —  
backpressure, shedding, or alerting — the buffer is dangerous.


## When the Buffer *Is* the Queue

In some systems, the buffer is not a shock absorber or a batch.

It *is* the queue.

Examples:

- A job channel with workers pulling tasks
- An in-memory task queue
- A bounded request backlog

Here, buffer sizing is capacity planning.

Buffer size represents **maximum queued work**, not “room for spikes.”

It should still represent time:

> buffer size ≈ throughput × acceptable waiting time


The difference is what happens when it fills.

A queue-buffer must have a defined overflow behavior:

- Block producers
- Shed load
- Persist to disk
- Reject requests

If the answer is “it keeps growing” or “we’ll tune it later,”  
the system is already broken.

Queues must be **lossless** and **explicit about overflow**.


## The Invariant That Applies Everywhere

No matter how you use a buffer, one thing is always true:

> A buffer is a commitment to hold work in memory instead of applying pressure.

The moment you add buffering, you are saying:

- “I am okay with this much work being in flight”
- “I am okay with this much delay”
- “I am okay with this much memory usage”

If you cannot articulate all three in concrete terms, the buffer is unsafe.


## The Fix

We didn’t tune the buffer.  
We made it honest.

We redesigned the system so that:

- Buffers were small and intentional
- Fill rate was constrained by downstream capacity
- Drain rate was observable and monitored
- A full buffer was treated as a system event, not an inconvenience

In some places, we removed buffers entirely.  
In others, we kept them small and made them visible in metrics and alerts.

Most importantly, we allowed producers to block again —  
but only where blocking was acceptable and meaningful.

Once pressure was allowed to propagate upstream:

- Latency became predictable
- Memory stopped creeping
- Load stopped accumulating invisibly

The system stabilized.


## The Lesson

**Buffered channels do not remove blocking.  
They only delay it.**

If you don’t decide where blocking is allowed to happen,  
your system will decide for you.

And it will choose the **worst possible time**.
