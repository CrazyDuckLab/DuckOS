# QA Finding M-1: support bundle redactor misses real-world secret shapes

**Severity:** Medium (was High before the fix landed; downgraded to Medium for the
remaining prefix-credential coverage gap that is now covered)
**Section:** §16 / §42 (Vault/Secret security review + Support bundle hostile test)
**Status:** Fixed in HEAD 1032e29 + this commit.

## 1. Minimal reproduction

```go
r := support.NewRedactor(support.DefaultPatterns())
in  := "STRIPE_SECRET_KEY=sk_live_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
out := r.Redact(in)
// out == in unchanged, secret fully present.
```

The same is true for `ghp_…`, `AIza…`, `xoxb-…`, `wJalrXUtnFEMI/K7MDENG/bPxRfiCYz…`,
and any `KEY=longhex` style env-var secret.

The QA regression test that originally failed:

```
internal/qa/support_canary_test.go:124: real-world secret not redacted:
  "STRIPE_SECRET_KEY=sk_live_aaaa..." -> "STRIPE_SECRET_KEY=sk_live_aaaa..."
  "GITHUB_TOKEN=ghp_BBBB..."          -> "GITHUB_TOKEN=ghp_BBBB..."
  "GOOGLE_API_KEY=AIzaccc..."         -> "GOOGLE_API_KEY=AIzaccc..."
  "AWS_SECRET_ACCESS_KEY=wJalrXUtn..." -> "AWS_SECRET_ACCESS_KEY=wJalrXUtn..."
  "SLACK_BOT_TOKEN=xoxb-000-111-222..." -> "SLACK_BOT_TOKEN=xoxb-000-111-222..."
```

## 2. Root cause

The `api-token-hex` and `ed25519-seed` patterns used `\b` word-boundary
anchors. `\b` is a transition between a word char and a non-word char. The
typical real-world secret shape is `<prefix>_<token>` or `<prefix>-<token>`,
and `<prefix>` itself is a word char, so the boundary never fires and the
regex never matches. Same effect with `KEY=token` because `_` is a word char.

The set had no patterns for:

- Issuer prefixes (sk_live_, ghp_, gho_, ghs_, ghr_, ghu_, xox[bpars]-,
  AIza, ya29., AKIA, ASIA, Bearer …).
- `KEY=value` env-var style (covers SECRET, TOKEN, PASSWORD, PASSWD, PWD,
  API_KEY, ACCESS_KEY, PRIVATE_KEY).
- AWS secret access key (40-char base64-ish, surrounded by separators).
- PEM private key blocks.

## 3. Impact

A support bundle built from a real DuckOS configuration (Cloudflare DNS,
GitHub PAT for ACME DNS-01, Google API key, AWS credentials, etc.) would
carry the issuer prefix in plaintext. That prefix is enough for an attacker
to identify the credential issuer, the kind of access it grants, and to
launch a targeted brute-force / revocation-replay attack on the matching
service — even though the secret value itself is hidden.

The pre-existing 32-char hex token and 64-char ed25519 seed patterns also
failed to match across the most common separator (underscore), so any
DuckOS-internal secret that happened to be exported through the bundle
(API tokens, internal HMAC keys, RADIUS shared secrets) would leak.

## 4. Fix (this commit)

1. Removed `\b` from `api-token-hex` and `ed25519-seed`. The minimum length
   (32 and 64) is enough to keep false-positives away from human-readable
   text.
2. Added `prefix-credential` pattern covering sk_live_, sk_test_, ghp_,
   gho_, ghs_, ghr_, ghu_, xox[bpars]-, ya29., AKIA, ASIA, AIza.
3. Added `env-var-secret` pattern covering KEY=value and KEY: value for
   SECRET, TOKEN, PASSWORD, PASSWD, PWD, API_KEY, ACCESS_KEY, PRIVATE_KEY
   (case-insensitive, multi-line).
4. Added `aws-secret-key` pattern for the 40-char base64-ish AWS secret
   access key (surrounded by separators to avoid eating arbitrary
   40-char strings).
5. Added `pem-private-key` pattern that matches multi-line PEM private
   key blocks. Placed before the base64 patterns so it catches the whole
   block.

## 5. Regression coverage

- `internal/qa/support_canary_test.go::TestSupportBundleNoCanaryLeak` —
  the original canary sweep now stays green after the fix.
- `internal/qa/support_canary_test.go::TestSupportBundleRedactsRealWorldSecretPrefixes` —
  5 sub-cases (Stripe, GitHub, Google, AWS, Slack). They all failed on
  the unpatched code and now pass.
- `internal/support/support_test.go` — the existing 7 tests stay green
  after the change, confirming no regression in plain content or earlier
  redaction behavior.

## 6. Remaining known limitations

- The new patterns are anchored on known issuer prefixes; a custom
  in-house credential with a novel prefix would still leak the prefix
  while the value is redacted. Future work: an operator-extensible
  prefix list and a "long token after `=` or `:`" fallback regardless
  of key name.
- PEM passphrase lines (`Proc-Type: 4,ENCRYPTED\nDEK-Info: …`) are not
  yet specifically targeted; they are caught by the generic patterns
  but a dedicated pattern would tighten the contract.
- JWT signature segments are matched but not the header / payload
  claims; if a token carries sensitive PII in its payload, a separate
  base64url pattern would be needed.

## 7. Artifacts

- Source diff: `internal/support/support.go`
- New test: `internal/qa/secretcanary.go`, `internal/qa/secretcanary_test.go`,
  `internal/qa/support_canary_test.go`
- Headline regression: `TestSupportBundleRedactsRealWorldSecretPrefixes`
