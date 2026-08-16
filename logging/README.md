# Logging — A Tutorial

**System 2 of 9.** Build this before anything harder, because everything harder is easier to debug with it.

---

## The problem, told as a story

Something is wrong in production. A user reports that their upload failed.

You open the logs. You find a line: `error`. Above it: `starting upload`. Above that, four hundred lines from other users' requests that happened to interleave with theirs, because your server handles many requests at once and they all write to the same log. Below it, more of the same.

You can't tell which `starting upload` belongs to which `error`. You can't tell what the user's ID was. You can't tell how long anything took. So you do what everyone does: you add print statements around the suspicious area, deploy, and wait for it to happen again.

**That "add print statements after the fact" loop is the thing this system eliminates.** The goal is that when something goes wrong, the information you need is *already there*, without a redeploy.

---

## The mental model

Two ideas do almost all the work.

**Idea one: log records, not sentences.**

Most people write logs as prose: "user 4821 uploaded avatar.png in 320ms". That reads nicely and is nearly useless at scale, because to find all slow uploads you have to write a regular expression against English.

Instead, log a *record*: an event name plus a set of labelled fields. The message is "upload complete", and alongside it, separately, `user_id=4821`, `filename=avatar.png`, `duration_ms=320`. Now "show me every upload over 1000ms" is a query, not an archaeology project. This is what **structured logging** means.

**Idea two: every line from one request carries the same ID.**

When a request arrives, generate a random identifier for it. Attach that ID to every log line produced anywhere while handling that request — including deep inside functions that have no idea they're in a web request.

Now the four-hundred-line interleaving problem disappears. Filter by that one ID and you get exactly that user's request, in order, and nothing else. This is called a **correlation ID** (or request ID, or trace ID — same thing).

Those two ideas together are 90% of the value. Everything else in this system is polish.

---

## What this system does

- Emits structured output: machine-readable in production, easy to read in development.
- Attaches a correlation ID to every line produced during one request or one background job.
- Supports levels (debug / info / warn / error) with a single threshold controlled by configuration.
- Catches errors that escape your handlers and sends them to one reporting destination.
- Provides a small helper for timing an operation and logging how long it took.

## What this system does *not* do

**The logging library itself.** Go's standard library already ships a structured logger that handles levels, key/value fields, and JSON output. You are not writing that. What you're writing is a thin layer on top: a component that formats output nicely in development, plus the plumbing that carries a correlation ID around. **This system is configuration and conventions, not a logger.**

This is worth internalizing, because it's the general shape of a good module: find the part that's genuinely yours (your conventions, your context propagation, your redaction rules) and build only that.

**Metrics and dashboards.** Counters, gauges, percentile latencies, graphs — that's a different problem with a different data model, and logging systems that grow into metrics systems do both badly. If you want to know "what is the p99 latency of the upload endpoint", that's a metrics question. Resist expanding this system into it.

**The audit trail.** This is the important distinction, and it's easy to blur:

> **Logs are for debugging and are allowed to be lossy. Audit records are neither.**

If your log sink is down for thirty seconds and you drop some lines, that's acceptable — annoying, not a crisis. If your *audit* system drops a record of who deleted a customer's account, that's a compliance failure and possibly a legal one. Because the tolerances are completely different, the implementations are completely different. Audit records live in `AuditLog/`, written transactionally to a database. Don't merge them.

---

## How to design it

### Structured, but readable when a human is watching

In production, logs are consumed by machines: shipped to an aggregator, indexed, queried. JSON is the right format — one JSON object per line, easy to parse, no ambiguity.

In development, logs are consumed by *you*, in a terminal, while you're trying to concentrate. JSON is actively hostile there. A wall of `{"time":"2025-...","level":"INFO","msg":"upload complete","user_id":4821}` is much harder to scan than a colourized line with the message first and fields aligned after it.

**Pick the format from configuration: JSON in production, pretty in development.** Same log calls, same fields, different rendering. This one feature is, honestly, the main reason to write this module at all rather than just calling the standard logger directly.

Go's structured logger is built for this: it separates *what gets logged* from *how it's formatted* by putting the formatting behind an interface called a handler. You write your own handler for development. The rest of your code never knows which one is installed.

**A note on Go interfaces, if you're new to the language:** an interface is just a list of method signatures. Unlike in Java or C#, a type does not declare that it implements an interface — if it has the right methods, it satisfies the interface automatically. So writing a custom log handler means writing something with the handful of methods the logger expects, and passing it in. Nothing needs to know your type exists ahead of time. You'll see this pattern constantly in Go, and it's a large part of why Go modules can stay so decoupled.

### How the correlation ID travels — the interesting design decision

You have an ID at the top of a request. You need it available at the bottom of a call chain, in a function five levels deep that knows nothing about HTTP. How does it get there?

**Option A: pass it explicitly.** Every function that might log takes an extra parameter. Ugly. It pollutes signatures throughout your codebase. Also: always correct. There is no ambiguity about where the value came from, no way for it to be missing, no magic.

**Option B: ambient storage.** Stash the ID somewhere the logger can find it implicitly, so no function signature changes. Beautiful at the call site. In Go, this is dangerous, because Go's concurrency model means any function might be running on a different goroutine (Go's lightweight threads) than the one that set the value, and the value simply won't be there. It works in tests and fails on the one code path that spawns a background operation.

**Go's answer is a middle path, and it's the idiomatic one: `context.Context`.**

