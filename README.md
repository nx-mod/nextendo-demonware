# nextendo-diablo-3-nx

**Written from scratch by nx-mod:** the Demonware server for **Diablo III** (Nintendo Switch) on the Nextendo
Network. Part of [nextendo-testing](https://github.com/nx-mod/nextendo-testing): the whole network, run on a LAN.

Diablo III's online layer is Demonware, not NEX. This server speaks it end to end (auth, the encrypted lobby,
remote tasks, matchmaking, NAT discovery) and plugs into the stack like the other game servers: an sni-router
route, nextendo-account's gates and presence, `/api/stats` for the dashboard.

## What works

- Login and "connected to the Diablo network"; season and community events from `pubfiles.json`
  ([PUBFILES.md](PUBFILES.md), [SEASONS.md](SEASONS.md)); season 37 tested on builds 2.7.6 and 2.7.7.
- Public games: create, find, join, player counts, NAT introductions; co-op between a Switch and an emulator.
- Friend lookups and presence reported to nextendo-account (lightly tested).

## New features (testing)

Added but not yet tried on a running game:

- Leaderboards and stats stored per board, shaped after Blizzard's Diablo III API, served on `/api/leaderboards`
  (typed replies behind `D3_FRAMED_REPLIES=1`) (testing).
- Hero uploads, counters and mail stored; everything persisted under `D3_STATE` across restarts (testing).
- Matchmaking: abandoned games expire (`D3_SESSION_TTL`), full games hidden from search (testing).
- `NEXTENDO_REQUIRE_TICKET=1` refuses ticketless logins (testing).
- Console friends' rich presence resolved to Nextendo PIDs (testing).
- Monthly season rotation and weekly Challenge Rifts (testing).

## Run

In nextendo-testing, `run_all.ps1` starts it. On its own: `go build`, then set `CERT_FILE`/`KEY_FILE`,
`NEXTENDO_HOST`, `NEXTENDO_SECRET_FILE` and run it next to `pubfiles.json`.

| port | what |
|---|---|
| 8460 TCP (TLS) | auth, behind sni-router (`crimson-switch-auth3.*.demonware.net`) |
| 3074 TCP / UDP | lobby (`crimson-switch-lobby.*`) / NAT discovery (`stun.*.demonware.net`) |
| 8093 HTTP | `/api/stats`, `/api/leaderboards`, `/healthz` |

Hosts, settings, limits and the to-do list are in [README.full.md](README.full.md), [NOTES.md](NOTES.md) and
[STATUS.md](STATUS.md). Changing the served season can damage a savegame.

## Credits

- **The Nextendo Network team** — the stack this server plugs into — https://nextendo.network.
- **[D3Hack](https://github.com/god-jester/D3StudioFork)** by jester, on [exlaunch](https://github.com/shadowninja108/exlaunch)
  by Shadow — instrumentation, and the publisher-file formats.
- Demonware references (protocol facts read and reimplemented, no code copied):
  [shield-development](https://github.com/project-bo4/shield-development),
  [open-bitdemon-emulator](https://github.com/Laupetin/open-bitdemon-emulator),
  [boiii-free](https://github.com/Ezz-lol/boiii-free),
  [bo2-wiiu-demonware](https://github.com/Protarium-Network/bo2-wiiu-demonware).
- **[CollectingW](https://github.com/CollectingW)** — Crash Team Racing Nitro-Fueled support, merged into this
  server; their CTR server: [CollectingW/diablo-3 (crash-team-racing)](https://github.com/CollectingW/diablo-3/tree/crash-team-racing).

Nextendo is awesome.
