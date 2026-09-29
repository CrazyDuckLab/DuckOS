# §30 API contract abuse torture — clean

## Scope

Destructive QA of `internal/api` HTTP boundary: bearer-token
contract, host gate, path manipulation, header injection,
method matrix, oversize requests, health-vs-privileged
endpoints, error sanitisation, security headers, loopback
aliases, cross-site blocking.

11 subtests, all pass on the existing code.

## Coverage

`internal/api/torture_test.go`:

| Test                              | Concern                              |
|-----------------------------------|--------------------------------------|
| `BearerContract`                  | every malformed Authorization → 401 |
| `PathManipulation`                | path-traversal / encoded variants    |
| `HeaderInjection`                 | CRLF / control chars in headers      |
| `MethodMatrix`                    | non-GET/HEAD → 405 on read endpoints|
| `OversizeRequest`                 | 64 KiB path / body-on-readonly       |
| `HealthUnauthenticated`           | only /health is unauthenticated      |
| `NoStackTraceLeak`                | engine errors sanitised in response  |
| `SecurityHeadersPresent`          | all 6 security headers on every resp |
| `LoopbackOnly`                    | non-loopback Host → 403/401          |
| `CrossSiteBlocked`                | Sec-Fetch-Site: cross-site → 403     |

`go test -count=1 -timeout 60s -run TestTorture_
./internal/api/...` — pass, 8 ms.

## Findings

No real defects. The existing handler is well-defended:

- Loopback gate enforces literal 127.0.0.1 / localhost / ::1.
- Origin and Sec-Fetch-Site are checked before any auth work.
- Read-only endpoints reject any body via
  `r.ContentLength != 0` and `r.URL.RawQuery != ""`.
- Authorization is split via `strings.CutPrefix` and verified
  with `subtle.ConstantTimeCompare`.
- Engine errors are mapped to 503 with sanitised bodies;
  `secret` / `internal` / `panic` / `goroutine` never leak.
- Six security headers (`X-Content-Type-Options`,
  `X-Frame-Options`, `Referrer-Policy`, `Permissions-Policy`,
  `Content-Security-Policy`, `Cache-Control`) are set on
  every response, both 2xx and 4xx.

## Boundary check

Management eth0 (`172.18.4.149/16`, default via
`172.18.1.254`, sshd pid 758) was not touched — these tests
use `httptest.NewRecorder` only, no real socket.
