# Management TLS

The API supports a TLS-only loopback listener with `--tls-cert` and `--tls-key`.
Both flags are required together. They accept PEM files provided through an
administrator-controlled deployment process, not a browser upload path.

Startup rejects missing/oversized/nonregular files, symlinks, hardlinks, unsafe
ownership, writable public certificate files, public private keys, mismatched
certificate/key pairs and certificates outside their validity interval. The
certificate must contain the IP SAN `127.0.0.1`. Include `localhost` and `::1`
SANs too when clients will use those names. Private key permissions are 0600 or
0640, owned by root or the service identity; never place keys in Git or argv.

The listener requires TLS 1.3. Origin validation uses HTTPS in TLS mode and
ignores forwarded scheme headers. It does not bind a new LAN/WAN address,
configure firewall rules, trust a certificate on client machines or weaken
certificate verification. No automatic HTTP-to-HTTPS redirect listener exists.

Example service arguments, added to the existing loopback invocation:

```text
--tls-cert /etc/duckos/management-cert.pem --tls-key /etc/duckos/management-key.pem
```

Use a trusted local CA or deliberately provision and verify a device
certificate using a separate trusted channel. The current installer does not
generate or deploy certificates automatically; it retains its documented
loopback HTTP/SSH-tunnel baseline. Do not expose that HTTP listener publicly.
Do not use `--insecure` or disable verification in a health checker. A checker
for a private CA must use an explicitly provisioned trusted CA file.

Certificate enrollment/renewal/rotation, management-network binding, HSTS policy,
ACME for published Sites, certificate UI and first-run trust experience remain
unfinished. They must not be inferred from TLS transport support alone.

Verification: `internal/api/tls_test.go` runs real local TLS handshakes with a
synthetic certificate and an explicit trust pool. It verifies TLS 1.3 success,
TLS 1.2 rejection, HTTPS Origin enforcement and unsafe certificate/key failures.
The test does not disable client certificate verification. No Linux target or
administrator credential is involved.
