# Logging — How To Build It

The [README](README.md) is the *what* and the *why*. This is the *how*: what to write, in what order, and the Go mechanics that make it work.

Fragments only. The gaps are the exercise.

---

## Before you start: four bits of Go

**An interface is a list of method signatures, and nothing declares that it implements one.** If your type has the methods, it satisfies the interface. This is how you'll swap the development formatter for the production one without a single line of your application knowing which is installed.

**`context.Context` is the first parameter, by convention.** `func DoThing(ctx context.Context, id int) error`. It carries cancellation, deadlines, and — the part you need here — values. You never mutate a context; `context.WithValue` returns a *new* one wrapping the old.

**`defer` runs a call when the enclosing function returns**, by any path including a panic. It's how you time things without repeating yourself on every `return`.

**Embedding is Go's composition.** Putting a type inside a struct with no field name promotes its methods:

```go
type wrapper struct {
    slog.Handler   // no field name — wrapper now has all of Handler's methods
}
```

That's a shortcut with a sharp edge, and it appears in Step 3 below.

---

## What you're actually building

Say it out loud before writing anything: **you are not writing a logger.** Go's `log/slog` already does levels, key/value fields, and JSON. You're writing three small things around it:

1. A **development handler** that formats records for a human.
2. A **context handler** that stamps the correlation ID onto every record automatically.
3. **Middleware** that creates the ID, times the request, and survives panics.

Everything else is a constructor that wires those together from config.

---

## Step 1 — Files, and the constructor

```
Logging/
  logging.go     New(Config) *slog.Logger — the wiring
  devhandler.go  the pretty handler
  ctxhandler.go  the correlation-ID handler
  middleware.go  RequestID, Recover, access logging
  logging_test.go
  cmd/demo/main.go
```

Per rule 3, config comes in as a plain struct with exported fields — this package never reads the environment:

```go
type Config struct {
    Level  string // "debug" | "info" | "warn" | "error"
    Format string // "json" | "pretty"
}

func New(cfg Config, w io.Writer) *slog.Logger
```

Take an `io.Writer` rather than hardcoding `os.Stdout`. It costs one parameter and it's what makes the whole thing testable — tests pass a `bytes.Buffer` and read back exactly what was written. Do this now, not later.

`New` picks a base handler from `cfg.Format`, wraps it in the context handler, returns `slog.New(...)`. Six lines.

---

## Step 2 — Levels come from a string

```go
var lv slog.Level
if err := lv.UnmarshalText([]byte(cfg.Level)); err != nil {
    // decide: default to info, or return an error
}
```

`slog.Level` already knows how to parse `"debug"`, `"info"`, `"warn"`, `"error"` — you don't need a switch or a map. Then set it once in `&slog.HandlerOptions{Level: lv}`.

You will see `slog.LevelVar` mentioned in the docs, which allows changing the level while running. Don't. Config is frozen (see [`Config/README.md`](../Config/README.md)) — a level that changes at runtime is a feature flag, and that's `Flags/`.

---

## Step 3 — The context handler, and the embedding trap

This is the cleverest twenty lines in the system, so read it twice.

`slog.Handler` has four methods:

```go
Enabled(context.Context, slog.Level) bool
Handle(context.Context, slog.Record) error
WithAttrs([]slog.Attr) slog.Handler
WithGroup(string) slog.Handler
```

Note that `Handle` receives the **context**. That's the whole trick: the handler can reach into the context and pull out the correlation ID itself. Your application code calls `slog.InfoContext(ctx, "upload complete", "bytes", n)` and never mentions the ID at all.

```go
type ctxHandler struct{ slog.Handler }

func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
    if id, ok := ctx.Value(ridKey{}).(string); ok {
        r.AddAttrs(slog.String("request_id", id))
    }
    return h.Handler.Handle(ctx, r)
}
```

Embedding gives you the other three methods for free, and **two of those freebies are wrong**. `WithAttrs` and `WithGroup` return a *new handler* — and the embedded implementation returns the inner handler, not your wrapper. So the moment anyone calls `logger.With("component", "storage")`, your wrapper is silently discarded and correlation IDs stop appearing. Nothing errors. The IDs are just gone on some lines and not others, which is a miserable thing to debug.

