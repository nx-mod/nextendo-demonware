# Nextendo Network — functionality audit

_Audit run: 2026-09-27. Method: `go build ./... && go vet ./... && go test ./...`
on every Go module, plus live boot + endpoint integration tests of the HTTP
services and a boot test of the game/Demonware servers. C++ homebrew repos are
reviewed but cannot be compiled here (no devkitPro/Aether/libnx toolchain)._

## Verdict

Every Go server in the network **builds clean, vets clean, and passes its
tests** (12/12). The HTTP service servers were booted and their real endpoints
exercised — all healthy after one fix. One genuine runtime bug was found and
fixed (Tagaya 500 on missing `versions.json`). The game/Demonware servers boot
and bind; full play verification needs a live console + TLS certs (by design).

## Go servers

| Repo | Role | Build/Vet/Test | Live-tested | Branch | Notes |
| --- | --- | --- | --- | --- | --- |
| nextendo-demonware (diablo-3) | Demonware: D3 + CTR | ✅ (65 tests) | boots; auth needs cert.pem | testing | state dir auto-creates; TLS required for auth listener |
| advance-wars | NEX (DataStore) | ✅ (5) | build only¹ | testing | binds :443 + secure UDP 60015 |
| borderlands-1 | NEX | ✅ (2) | build only¹ | testing | secure UDP 60012 |
| torchlight-2 | NEX | ✅ (2) | build only¹ | testing | secure UDP 60014 |
| super-smash-bros-ultimate | NEX (arena) | ✅ (2) | build only¹ | master | secure UDP 60016 |
| nextendo-bcat-nx | BCAT/d4c delivery | ✅ (8) | ✅ health+stats+route | testing | serves signed BCAT containers, local key |
| nextendo-tagaya-nx | title version list | ✅ (3) | ✅ list+ETag+304 | testing | **fixed**: embedded fallback (was 500 w/o file) |
| nextendo-telemetry-nx | telemetry sink | ✅ (2) | ✅ any-path → 200 {} | main | capture surface, optional dump |
| nextendo-aauth-nx | app-auth JWT | ✅ (3) | ✅ token+JWKS | main | RS256, /.well-known/jwks.json |
| nextendo-npns-nx | push (WebSocket) | ✅ (2) | ✅ register+notify+400 | main | live-or-queue, RFC6455 hand-rolled |
| nextendo-gamespy-nx | NWFC / GameSpy | ✅ (4) | ✅ NAS login round-trip | main | NAS done; GameSpy TCP/UDP = capture only |
| nextendo-eos-nx | Epic Online Services | ✅ (4) | ✅ token+connect+lobby+unhandled | fall-guys | in-memory auth/lobby + unhandled recorder |

¹ NEX servers listen on :443 with TLS and a secure UDP port; a full session needs
a real console (PRUDP client) and certs, so build/vet/test is the verifiable bar
here. They started and bound in earlier runs.

## The fix pushed this pass

**nextendo-tagaya-nx @ testing** — `buildVersionList` read `versions.json`
straight off disk, so a deploy without that file in the working directory
returned **500 on every `/tagaya/hac_versionlist` request** instead of serving a
list (a console would then nag "software update required" or gate online play).
Fixed by embedding the repo's `versions.json` via `go:embed` and falling back to
it when no on-disk file exists; a real `TAGAYA_VERSIONS` file still overrides, and
a genuine read error (permissions) still surfaces. Regression test added. Verified
200 in an empty directory where it previously 500'd.

## Fragility scan (whole network)

Grepped every server for the "read a path, hard-fail if absent" pattern that bit
Tagaya. **Tagaya was the only one that surfaced the error to the client.** All
others guard reads with `if _, err := …; err == nil` and degrade to empty/default
(NEX `datastore/`, bcat `deliverycache`, demonware session/pubfiles dirs), or
create the directory with `MkdirAll` on startup (demonware `D3_STATE`). No further
instances to fix.

## What is genuinely complete vs. capture-surface

- **Complete + testable now:** Demonware D3 service framework (leaderboards, hero
  storage, mail, counters, user data, persistence), Tagaya, Telemetry, aauth
  (JWT+JWKS), NPNS (register/queue/deliver), EOS (auth/connect/lobby), BCAT
  container build/verify + local signing key, GameSpy **NAS** auth.
- **Capture-surface (needs a live console capture to finish the wire format):**
  GameSpy GP/SB/QR/NatNeg TCP/UDP (accept + log), EOS unhandled endpoints
  (recorded + 200), Demonware/NEX unhandled tasks. This is deliberate: implement
  the verifiable core, log the rest so a real capture maps it.
- **By-design external requirements:** NEX servers + Demonware auth need TLS
  certs (home-lab supplies them); BCAT needs its signing key (generated locally
  via `server pubkey`).

## C++ / homebrew repos (reviewed, not compiled here)

| Repo | State |
| --- | --- |
| nextendo-prelude → **nextendo-nx** (branch `nextendo-nx`) | Trimmed + restructured onto Aether; core (hosts/apply) ported and reviewed, UI scaffold follows Aether's API. Needs a devkitPro+Aether build (see its NOTES.md). |
| exefs-logger-nx | Universal RE logger framework (from bl1-hack/d3hack). |
| bl1-hack (main=universal logger, `bl1` branch=BL1 specifics) | Restructured. |
| d3hack | Source for the logger framework. |
| zerotier-p2p-nx / libzt-nx | Audited; native-lib work deferred (not pushed per user). |

## Recommendations (next passes)

1. **Branch hygiene:** telemetry/aauth/npns/gamespy default to `main` and eos to
   `fall-guys` locally; if the intent is "testing is where LAN fixes land," make
   sure each has a `testing` branch (Tagaya/Demonware/BCAT/NEX already do).
2. **Optional home-lab QoL:** the TLS servers (NEX, Demonware auth) exit without
   `cert.pem`. A self-signed auto-generate on missing cert would make them run
   out-of-the-box on the LAN (Prelude installs `disable_ca_verification`, so the
   console won't reject a self-signed cert). Left as a proposal, not a change.
3. **Live captures** to convert the capture-surfaces above into implementations:
   GameSpy per-game GP/SB exchange, EOS SDK-version request bodies.
4. **nextendo-nx build:** stand up devkitPro + Aether and work the first-build
   checklist in its NOTES.md.
