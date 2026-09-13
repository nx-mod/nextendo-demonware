# d3-server

A self-hosted Demonware backend for **Diablo III on Nintendo Switch**, built to
run alongside a local [Nextendo Network](https://nextendo.network) stack so the
game can go online without contacting Activision/Blizzard servers. Source only,
no game assets, no keys. Not affiliated with Blizzard, Activision, Demonware or
Nintendo.

| component | role |
|---|---|
| `d3-auth` | Demonware `/auth/` login (HTTPS JSON), issues tickets |
| `d3-lobby` | encrypted lobby (TCP 3074), remote tasks, NAT probes (UDP 3074) |
| `d3-pubfiles` | season / community-event / blacklist publisher files |

Protocol details and the reverse-engineering method are in [NOTES.md](NOTES.md).

## Credits

- **[Nextendo Network](https://nextendo.network)** ([NextendoNetwork](https://github.com/NextendoNetwork))
  — the Switch online stack this server plugs into: NSA/BaaS accounts, dauth,
  the SNI router that carries Demonware traffic, and the NEX server core that
  set the pattern for these per-game servers.
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
  — Demonware lobby/service emulation of the same SDK generation.
- **[skkuull/dwd3](https://github.com/skkuull/dwd3)** (GPL-3.0),
  **[jordam/demonbugger](https://github.com/jordam/demonbugger)**,
  **[hosseinpourziyaie/demonware-companion](https://github.com/hosseinpourziyaie/demonware-companion)**
  — Demonware reverse-engineering tools.

Everything specific to Diablo III (ticket layout, lobby handshake and crypto,
task reply format, the service/task map) was reverse-engineered from the game
binary; see NOTES.md for addresses.
