
Hi! While AI was used to create these READMEs and the BUILDING.MDs, any actual code is all me! This series is how I'll be teaching myself Go, as well as the inner workings of some of these backend systems.
Feel free to follow along, or just use the snippets I've written! (I can't quite guarantee that they'll be the best options for you, however)
-Chris

# ModularSystems — A Tutorial Series

This is a set of nine small backend systems that almost every real application ends up needing: logins, settings, logs, emails, file uploads, background work, an audit trail, feature switches, and an internal admin screen.

You are going to build all nine, by hand, in Go.

These documents are not API reference. They are tutorials. Each one explains what a system is *for*, what shape it should have, why that shape and not another, and what goes wrong when you get it wrong. There is deliberately no code in them. Code you copy teaches you nothing; a design you understand lets you write the code yourself and — more importantly — lets you debug it at 3am when it misbehaves.

**Each folder holds two documents.** `README.md` is the design — read it first, and read all of it. `BUILDING.md` is the companion for when you sit down to write the thing: file layout, the specific Go you need at each step, the order to build in, and the traps that cost an afternoon. It contains fragments — a signature, three lines to show a mechanism — never a finished implementation. The gaps are the exercise, and they are where the learning is.

---

## Who this is written for

Someone who can already program. You know what a function is, what a loop does, what a database roughly is, what an HTTP request roughly is. You may have written some JavaScript, Python, or Java.

You have written little or no Go. That's fine — each tutorial explains the Go-specific idea it needs at the moment it needs it, in plain language. You will not be asked to memorize the language up front.

---

## Why build these yourself?

There are libraries for every one of these. Installing them is faster. So why not?

**Because "install the library" is a skill with a ceiling, and "understand the system" is not.** Every one of these nine systems is something you will encounter for the rest of your career. If you have built a session system once, then every session system you ever meet — in any language, any framework — is legible to you. You know what the cookie flags are for. You know why revoking a token is hard when the token is signed and easy when it's opaque. You know the shape of the bug before you find it.

The libraries are still the right call in a hurry. That's not the goal here. The goal here is the second thing:

> **Two goals, in this order: understand how these work by building them, then have them ready for the next project. Where those conflict, learning wins.**

A system you wrote and can defend beats one you assembled.

---

## What "modular" means here, and why it's hard

The word gets used loosely. Here it means something specific and slightly uncomfortable:

**Each of these nine systems must not know that the other eight exist.**

Not "should ideally avoid". Must not. The `Users` system contains no reference to `Billing`. The `Jobs` system contains no reference to `Notifications`. When two systems need to cooperate — and they will — the cooperation happens in the *application* that hosts them both. The application imports both systems and wires them together. The systems themselves stay ignorant.

Why is this worth the discomfort? Because the moment `Billing` imports `Users`, you no longer have two modules. You have one framework with two folders in it, and you cannot take `Users` to your next project without dragging `Billing` along, which drags whatever `Billing` imports, and so on until you're carrying the entire old application into the new one. This is the single most common way a "reusable library" stops being reusable, and it happens gradually, one innocent import at a time.

Keeping the rule is a discipline, not a technique. There's no tool that enforces it for you. You just have to notice, every time you're about to add an import, whether you're about to weld two systems together.

### The four rules

These apply to every system in this repo, and each tutorial assumes them.

**1. No cross-imports.** As above. Systems talk through the app that hosts them, never to each other. If `Notifications` needs to know a user's email address, the app looks the address up and passes it in. `Notifications` never asks `Users` anything.

**2. Runs standalone.** Every system has a small demo entry point that works with nothing else present. If you can't start it up alone, on a blank machine, and watch it do its one job, it isn't a module — it's a fragment of an application. This rule is what keeps rule 1 honest, because the demo won't compile if you've secretly created a dependency.

**3. Config in, nothing global out.** A system receives its settings as arguments when the app creates it. It never reads environment variables, never reads a global variable, never reaches out to a config file on its own. There is exactly one exception, and it's the `Config` system, whose entire job is reading the environment. Everything else gets handed what it needs.

Why does this matter so much? Because a system that reads globals cannot be tested twice with different settings in the same test run, cannot be instantiated twice in one process, and cannot be understood by reading its own source — you have to go hunting for who set the global and when. Passing settings in is a small amount of extra typing that buys all of that back.