Override both, re-wrapping:

```go
func (h ctxHandler) WithAttrs(as []slog.Attr) slog.Handler {
    return ctxHandler{h.Handler.WithAttrs(as)}
}
```

Same shape for `WithGroup`. Write a test that calls `.With(...)` and asserts the ID still shows up — that's the test that would have caught it.

**The context key must be an unexported type, not a string:**

```go
type ridKey struct{}   // unexported, zero-size, collision-proof
ctx = context.WithValue(ctx, ridKey{}, id)
```

If you use `context.WithValue(ctx, "request_id", id)`, any other package using the same string key overwrites yours. The key space is global and untyped; an unexported struct type is unforgeable from outside your package. This is the standard Go idiom and `go vet` will complain if you use a bare string.

Expose one small accessor so other code can read the ID without knowing your key type:

```go
func RequestIDFrom(ctx context.Context) string
```

---

## Step 4 — The middleware

Middleware is a function that takes a handler and returns a handler:

```go
func RequestID(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // before
        next.ServeHTTP(w, r.WithContext(ctx))
        // after
    })
}
```

`http.HandlerFunc` is an adapter that turns a function into an `http.Handler` — it's a named function type with a `ServeHTTP` method on it. Worth reading its three-line source once; it's the clearest example of Go interfaces in the standard library.

For the ID itself: `rand.Text()` from `crypto/rand` gives you a random string in one call. Don't reach for a UUID package — the ID needs to be unique and unguessable, not to conform to a spec.

