# Easy Mode

Easy Mode is DuckOS V1's single-page dashboard. It is the default
landing page after login. Easy Mode exists so a non-technical user
can see whether the system is working, and a technical administrator
can find the affected subsystem at a glance.

Easy Mode does not configure anything by itself. It is a view, not a
control. Configuration lives in Full Mode (see
[`full-mode.md`](full-mode.md)) and goes through Safe Apply (see
[`safe-apply.md`](safe-apply.md)).

## What Easy Mode shows

The page has five tiles:

- **Internet** — the link to Easy Mode's Internet wizard.
- **Wi-Fi** — the link to the Wi-Fi configuration wizard.
- **Devices** — the link to the device inventory.
- **VPN** — the link to the WireGuard and Xray tunnel configuration.
- **System** — the only tile that surfaces a verdict. Easy Mode
  renders **Status check incomplete** by default; **Some things need
  attention** when Doctor has raised a finding.

Above the tiles is a headline. The headline is one of:

- **Some things need attention** — Doctor or the overview has raised a
  finding. Open Doctor.
- **Status check incomplete** — the public health endpoint responded,
  but the production contracts do not yet provide enough evidence to
  claim verified health. The system may be working; Easy Mode refuses
  to claim it is until the backend grows verified-health evidence.
- **Status unavailable** — the public health endpoint did not
  respond. The browser can talk to the page; the backend cannot be
  reached.

## What Easy Mode deliberately does not say

Easy Mode does not say **Everything is working** when Doctor returns
zero findings. Doctor empty ≠ verified healthy. It does not say
**Observation fresh** for the same reason. The UI is honest about
which evidence the backend currently provides.

See [`docs/architecture.md`](../architecture.md) and the centralized
derivation in `web/src/state/health.ts` for the implementation.

## The Internet tile

The Internet tile shows the live link-layer state and the nameservers
DuckOS is using. It does not prove Internet connectivity. Today no
production API proves Internet connectivity end-to-end; the tile
therefore surfaces **Connectivity not verified** until that evidence
exists. Open the **Configure Internet** action to launch the Easy Mode
wizard when the wizard is qualified for your target.

## The System tile

The System tile is the only tile that surfaces a verdict. The verdict
mirrors the centralized derivation. See
[`doctor-and-diagnostics.md`](doctor-and-diagnostics.md) for what each
verdict means.

## Switching to Full Mode

Click **Full** in the top bar to enter Full Mode. The same Easy Mode
data appears under the relevant by-domain pages, plus the dense
configuration views. Click **Easy** to return.

## When to use Easy Mode

- You want a quick answer to "is the system working?"
- You want to find the affected subsystem when something is wrong.
- You want to hand the device to a non-technical user.

When you need to make a change, switch to Full Mode.