**4. Near-zero dependencies.** Go's standard library, or your own code. Two exceptions across all nine systems: a Postgres driver (Go has no built-in one) and one cryptography package for password hashing (the standard library doesn't ship the right algorithm).

The rule of thumb: **when something looks like it needs a package, that's usually the part worth writing.** The moment you feel the urge to install something is usually the moment you've arrived at the interesting problem. That urge is the signal that you're about to learn something.

---

## The nine systems

| Folder | What it does |
| --- | --- |
| `Config/` | Loads and validates settings when the app boots, and crashes loudly if any are wrong |
| `Logging/` | Structured logs, with an ID tying together every line from one request |
| `Users/` | Identity: accounts, passwords, login, sessions, roles |
| `Notifications/` | Sends email/SMS/push — and a dev mode that pretends to |
| `Storage/` | File upload and retrieval: local disk in dev, cloud storage in prod |
| `Jobs/` | Work that outlives the request that asked for it |
| `AuditLog/` | An append-only record of who changed what |
| `Flags/` | Runtime switches: ship-dark toggles and emergency kill switches |
| `Admin/` | An internal table UI for looking at and fixing rows |

---

## The order to build them in, and why

Don't build these in a vacuum. **Build a real application alongside them, and let the application pull each system out of you as it needs one.**

Not a test harness — something you would genuinely use. It needs a login, a file upload, and at least one operation slow enough that doing it during a request would feel wrong. That application is what tells you which decisions actually matter. Systems designed in the abstract encode your guesses about what will be needed. Systems extracted from working code encode what was actually needed. The difference shows up as roughly half the features you would otherwise have built and never used.

| # | System | Why it's here | What building it teaches you |
| --- | --- | --- | --- |
| 1 | `Config` | The smallest complete system, and nothing security-critical | Failing fast at boot, avoiding globals, immutable objects |
| 2 | `Logging` | You want this before you debug anything harder | Structured output, correlating one request's lines |
| 3 | `Users` | The big one. Most of the learning in this repo lives here | Password hashing, session lifecycle, cookie flags, CSRF, timing attacks |
| 4 | `Notifications` | Arrives whether you want it or not — password reset needs email | A dev mode that logs instead of sending |
| 5 | `Storage` | Follows naturally once you have users with things | Path traversal, content-type sniffing, signed URLs |
| 6 | `Jobs` | The first genuinely concurrent system | Database row locking, at-least-once delivery, idempotency, dead letters |
| 7 | `AuditLog` | Meaningless until you have an actor from `Users` | Append-only discipline enforced by the database |
| 8 | `Flags` | Needs something worth gating before it makes sense | Failing to a safe default, TTL caching, sticky percentage rollouts |
| 9 | `Admin` | Always last | Composing the other eight; per-request authorization |

The dependencies in that table are conceptual, not code-level. `AuditLog` needs the *idea* of an actor to be useful, but it does not import `Users` — the app passes an actor identifier in.

---

## The most important instruction in this repo

Every tutorial has a section called **Danger zones**. Each one lists rules with a specific attack next to them.

**Do not just read those. For each one, build the broken version first, and break it yourself.**

Write a token comparison using ordinary equality, then actually recover a token from your own server byte by byte off response timing. Leave off the login rate limit, then run a credential-stuffing loop against your own box and watch it work. Put a user-supplied filename into a storage key, then fetch a file three directories above where it should be able to reach.

This will feel like a waste of time. It is not. Reading "use a timing-safe comparison" gives you a rule that you will apply inconsistently, in the places you remember, for about six months. Extracting a token from your own server gives you a reflex that fires every time you type an equals sign next to a secret, forever.

That gap — between a rule you've read and a reflex you've earned — is the entire reason to build these by hand instead of installing them.

---

## The technology choices

You don't need to understand all of these yet; each tutorial explains the ones it uses. This is here so you can see the shape of the whole thing.