Order of work in the middleware: reuse an incoming `X-Request-ID` header if present (that's how a trace crosses services), otherwise generate; put it in the context; echo it in the response header **before** calling the next handler — headers can't be set after the body starts.

**Capturing the status code needs a wrapper**, because `http.ResponseWriter` is write-only:

```go
type statusRecorder struct {
    http.ResponseWriter
    status int
}

func (s *statusRecorder) WriteHeader(code int) {
    s.status = code
    s.ResponseWriter.WriteHeader(code)
}
```

Two traps. First, a handler that just writes a body never calls `WriteHeader` at all, so initialize `status` to `200` rather than leaving the zero value — otherwise your access log claims every successful request returned `0`. Second, embedding a `ResponseWriter` hides any extra interfaces the real one had (`http.Flusher`, `http.Hijacker`), which breaks streaming and websockets. The modern fix is `http.ResponseController`, which finds the underlying capability through wrappers. Note it; you probably don't need it yet.

---

## Step 5 — The timing helper, and the `defer` trap

```go
start := time.Now()
defer slog.InfoContext(ctx, "done", "duration_ms", time.Since(start).Milliseconds())
```

**That is wrong**, and it's the single most common Go beginner bug. `defer` evaluates the *arguments* immediately and only delays the *call* — so `time.Since(start)` is measured at the top of the function and always logs approximately zero.

`go vet` flags this exact shape (`call to time.Since is not deferred`), which is a rare kindness — most of the traps in these documents have no such warning. Don't rely on it to catch every variation, though: hide the duration behind a helper and the analyzer has nothing to match on.

Defer a closure instead:

```go
defer func() {
    slog.InfoContext(ctx, "done", "duration_ms", time.Since(start).Milliseconds())
}()
```

Now `time.Since` runs at exit. Log the number in a field, never inside the message string.

---

## Step 6 — Recover

```go
defer func() {
    if v := recover(); v != nil {
        slog.ErrorContext(ctx, "panic",
            "value", v,
            "stack", string(debug.Stack()))
        w.WriteHeader(http.StatusInternalServerError)
    }
}()
```

Three rules the compiler won't enforce:

- **`recover()` only works when called directly inside a deferred function.** Call it from a helper the deferred function calls, and it returns `nil` and the process dies anyway.
- **Capture the stack inside the recover**, with `runtime/debug.Stack()`. By the time you've returned from the deferred function, the stack is gone.
- **A panic in a goroutine you spawned cannot be recovered here.** `go doWork()` that panics takes down the entire process, no matter how many recover middlewares wrap the request. Every goroutine needs its own. This will matter enormously in `Jobs/`.

Order matters when you install these: recover must be *outside* the request-ID middleware so the ID is already in the context when it logs. Middleware nests like parentheses — the first one you wrap with is the outermost.

---

## Step 7 — The development handler

The only piece with real code in it, and it's a satisfying afternoon. Implement the four methods; `Handle` does the work:

```go
func (h *devHandler) Handle(_ context.Context, r slog.Record) error {
    // r.Time, r.Level, r.Message
    r.Attrs(func(a slog.Attr) bool {
        // append " key=value"
        return true   // false stops iteration early
    })
    // one Write to h.w
}
```

`Record.Attrs` takes a callback rather than returning a slice — that's slog avoiding an allocation on every log line. Returning `false` stops early.

Points that decide whether this is nice to use:

- **Message first, fields after.** You scan for the message; the fields are detail.
- **Colour by level**, with raw ANSI escapes (`\033[31m`). Skip a colour library. Check whether the output is a terminal before colourizing, or your log files fill with escape codes.
- **One `Write` call per record.** Two goroutines logging at once interleave *per write*, so building the line in a `strings.Builder` and writing once is what stops output from being shredded under concurrency. If you're being careful, guard the write with a `sync.Mutex` — that's what the standard handlers do.
- `WithAttrs` must return a handler holding the accumulated attributes; it must not modify the receiver. Handlers get shared across goroutines.

---

## Step 8 — Redaction

The README calls for one list, applied in one place. The place is your handler's `Handle` — it's the funnel every record passes through, which is precisely why the funnel is worth having.

```go
var redactKeys = map[string]bool{"password": true, "token": true, "authorization": true, ...}
```

Match case-insensitively, and match substrings (`api_key` should trip on `key`). Replace the value with `"[redacted]"` rather than dropping the field, so you can see the call happened.

Understand the ceiling here: this catches `slog.String("password", p)`. It does **not** catch `slog.Any("user", u)` where the struct has a `Password` field, and it does not catch a secret pasted into the message text. Structs get an explicit `LogValue() slog.Value` method (implement `slog.LogValuer`) that names the safe fields — an allowlist, per the README, not a blocklist.

---

## Step 9 — Tests

The `io.Writer` from Step 1 pays off:

```go
var buf bytes.Buffer
log := New(Config{Level: "info", Format: "json"}, &buf)
log.InfoContext(ctxWithID("abc123"), "hello")

var got map[string]any
json.Unmarshal(buf.Bytes(), &got)   // one JSON object per line
if got["request_id"] != "abc123" { t.Fatalf(...) }
```

Three tests are enough:

1. The ID appears in output when it's in the context.
2. The ID still appears **after `.With(...)`** — the Step 3 trap.
3. A `debug` line is absent at `info` level.

Testing the pretty handler's exact spacing is a waste; it's for eyes, and you'll change it.

---

## The mistake everyone makes at the end

You'll want other systems to log. So `Storage` grows a `logger *slog.Logger` field, and to construct one it imports `logging`. Rule 1, broken.

Two ways out, and you want the first:

**Take `*slog.Logger` directly.** It's a standard-library type, so depending on it isn't a cross-import at all — `log/slog` is as neutral as `io.Writer`.

```go
// in Storage/storage.go
func New(cfg Config, log *slog.Logger) *Storage
```

The app passes the configured logger in. `Storage` gets structured logging, correlation IDs ride along in the context for free, and it has never heard of the `Logging` package.

**Or take nothing** and use `slog.Default()`. Less testable, one less parameter. Fine for a system that logs twice.

What you must not do is define a `Logger` interface in each system "for decoupling". `*slog.Logger` is already the decoupled thing. An interface with one implementation is the abstraction the root README warns about.

---

## Order of work

1. `New` + JSON handler + level parsing. `go build ./...` passes, one log line appears.
2. Context handler and `RequestIDFrom`. Test 1.
3. `RequestID` middleware with the status recorder. Watch two concurrent requests get different IDs.
4. Recover middleware. Panic on purpose; confirm the server keeps serving.
5. The pretty handler.
6. Redaction and the remaining tests.

Steps 1–4 are the system. Step 5 is the part you'll enjoy and the part you'll rewrite twice.

---

**Back to:** [`Logging/README.md`](README.md) — the design and the danger zones.
