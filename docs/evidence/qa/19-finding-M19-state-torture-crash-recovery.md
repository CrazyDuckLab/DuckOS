# M-19 — Real state.Store Safe Apply torture + crash recovery

**Severity:** N/A (positive qualification, regression coverage
landed)
**Section:** §12 (Safe Apply torture), §13 (power-loss / crash)
**Status:** §12.1-12.3 PASS, §13.1-13.4 PASS
**Found during:** Phase E §12/§13 hostile qualification

## 1. §12 Safe Apply torture — concurrent writers + stale + rollback

| Sub | Test | Result |
|---|---|---|
| §12.1 | 16 writers × 10 iters Prepare+Commit race | PASS |
| §12.2 | Stale revision rejected, conflict not overwriting | PASS |
| §12.3 | Rollback recovers pending transaction | PASS |

§12.1 final state: 16 writers × 10 iters converged to a single
consistent revision. Conflicts (ErrConflict, ErrPending) were
treated as expected races, not failures.

§12.2 critical invariant: when admin B commits first and admin A
still holds a stale revision, A's commit MUST be rejected.
Verified: A's `Commit` returned an error; final state remained B's
revision.

§12.3 critical invariant: after a transaction is stuck in
`RollingBack` and `RolledBack` is called, a fresh transaction
from the original baseline succeeds. Verified.

## 2. §13 power-loss / crash recovery — atomic rename survival

| Sub | Test | Result |
|---|---|---|
| §13.1 | state.tmp tampered after close | PASS |
| §13.2 | state.json truncated to half | PASS |
| §13.3 | Dir wiped empty | PASS |
| §13.4 | Other files in state dir | PASS |

§13.1: After committing one transaction and closing, we wrote
garbage to `state.tmp` (the file state.Store uses for atomic
rename). `OpenExisting` correctly read the previous committed
state (value=2). The atomic-rename semantics mean the
un-renamed `state.tmp` is invisible to recovery.

§13.2: Truncating `state.json` to half its size causes
`OpenExisting` to refuse to open. The store fails closed on
detected corruption rather than returning a partial state.

§13.3: Wiping the entire state dir and calling `Open` again
provisioned a fresh store with the new initial config. Recovery
from total loss works.

§13.4: A spurious `stale.txt` file in the state dir does NOT
prevent `OpenExisting` from reading the journal. The store
tolerates non-state files.

## 3. Linux qualification: management boundary preserved

```
eth0   UP   172.18.4.149/16   fe80::b755:7a90:2ad0:437d/64
default via 172.18.1.254 dev eth0 proto dhcp src 172.18.4.149 metric 1002
758 sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups
```

(verified at start and end of each batch)

## 4. Commits

- `feat(qa+state): §12 Safe Apply torture + §23 leak metrics + §8.4 IPv6 firewall`
- `docs(qa): §13 §12 M-19 state torture + crash recovery`
