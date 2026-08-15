# Config — A Tutorial

**System 1 of 9.** The smallest complete system in the repo, and the one with no security surface, which makes it the right place to start.

---

## The problem, told as a story

It's 3am. Your phone goes off. A customer tried to reset their password and the application returned a 500 error.

You dig into it. The password reset flow sends an email. Sending email requires an SMTP host. The SMTP host comes from an environment variable. That environment variable was never set on the production server, so the value was the empty string, so the connection failed, so the handler exploded.

Here's the part that should bother you: **the application started up perfectly fine.** It ran for eleven days serving traffic. It was, in a meaningful sense, broken the entire time — it just hadn't reached the one code path that revealed it. The reset flow is rare. Nobody hit it for eleven days.

That's the problem this system solves. Not "how do I read an environment variable" — that's one function call. The problem is: **a misconfigured application should refuse to start, loudly, before it serves a single request, rather than quietly waiting to fail on some rare code path later.**

---

## The mental model

Think of your application's settings as a contract that gets checked at the door.

When the process starts, before it opens a network port, before it does anything else:

1. Read everything the application needs from the environment.
2. Check every single one — is it present, is it the right type, is it a sensible value?
3. If anything is wrong, print a message that says exactly which setting is wrong and what a correct one looks like, and **exit**.
4. If everything is right, produce one object holding all the settings, and hand it to the rest of the application.
5. Never look at the environment again for the rest of the process's life.

That's the whole system. It's small. But every decision in it earns its place.

---

## What this system does

- Loads settings from environment variables, plus optionally a file for local development convenience.
- Lets you *declare* each setting: what it's called, what type it should be, whether it's required, and what the default is if it isn't.
- Validates and converts: a port must actually parse as a number in a valid range, a URL must actually parse as a URL, an enum-ish setting must be one of the values you listed.
- Hands back a single object that the rest of the application reads settings from — and cannot modify.
- **This is the only system in the repo permitted to touch environment variables.** Everything else gets settings passed in as arguments.

## What this system does *not* do

**Secret storage and rotation.** Where secrets are kept, how they get onto the machine, how they get rotated — that's the deployment platform's job. Your config system's responsibility begins the moment a value exists in the environment. Trying to own secret management here means writing an encryption layer, a key store, and a rotation schedule, and you would be reimplementing something your hosting provider already does better.

**Values that change while running.** If a setting needs to change without restarting the process, it is not configuration. It's a feature flag, and that's a different system with a completely different design (see `Flags/`). The distinction matters because it drives the single most important property here: config is **frozen**. The moment you allow one value to be changeable at runtime, you've given up the guarantee that a setting validated at boot is still valid now.

**Per-user preferences.** A user's timezone is not configuration. It's profile data belonging to that user, and it lives in `Users/`. Configuration is per-*deployment*, not per-*person*.

---

## How to design it

### Everything is declared, nothing is discovered

The instinct is to write a helper that reads a variable when you need it, somewhere in the middle of your code. Resist this. It's precisely how you get the 3am story above — a setting that only gets read on a rare path only gets validated on a rare path.

Instead, there is one place — one list — that names every setting the application takes. Each entry declares:

- **The name** (the environment variable to read)
- **The type** (string, integer, boolean, URL, duration, one-of-a-list)
- **Whether it's required**, or what its default is if not
- **Any extra constraint** (a port between 1 and 65535, a positive number, a non-empty string)

Because the list is exhaustive and it's all checked at boot, an application that starts is an application whose configuration is entirely known-good. That's a genuinely strong guarantee, and it comes almost free.

### Group settings by system, don't use one flat namespace

Two shapes are possible. One flat bag of settings, or nested groups where each system's settings live together.

**Use groups.** The reason is rule 3 from the root README: config in, nothing global out. When your app starts the `Storage` system, it hands `Storage` only the storage settings. Not the whole config object.

This matters more than it looks like it does. If you hand the whole config object to every system, then every system *can* read the database password, the SMTP credentials, everything. Nothing stops a future version of `Storage` from quietly reaching for a setting that belongs to `Users`, and now those two systems are coupled through the config object, which is exactly the coupling the whole repo is trying to avoid. Passing a narrow slice makes the coupling impossible rather than merely discouraged.

There's a secondary benefit: when you read a system's setup code, you can see its entire configuration surface in one place, because it literally cannot use anything else.

### The result is immutable, and Go has a specific way to do that

"Immutable" here means: once the config object exists, nothing in the application can change a value in it.

Why does this matter? Because a mutable config object turns "what is this setting's value?" into a question you can't answer by reading. It becomes: *it depends when you ask, and on who ran first.* You lose the ability to reason about the running system. And it's a beautiful source of bugs that only appear under load, when two things run in a different order than they usually do.

