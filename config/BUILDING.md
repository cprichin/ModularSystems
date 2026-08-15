# Config — How To Build It

The [README](README.md) is the *what* and the *why*. This is the *how*: the order to write things in, the Go you need at each step, and the traps that cost a beginner an afternoon.

There is no finished implementation here. Every snippet is a fragment — a signature, a shape, three lines to show a mechanism. The gaps are the exercise.

---

## Before you start: five bits of Go

You need these five and nothing else. Each shows up below.

**A package is a folder.** Every `.go` file in `Config/` starts with `package config`. Files in the same package see each other's names without imports. That's the whole module system at this scale.

**Capitalization is the access modifier.** `Port` is visible to other packages. `port` is not. There is no `private` keyword — the letter case *is* the keyword. This one fact is what makes the immutability in this system work.

**A method is a function with a receiver.**

```go
func (c Config) Port() int { return c.port }
//   ^^^^^^^^^^ the receiver: this function belongs to Config
```

`(c Config)` is a **value receiver** — `c` is a copy. `(c *Config)` is a **pointer receiver** — `c` is the original. For accessors you want value receivers, always.

**Errors are values you return, not things you throw.** A function that can fail returns two things:

```go
v, err := strconv.Atoi(s)
if err != nil {
    // handle it
}
```

There is no `try`. There is no stack unwinding. You check `err` every single time, and the compiler makes you use the values you declare.

**The zero value.** A declared-but-unset `int` is `0`, a `string` is `""`, a `bool` is `false`, a pointer is `nil`. Nothing is undefined. This matters here because a failed setting will leave a zero behind, and zero is a *plausible-looking* value — which is exactly why you must collect the error rather than trust the result.

---

## Step 1 — Lay out the files

```
Config/
  config.go     the Config type and its accessors
  load.go       Load(), and the small typed readers
  dotenv.go     the dev-only .env reader
  config_test.go
  cmd/demo/main.go   the standalone demo (rule 2)
```

Four files, one package (`cmd/demo` is its own `package main`). Don't build a folder tree; there isn't enough here to justify one.

Start with `config.go`, because the shape of the type decides everything else.

---

## Step 2 — The type, and the immutability trick

Grouped, per the README. Lowercase fields, exported accessors:

```go
type Config struct {
    server ServerConfig
    db     DBConfig
    smtp   SMTPConfig
}

type ServerConfig struct {
    port int
    host string
}

func (c Config) Server() ServerConfig { return c.server }
func (s ServerConfig) Port() int      { return s.port }
```

Read what that buys you. Outside the package, `cfg.Server()` hands back a **copy** of a struct whose fields are invisible. There is no field to assign to, no pointer to follow. `cfg.Server().port = 9999` doesn't fail at runtime — it fails to compile, with `port undefined (cannot refer to unexported field)`. That compile error is the system working.

Two traps:

- **Never return `*Config` or `*ServerConfig`.** A pointer hands over the original, and the whole thing collapses. Value receivers, value returns, no exceptions.
- **Slices and maps escape anyway.** If you add `origins []string`, then `func (c CORSConfig) Origins() []string { return c.origins }` returns a *header pointing at the same array* — the caller can write `origins[0] = "evil"` and change yours. Either copy on the way out (`slices.Clone`) or don't expose the collection at all: `func (c CORSConfig) AllowsOrigin(s string) bool` is smaller and leaks nothing. Prefer the second.

Write the accessors by hand. There will be a dozen. It's boring, it's thirty seconds each, and it's the whole security boundary.

---

## Step 3 — Collect errors instead of returning the first one

The README insists you report *every* problem at once. The lazy way to do that is a tiny struct that carries an error list, with the typed readers hanging off it as methods:

```go
type loader struct {
    errs []error
}

func (l *loader) requireString(key string) string {
    v, ok := os.LookupEnv(key)
    if !ok || v == "" {
        l.fail("%s: required, but not set", key)
        return ""
    }
    return v
}

func (l *loader) fail(format string, args ...any) {
    l.errs = append(l.errs, fmt.Errorf(format, args...))
}
```

Note the **pointer receiver** here — `*loader`. Appending to `l.errs` has to mutate the original; a value receiver would append to a copy and silently throw it away. That's the mirror image of Step 2, and the reason to understand the difference rather than memorize a rule.

`os.LookupEnv` returns `(value, wasItSet)`. `os.Getenv` returns only the value, so it can't distinguish "unset" from "set to empty" — use `LookupEnv`. (Whether you treat an explicitly-empty value as unset is your call; treating it as missing is usually kinder.)

Now write the rest of the readers, all the same shape:

| Reader | Parses with | Also check |
| --- | --- | --- |
| `requireString(key)` | — | non-empty |
| `optString(key, def)` | — | — |
| `intInRange(key, def, lo, hi)` | `strconv.Atoi` | within bounds |
| `boolean(key, def)` | `strconv.ParseBool` | — |
| `duration(key, def)` | `time.ParseDuration` | positive |
| `httpURL(key)` | `net/url`.`Parse` | **see below** |
| `oneOf(key, def, allowed...)` | — | membership |

`url.Parse` is the trap in that table: it accepts almost anything, including `smtp.example.com`, which it happily reads as a *path* with no scheme. Check `u.Scheme` and `u.Host` yourself after parsing, or the validation does nothing.

Every one of them, on failure, calls `l.fail(...)` and returns a default — it does **not** return early. The caller keeps going and keeps collecting.

---

## Step 4 — `Load`, which is just a list

This is the "one place that names every setting" from the README. With no struct tags and no reflection, it's exactly what it looks like:

