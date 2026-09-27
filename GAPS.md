# Nextendo Network — gap analysis

_What the network does, and what it could/should do that it doesn't yet. "Verify"
= may already exist as a repo I don't have attached (some nx-mod repos are
private / not in this session); "GAP" = no implementation seen anywhere in the
work so far. 2026-09-27._

## Server services

### Present and working
demonware (Diablo III + CTR), NEX secure servers (advance-wars, borderlands-1,
torchlight-2, super-smash-bros-ultimate + wildcard-routed titles), bcat delivery
(nextendo-bcat-nx), tagaya (version list), telemetry sink, aauth (application
auth JWT+JWKS), npns (push), gamespy NAS (NWFC), eos (Epic / Fall Guys).

### Gaps / to verify
| Piece | Status | Why it matters |
| --- | --- | --- |
| **dauth (device auth)** | **GAP/verify** | Hosts redirect `dauth-lp1.ndas.srv.nintendo.net` to us, but only *application* auth (aauth-nx) is implemented. Device-auth tokens are a separate endpoint; several titles/system flows want one. No `nextendo-dauth-nx` seen. |
| **BaaS / Nintendo Account (`nx-account`)** | **verify** | `accounts.nintendo.com`, `*.baas.nintendo.com` are redirected, and demonware calls `NEXTENDO_ACCOUNT_URL` (127.0.0.1:8080) for presence. An account/BaaS server is clearly assumed but not in this session's repos — confirm it exists; if not it's the biggest gap (login, NA tokens, friends presence). |
| **NPLN (Splatoon 3)** | **GAP** | Splatoon 3 uses NPLN (gRPC/HTTP2), routed by hosts but with no backend. Needs a `nextendo-npln-nx` (KeepUserSession stream on 7575, gamesync, dragons). Today S3 only gets 304s from conntest, not real play. |
| **nncs2 responder** | **verify** | Hosts point `nncs2-*` at a specific IP running a UDP responder (10025/10125). Essential for MK8/Splatoon NAT traversal. Confirm it's a repo (`nextendo-nncs2-nx`?) — if it only lives on the VPS as an untracked script, that's a gap. |
| **conntest responder** | **verify** | `conntest.nintendowifi.net` / `ctest.cdn.nintendo.net` must answer 200 + `X-Organization: Nintendo`. Probably the sni-router's nginx; confirm it's tracked. |
| **sni-router / TLS terminator** | **verify** | The whole HTTP stack assumes something terminates TLS on :443 and routes by SNI to each backend (the servers serve plain HTTP behind it). This is core infra — confirm the router is a repo. |
| **friends / presence (NEX)** | **partial** | demonware has its own friends; a network-wide `nn::friends` service (for NEX games' friend rosters) likely rides on BaaS — verify. |

## Client apps (NRO)

### Present
- **nextendo-nx** (was Prelude) — network switcher, cert-trust provisioning, BCAT
  host redirect. Trimmed + Aether rewrite in progress.
- **exefs-hack-nx** (was exefs-logger-nx) — RE/logger framework; **bl1-hack**,
  **d3hack** per-game forks.

### Gaps / nice-to-have
- **A status/diagnostics NRO** (or fold into nextendo-nx's Diag screen — already
  scaffolded): show which servers are reachable on the LAN, which mode is loaded,
  patch status. Would make "does my home-lab work?" self-serve.
- **tl2-hack / aw-hack** loggers — the framework README lists them but only
  bl1/d3 exist. Low priority (loggers, not play-critical).
- **A one-shot BCAT content pusher** companion to bcat-mitm (push a splatfest /
  event into the delivery cache), so BCAT isn't just "stops erroring."

## Sysmodules

### Present (as patches / modules)
- **nextendo-bcat-mitm-nx** — BCAT public-key swap (new this pass).
- Cert-trust via Prelude/nextendo-nx exefs patches + CA bundle.
- **ldn-mitm / ryu-ldn** (in the build superproject) for LAN play.

### Gaps / to verify
| Piece | Status | Why |
| --- | --- | --- |
| **network_mitm (account-link fallback)** | **GAP as a repo** | Old Prelude bundled sysmodule `4200000000000666` (ssl:s Client-PKI fallback for blanked PRODINFO). It lived only inside Prelude's romfs; it has no repo of its own. Should be `nextendo-network-mitm-nx` so it's maintained and buildable. |
| **dns.mitm** | **n/a** | Atmosphère provides it; nextendo-nx just writes the hosts. Correct — no gap. |
| **A sysmodule to serve BCAT data locally** | optional | bcat-mitm makes signatures pass; actually *delivering* event data still needs the server + a push. Covered by nextendo-bcat-nx, so not a sysmodule gap. |

## Bottom line

The **play-critical gaps** are: (1) confirm the **BaaS/account** server and the
**sni-router** exist as repos (everything else assumes them), (2) **dauth**
device auth, (3) an **NPLN** backend for Splatoon 3, and (4) the **nncs2 /
conntest** responders being tracked. The **maintainability gaps** are pulling
**network_mitm** out of Prelude into its own repo. Everything the network already
implements builds, runs, and is tested (see NETWORK_AUDIT.md).