Go doesn't have a keyword for "this object is frozen". You achieve it structurally instead:

- **Package boundaries.** In Go, whether something is visible outside its package is determined by capitalization: a name starting with a capital letter is exported (visible to other packages), a lowercase name is private to the package. So you store the actual values in lowercase fields — invisible outside the config package — and expose small capitalized methods that return copies of them. Code outside the package can read a setting; there's no field for it to assign to.

- **Return values, not pointers.** In Go, most values are copied when you pass them around. A pointer is an explicit reference to the original. If your accessor hands back a value, the caller gets their own copy, and scribbling on it doesn't affect yours. Hand back a pointer and you've handed over the original. For config, hand back values.

- **Watch out for maps and slices.** Go's copying is shallow: a map or a slice is internally a reference, so copying the struct that holds one still shares the underlying data. If a setting is a list or a map, copy the contents when you hand it out, or make the accessor return one item at a time instead of the whole collection.

### Fail loudly, and be specific about it

When validation fails, the message is the entire user interface of this system. Someone is reading it at 3am, or on their first day, or in a CI log with no other context. Make it good.

A bad message: `configuration error`.

A good message names the setting, says what was wrong with the value it found, and shows what a correct value looks like. "SMTP_PORT: expected an integer between 1 and 65535, got `smtp.example.com` (looks like you may have swapped SMTP_HOST and SMTP_PORT)." That last parenthetical is a nice touch when you can manage it, but the first three parts are non-negotiable.

**Report every failure at once, not just the first.** If four variables are missing, tell the person all four. Otherwise they fix one, redeploy, wait four minutes, and discover the next one. Collect all the errors, then print them all, then exit.

**Exit, don't return an error nobody handles.** For this specific system, at this specific moment — process startup, before anything has been opened — refusing to continue is correct. Print to standard error and exit with a non-zero status. This is the one place in the whole repo where crashing is the right answer, and it's because there's nothing yet to lose: no in-flight requests, no open transactions, no partially written files.

---

## Decisions to make before you write anything

**Grouped or flat?** Grouped, per the reasoning above. Each system receives only its own slice.

**What happens to an optional setting that's missing with no sensible default?** You have to pick a philosophy, and both are defensible:

- *It's an error.* Simple, predictable, no surprises. But it means a small app can't run without configuring features it doesn't use.
- *The feature is disabled.* If there's no SMTP host, notifications are off. Flexible — but now a typo in a variable name silently disables a feature instead of stopping you.

If you choose "disabled", **log a clear line at boot saying the feature is off and why.** The failure mode you're guarding against is someone spending two hours wondering why emails aren't sending. A single startup line saying "notifications disabled: SMTP_HOST not set" turns two hours into two seconds.

**Do you print the whole config at boot?** It's genuinely useful — you can see exactly what the process thinks its settings are. But you must redact anything secret. Redact by matching against key name patterns (anything containing "password", "secret", "token", "key", "credential", and so on), and — importantly — **default to redacting keys you don't recognize.** A redaction list that only hides what it knows about will eventually meet a setting nobody added to the list.

---

## Danger zones

Do not simplify these away.

**Never log a secret value.** The mechanism is the key-name pattern list above, plus defaulting to redacted for anything unfamiliar. The reason this is a real risk rather than a theoretical one: logs go places. They get shipped to a log aggregator, they get pasted into support tickets, they get screenshotted. A secret in a log line is a secret in twelve systems you weren't thinking about.

**Validate everything at boot, including settings only used on rare paths.** This is the entire point of the system. It is also the rule you'll be most tempted to bend, because validating a setting you're not currently using feels pointless. It is exactly the opposite: the rarely-used settings are precisely the ones that stay broken for eleven days.

**No lazy re-reads.** Load once, freeze, done. No "check if the file changed", no "re-read on demand", no cache invalidation. The moment there are two possible values for a setting over the process's lifetime, you've lost the guarantee that validation gave you.

---

## You'll know it works when

Take an application, remove one required environment variable, and start it. It should refuse to start, and the message should name the missing variable and describe what a correct value looks like — with no stack trace, no framework noise, just the fact.

Then set every variable correctly and start it again. It should come up, and the rest of the application should receive a typed settings object that it has no way to modify.

Then, for the lesson: try to modify it from outside the config package. It should not compile. That compile error is the whole system working.

---

**When you're ready to write it:** [`BUILDING.md`](BUILDING.md) — the same system from the other side: file layout, the Go you need at each step, and the traps.

**Next:** [`Logging/`](../Logging/README.md) — because you want it in place before you debug anything harder than this.