```go
func Load() (Config, error) {
    var l loader

    cfg := Config{
        server: ServerConfig{
            port: l.intInRange("PORT", 8080, 1, 65535),
            host: l.optString("HOST", "0.0.0.0"),
        },
        db: DBConfig{
            dsn: l.requireString("DATABASE_URL"),
        },
        // ...
    }

    return cfg, errors.Join(l.errs...)
}
```

`errors.Join` bundles many errors into one, and returns **`nil` when the slice is empty** — so the happy path needs no special case. Printing the joined error puts one message per line. That's your entire error report.

Read that literal again: it is the config schema. Someone new can see every setting the application takes, its type, its default and its bounds, in one screen. That is worth more than any amount of clever automation, and it's why the root README rules out struct tags.

---

## Step 5 — Crash properly

In `cmd/demo/main.go`, and in every real app that uses this:

```go
cfg, err := config.Load()
if err != nil {
    fmt.Fprintln(os.Stderr, "configuration is invalid:")
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
}
```

Three details:

- **`os.Stderr`, not stdout.** Errors go to stderr so they survive redirection and show up in logs.
- **`os.Exit(1)`, not `panic`.** Panic prints a stack trace, which is noise — nothing crashed, you *decided* to stop. A clean message and a non-zero status is the whole interface.
- **`os.Exit` does not run deferred functions.** Right here that's harmless, since nothing is open yet. Later, in `Jobs`, it very much won't be — remember it now.

Do this in `main` and nowhere else. `Load` returns an error like any other function; only the top of the program is allowed to decide to die.

---

## Step 6 — The `.env` convenience, kept honest

Optional, and thirty lines. Read the file, and set only what isn't already in the environment:

```go
k, v, ok := strings.Cut(line, "=")
if !ok { continue }
if _, exists := os.LookupEnv(k); !exists {
    os.Setenv(k, v)
}
```

Wrap that in a `bufio.Scanner` loop, skip blanks and `#` comments, `strings.TrimSpace` both halves. Call it at the top of `Load`, before any reader runs.

Two rules that keep this from becoming a liability:

**A missing file is not an error.** Production has no `.env`; that's the normal case. Check for `os.IsNotExist` and return `nil`.

**Real environment variables always win.** That's what the `LookupEnv` guard above is doing. Get this backwards and a stale `.env` on someone's laptop overrides what their shell says, and you get a bug that reproduces on exactly one machine.

---

## Step 7 — Printing the config without leaking it

Give `Config` a `String()` method. Any type with `String() string` is used automatically by `fmt` — which means once this exists, someone who logs the whole config object by accident gets the redacted version. You're not building a debug feature; you're building a safety net.

```go
func (c Config) String() string {
    return fmt.Sprintf("PORT=%d DATABASE_URL=%s", c.server.port, redacted(c.db.dsn))
}
```

Write it by hand and name the safe fields explicitly. Every field you add is redacted until you make it visible, which is the "default to redacting what you don't recognize" rule from the README, enforced by the fact that you had to type it.

**The trap:** inside `String()`, never `fmt.Sprintf("%v", c)` on the same value. `%v` calls `String()`, which calls `%v`, which calls `String()` — stack overflow. Format the *fields*, never the receiver.

For `redacted`, showing a hint beats showing nothing: `"postgres://***"` or a length, so a person can tell "wrong password" from "no password".

---

## Step 8 — One test that actually proves it

Go's testing is built in. A file ending `_test.go`, functions starting `Test`, no framework:

```go
func TestLoadReportsAllMissing(t *testing.T) {
    t.Setenv("PORT", "not-a-number")   // DATABASE_URL deliberately left unset

    _, err := Load()
    if err == nil {
        t.Fatal("expected an error, got nil")
    }
    if !strings.Contains(err.Error(), "DATABASE_URL") ||
       !strings.Contains(err.Error(), "PORT") {
        t.Fatalf("both problems should be reported, got: %v", err)
    }
}
```

`t.Setenv` sets a variable for one test and restores it afterwards — that's why you can test a config loader without wrecking your shell. (It also prevents `t.Parallel()` in that test, deliberately: environment variables are global, and two parallel tests would fight.)

That single test covers the property the entire system exists for. Add a happy-path one and stop; there's no need for a case per reader.

Run with `go test ./...`.

---

## The mistake everyone makes at the end

You've built grouped config so you can hand `Storage` only its own settings. So you write:

```go
// in Storage/storage.go — WRONG
func New(cfg config.StorageConfig) *Storage
```

That's `Storage` importing `Config`. Rule 1, broken, on your first system.

Each system declares **its own** small settings struct, with plain exported fields, knowing nothing about where the values came from:

```go
// in Storage/storage.go — right
type Config struct {
    Root      string
    MaxUpload int64
}
func New(cfg Config) *Storage
```

And the *application* — which is allowed to import both — does the wiring:

```go
store := storage.New(storage.Config{
    Root:      cfg.Storage().Root(),
    MaxUpload: cfg.Storage().MaxUpload(),
})
```

Yes, those field names are written twice. That duplication is the seam that lets you copy `Storage/` into your next project without dragging this config package along. It's the cheapest coupling you will ever decline to create.

(And note `Storage.Config` has ordinary exported fields — no accessor ceremony. Immutability is *this* system's job. Once a value has been handed over, the receiving system just holds a struct.)

---

## Order of work, if you want a checklist

1. `config.go` — types and accessors, no loading yet. `go build ./...` should pass.
2. `loader` plus `requireString` and `intInRange` only.
3. `Load` with two or three real settings. Wire up `main.go`. Watch it refuse to start.
4. The remaining readers, one at a time.
5. `dotenv.go`.
6. `String()` and redaction.
7. The test.

Steps 1–3 are one sitting and give you a working system. Everything after is filling in.

---

**Back to:** [`Config/README.md`](README.md) — the design and the danger zones.