Context is a value that gets passed as the first argument to functions, by convention, throughout Go code. You can attach values to it, producing a new context that carries them. So you put the correlation ID into the context at the start of the request, and every function down the chain already takes a context (because that's the convention for cancellation and deadlines anyway), so the ID rides along for free.

It's technically explicit — the parameter is right there in the signature — but you're not adding a new parameter, you're using one that should be there regardless. That's why the whole Go ecosystem settled on it.

**The convention to follow:** functions that do work take a context as their first parameter. Your logging helper takes a context and pulls the correlation ID out of it. Never store a context in a struct field — it's meant to flow through calls, not be held.

### Where the ID gets created: middleware

An HTTP middleware is a piece of code that wraps a handler: it runs before, calls the handler, and runs after. It's the natural home for per-request setup.

Your logging middleware:
1. Checks whether the incoming request already has an ID header (from a load balancer or another service). Use it if it's there — that's how you follow one user action across multiple services.
2. Generates a fresh random one if not.
3. Puts it into the request's context.
4. Echoes it back in a response header, so a user reporting a problem can tell you the exact ID.
5. Calls the actual handler.
6. When the handler is done, logs one line: method, path, status code, duration.

That last step gives you an access log for free, in the same structured format as everything else, correlated with everything else.

**Background jobs need the same thing.** A job isn't an HTTP request, but it's still a unit of work whose lines you want grouped. Give it an ID when it starts. Better still: when the job was enqueued during a request, pass the request's ID along in the job payload, so you can trace from "user clicked the button" all the way through "the job ran forty seconds later and failed."

### Levels, and the one setting that controls them

Four levels is plenty: debug, info, warn, error.

The useful mental test for choosing one:

- **debug** — you'd want this while actively investigating; noisy by design.
- **info** — a thing happened that you'd want to see in a normal day's logs.
- **warn** — something is wrong but the system handled it. A retry succeeded. A deprecated endpoint was called.
- **error** — something failed and a user or a job was affected.

One threshold, from configuration. Production runs at info; development runs at debug. Don't build per-module level configuration until you've genuinely been drowned by one specific module's output — it's a configuration surface that's almost never used.

### Catching what escapes

If a handler panics — Go's term for an unrecoverable runtime error, like dereferencing a nil value — the default behaviour is to crash the whole process. In a web server, that's the wrong response to one bad request.

Go's `recover` mechanism lets you catch a panic and keep running. So: a middleware that recovers, logs the failure at error level *with the stack trace and the correlation ID*, returns a 500 to the client, and lets the server continue serving everyone else.

Send those to one reporting destination as well as the log — a file, or an error-tracking service. The shape is the same either way: an interface with one method that takes an error and some context, and one implementation per destination. Which one is installed comes from configuration, so tests can install a no-op.

**A caveat worth knowing:** a recovered panic leaves that request's work in an unknown half-finished state. Recovering is right for keeping the server up, but it is not a substitute for handling errors properly. If you find yourself relying on the recover middleware regularly, that's a signal, not a solution.

### The timing helper

Small, and you'll use it constantly. Something that you start at the top of an operation and that logs the elapsed duration when the operation finishes, whether it succeeded or not.

In Go, `defer` schedules a function call to run when the enclosing function returns — no matter how it returns, including via panic. That makes it the natural fit: start the timer, defer the log, and the duration gets recorded on every exit path without you thinking about it.

Log durations as a number in a labelled field, not as text inside the message. `duration_ms=320`, not "took 320ms". Numbers in fields can be sorted and filtered; numbers in sentences cannot.

---

## Decisions to make first

**JSON everywhere, or JSON in production and pretty in development?** The latter, and as noted, it's the main justification for this module existing.

**How does the correlation ID travel?** Through `context.Context`, per the reasoning above. Explicit is uglier and always correct; context is the version of explicit that Go's ecosystem is already built around.

**Do you sample high-volume lines, or log everything and pay for it?** Log everything until the bill or the noise actually hurts. Sampling is a real technique but it introduces a genuinely nasty property: the line you need might be one of the ones that got dropped. Add it when volume forces you to, and when you do, never sample errors.

---

## Danger zones

**Never log credentials, tokens, full request bodies, or personal data.** Have exactly one redaction list, and route every log call through it. The reason this needs to be enforced in one place rather than by everyone being careful: "log the whole request body so I can debug this" is an extremely reasonable-sounding thing for a tired person to do at 2am, and request bodies contain passwords.

Practical form: a list of field names that are always redacted, and — for structured objects — an allowlist of fields you're permitted to log rather than a blocklist of fields you're not. Blocklists fail silently when someone adds a new field.

**Logging must never take down a request.** If the log destination is unreachable, if the disk is full, if the JSON encoder chokes on a weird value — the logger drops the line and moves on. It does not return an error that propagates. It does not panic. It does not block waiting for a network write.

This one is easy to get wrong by accident. A synchronous write to a remote log service, inside a request, with no timeout, is a perfectly normal-looking piece of code that will hang every request in your application when that service has a bad day. If a sink can be slow, it gets a buffer and a background writer, and the buffer drops lines when it's full rather than blocking.

---

## You'll know it works when

Send one request that does several things internally. Every line it produced shares one ID, and you can filter to exactly that request.

Cause an unhandled failure inside that request. It lands in the error destination with a stack trace and the same correlation ID, the client gets a 500, and the server is still serving other requests.

Then check the output for anything from `Config` that shouldn't be there. No secret appears anywhere.

---

**When you're ready to write it:** [`BUILDING.md`](BUILDING.md) — file layout, the `slog` mechanics, and the traps.

**Next:** [`Users/`](../Users/README.md) — the big one. Most of the learning in this repo lives there.
