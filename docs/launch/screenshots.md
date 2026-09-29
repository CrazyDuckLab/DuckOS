# Screenshot qualification

Launch images come from implemented UI at an identified commit, using an isolated
environment. Record viewport, theme, build and whether data is observed or a
labeled fixture. Fixtures must never appear as production observations.

Capture Overview, Network, Firewall, Selective Routing, WireGuard, Client QR,
Subscriptions, Port Exposure, Safe Apply, Duck Doctor and Monitoring only when
the screen exists. Until then the screen has no launch image.

Inspect every pixel for usernames, endpoints, public IPs, keys, recovery codes,
subscription tokens and browser/terminal context. A QR encodes a credential even
when text is hidden: use a disposable revoked test client, label it as a test,
and confirm revocation before export. Never publish a working connection QR.

Use the compact text mark from [brand-mark.md](../brand-mark.md). Captions state
what is demonstrated, without implying untested reachability or certification.