| Layer | Choice | Why |
|---|---|---|
| Language | Go, current stable release | Small language, excellent standard library, easy concurrency, single-binary deploys |
| HTTP | Go's built-in `net/http`, with its pattern-matching router | Since Go 1.22 the standard router handles method and path parameters. You do not need a framework |
| Password hashing | Argon2id, from the extended crypto package | The standard library doesn't ship a memory-hard hash, and you need one |
| Sessions, tokens, one-time codes | Standard library only — random bytes, hashing, constant-time comparison | Every primitive you need is already there |
| Database | Postgres, in development *and* production | Not SQLite in dev. See below |
| Database driver | `pgx`, using its own native interface | Faster and more expressive than routing through Go's generic database interface |
| Migrations | Numbered SQL files plus a tiny runner, embedded into the binary | About thirty lines of code replaces an entire tool |
| Config validation | Written by hand, one setting at a time. No struct tags, no reflection | This *is* what the `Config` system is. Automating it away removes the lesson |
| Admin UI | Go's built-in HTML templating, server-rendered | It escapes output by default, which quietly removes a whole class of bug |
| Tests | Go's built-in testing package and plain comparisons | Go's testing story genuinely doesn't need a framework |
| Frontend | TypeScript, talking to these over JSON/HTTP only | Not a system in this repo. It never imports one — it calls the HTTP endpoints an app exposes |

**Why Postgres in development, not SQLite?** Because the `Jobs` system depends on a specific Postgres feature for safely handing work to multiple workers, and SQLite has no equivalent. If you develop against SQLite you cannot test the exact concurrency behaviour that is hardest to get right — you'd find out it was wrong in production. Using the same database in both places also eliminates an entire category of "works locally, fails deployed" bug.

**Packaging:** one Go module, one package per folder. To reuse a system in a new project, you copy the folder. That's it. Turn a folder into its own published module on the day a *second* project actually wants it — not before. Premature packaging costs you version numbers, release processes, and changelogs in exchange for nothing.

---

## Two things about Go that change what you build

If you're coming from Node or Python, two standard-library features here are stronger than what you're used to, and it shrinks two of these systems.

**Structured logging is built in.** Go ships a logging package that already emits machine-readable structured output with levels and key/value fields. So the `Logging` system isn't "write a logger" — it's "write a small piece that plugs into the built-in one to make it pretty in development, plus a way to carry a request ID around." Still absolutely worth writing. Just smaller than you'd expect.

**HTML templates escape by default.** Go's HTML templating understands HTML context and escapes values automatically based on where they land — inside a tag, inside an attribute, inside a script block. This means the `Admin` system stops being a place where you can accidentally hand-roll a cross-site-scripting hole. You have to work quite hard to create one.

---

## What is deliberately *not* here

**Not yet — no folder until a project needs one:** billing and subscriptions, rate limiting, API keys, CSV import/export, outbound webhooks with retries, search, scheduled tasks, analytics events.

**Teams / organizations / invites — deliberately deferred, and worth understanding why.** This looks like a tenth system you could add later. It isn't, and this is the single most expensive architectural mistake in this category.

Adding organizations changes *who owns every row in your database*. Every query in the entire application grows a filter restricting it to the caller's organization. Miss that filter on exactly one query and you have a cross-tenant data leak — one customer reading another customer's data. There is no way to add this safely as an afterthought, because "add a filter to every query, and never forget one" is not a thing humans do reliably across a codebase that already exists.

So: decide it at the *start* of a project. Even a trivial version, where every user silently gets their own personal one-member organization, is enough — because it puts the ownership column in every table from day one and makes the filter a habit before there are five hundred queries. Or accept, consciously, that it will be expensive later. What you must not do is add half of it.

**Never — these teach little and cost a lot:**

- **OAuth provider flows ("Log in with Google").** The specification has several interacting security parameters, and every provider deviates from it in its own particular direction. It's endless per-provider maintenance, and after the first one you've learned everything transferable there is. Build password login first. Add a provider as its own separate, scoped project if a real application demands it.
- **WebAuthn / passkeys.** Binary encoding formats, cryptographic key formats, attestation certificate chains. This is the one place in this whole repo where taking a real dependency is straightforwardly correct.
- **TLS, the Postgres wire protocol, JSON parsing.** Already solved, correctly, below the level you're working at.
- **Internationalization.** Genuinely not needed yet, and the wrong problem to solve speculatively.

---

## One last thing: building it and shipping it are different milestones

Building the `Users` system yourself is one achievement. Running it against real people's real credentials is a different, later one.

The first is a weekend. The second comes after you've attacked your own implementation and failed to break it — and it is a *deployment* decision, not a code decision. There is no line of code that flips it from "learning project" to "safe to hold strangers' passwords."

Nothing else in this repo carries that gap. Ship `Config`, `Jobs`, and `Flags` the day they pass their own checks.

---

Start with [`Config/`](Config/README.md). It's the smallest one, and everything else takes its settings from it.
