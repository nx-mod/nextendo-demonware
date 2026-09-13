# diablo-3

Game server for **Diablo III** on Nintendo Switch, for [Nextendo Network](https://nextendo.network). Source only — no binaries, no certs, no game assets. Not affiliated with Blizzard, Activision, Demonware or Nintendo.

Diablo III does not use NEX: its online layer is **Demonware**. This server speaks it end to end — auth, the encrypted lobby, remote tasks, matchmaking, NAT discovery — and plugs into the Nextendo stack like the other game servers: a route in sni-router, the account gates and presence of nextendo-account, and `/api/stats` for nextendo-dashboard.

## What works

- Login and "connected to the Diablo network"; season and community events served from `pubfiles.json`
- Public games: create, find, join, player counts, NAT introductions
- Co-op between a Switch and an emulator (tested: CFW Switch, Citron 2.7.7)
- Friend lookups inside the game, and presence reported to nextendo-account

## Build

    go build -o server.exe .

Standard library only.

## Run

See `example.env`.

| port | what |
|---|---|
| 8460 TCP (TLS) | Demonware auth, behind sni-router (`*.demonware.net` → `BACKEND_D3`) |
| 3074 TCP | lobby |
| 3074 UDP | NAT discovery and introductions |
| 8093 HTTP | `/api/stats`, `/healthz`, `/pubfiles/` |

DNS must send `crimson-switch-auth3.*.demonware.net`, `crimson-switch-lobby.*.demonware.net` and `stun.{us,eu,jp,au}.demonware.net` to the stack; the game must never reach the real Demonware.

    server.exe pubfiles    # regenerate Config.txt / Seasons.txt / Blacklist.txt and exit

The lobby reads the publisher files on every request, so changing the season or an event needs no restart.

Protocol notes and the reverse-engineering method are in [NOTES.md](NOTES.md).

## Credits

- **[Nextendo Network](https://nextendo.network)** ([NextendoNetwork](https://github.com/NextendoNetwork))
  — the Switch online stack this server plugs into: NSA/BaaS accounts, dauth,
  the SNI router that carries Demonware traffic, and the game-server pattern
  (gates, presence, dashboard) this server follows.
- **[D3Hack](https://github.com/god-jester/D3StudioFork)** by **jester**, on
  [exlaunch](https://github.com/shadowninja108/exlaunch) by **Shadow** — used
  as the instrumentation platform (hooks and logging inside the game) and as the
  source of the publisher-file formats (`Config.txt`, `Seasons.txt`, `Blacklist.txt`).

Demonware reference implementations consulted (all for other titles). Protocol
facts were read from them and **reimplemented** here in Go; no code was copied.

- **[project-bo4/shield-development](https://github.com/project-bo4/shield-development)** (GPL-3.0)
  — Demonware STUN/NAT-discovery packet format (UDP 3074, types 20/21 and 30/31);
  service ID table.
- **[Laupetin/open-bitdemon-emulator](https://github.com/Laupetin/open-bitdemon-emulator)** (AGPL-3.0)
  — standalone Demonware backend; reference for service result layouts.
- **[Ezz-lol/boiii-free](https://github.com/Ezz-lol/boiii-free)** (GPL-3.0)
  — Demonware lobby/service emulation of the same SDK generation; bdMatchMakingInfo layout.
- **[Protarium-Network/bo2-wiiu-demonware](https://github.com/Protarium-Network/bo2-wiiu-demonware)**
  — NAT traversal packet format (introductions).
- **[skkuull/dwd3](https://github.com/skkuull/dwd3)** (GPL-3.0),
  **[jordam/demonbugger](https://github.com/jordam/demonbugger)**,
  **[hosseinpourziyaie/demonware-companion](https://github.com/hosseinpourziyaie/demonware-companion)**
  — Demonware reverse-engineering tools.

Everything specific to Diablo III (ticket layout, lobby handshake and crypto,
task reply format, the service/task map) was reverse-engineered from the game
binary; see NOTES.md for addresses.
