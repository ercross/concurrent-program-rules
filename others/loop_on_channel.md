# Two Go loops. Same behavior… until production

Most Go engineers have written both of these:

### Loop 1
```go
for {
    <-workSignal
    checkForWork()
}
````

### Loop 2

```go
for range workSignal {
    checkForWork()
}
```

They *look* equivalent, but they are not.


## The hidden assumption in Loop 1

Loop 1 encodes an assumption you probably didn’t mean to make:

> “This goroutine will run forever, and this channel will never be closed.”

That assumption is **structural**, not documented.

If `workSignal` is ever closed (intentionally or by accident)
`<-workSignal` returns immediately. **Forever.**

### Result

* `checkForWork()` runs in a tight loop
* CPU spikes
* The system *melts*
* Debugging is painful because nothing looks obviously wrong

You didn’t build a loop.
You built a shutdown hazard.


## The *safer* version of Loop 1 (explicit receive)

If you *must* use an infinite loop, the minimum safe version looks like this:

```go
for {
    _, ok := <-workSignal
    if !ok {
        return // channel closed, shut down cleanly
    }
    checkForWork()
}
```

This version makes the shutdown behavior **explicit**.

But notice something important:

> This is exactly what `for range` already does for you.


## Why Loop 2 is safer by default

Loop 2 expresses a much safer contract:

```go
for range workSignal {
    checkForWork()
}
```

Which reads as:

> “Handle signals **until the channel is closed**.”

When the channel closes:

* The loop exits
* The goroutine shuts down cleanly
* No spin
* No surprises

This is functionally equivalent to explicitly checking the `ok` value but harder to get wrong.


## Why this matters

Most background managers, schedulers, and coordinators:

* Are long-lived
* But not immortal
* And eventually need a clean shutdown

The first loop **hides that reality**.
The second loop **encodes it directly into the structure of the program**.


## Rule of thumb

* If a goroutine should ever stop → **use `for range`**
* If it must never stop → **document that assumption very loudly**

Concurrency bugs rarely come from wrong syntax.
They come from **structures that encode the wrong guarantees**.



If this resonates, follow my series on **Concurrent Program Rules**.
