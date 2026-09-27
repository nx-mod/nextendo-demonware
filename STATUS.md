# What is missing, and what is unknown

What this server does today, what a complete Diablo III server would also do, and what we do not know yet. Each point says how it is known: **code** (read from this repository), **log** (seen in a live session), **binary** (from the game's own call sites), or **guess**.

## What is implemented

Auth and tickets; the encrypted lobby; NAT discovery and introductions; public-game matchmaking (create, update, delete, find, player counts); friend name lookups and per-player user data; server time; the publisher files (`Config.txt`, `Seasons.txt`, `Blacklist.txt`); optional weekly Challenge Rifts; presence reporting to nextendo-account. Of the roughly 30 remote-task call sites in the game binary, 10 are answered for real: 12/6, 12/9, 10/21, 21/1, 21/2, 21/3, 21/5, 21/12, 29/1 and 29/4 (binary, code).

## Framed on the `testing` branch: state kept, replies gated

These services previously took an empty success and dropped everything. The
`testing` branch now keeps their state and frames a reply. The state-keeping is
real and persisted; the typed **reply** for the tasks whose wire layout is not
captured from the Switch client is behind `D3_FRAMED_REPLIES=1` (off by default,
because an unrecognised reply shape can mark a service unavailable, while an
empty success is known-safe). Service names for 10, 12, 21 are confirmed; the
rest follow the bd SDK generation and the official API's board set.

| service | tasks | state kept now | reply status |
|---|---|---|---|
| 4 stats / leaderboards (`leaderboards.go`) | 1, 4, 5, 11, 13 | scores stored per board, best-kept; boards shaped after Blizzard's own D3 Game Data API and served on `/api/leaderboards` | submit parsed best-effort; read replies gated |
| 6 messaging (`messaging.go`) | 14 | persisted per-player mailbox (send/list/delete) for the season-swap flow | list reply gated |
| 10 storage (`herostorage.go`) | 10, 12, 13 | uploaded hero/"account" blob stored per player+file, persisted | upload is an empty success (correct); read replies gated |
| 23 counter (`misc_services.go`) | 1 | shared counters incremented and persisted, shown on the dashboard | new-value reply gated |
| 27 DML (`misc_services.go`) | 2 | server-owned DML blob (empty until configured) | blob reply gated |
| 29 user data (`friends.go`) | 5, 8, 11 | 29/1 set and 29/4 get now persisted; 29/11 paged query added | query rows gated; 5/8 still empty success |
| 67 event log (`misc_services.go`) | 6 | payload counted for the dashboard, then discarded (a sink) | empty success (correct) |
| 68 rich presence (`richpresence.go`) | 3, 5, 7 | implemented from the CTR pull request and our log | unconfirmed in a live game |

What still needs a live capture to finish: the request layout of the bdStats
submit task and the row layout each read task's reader expects (services 4, 10,
6, 23, 29/11). With one capture, turn `D3_FRAMED_REPLIES` on, adjust the row
writers to match, and the boards, hero downloads and mail become end-to-end.

Effect in game today, `D3_FRAMED_REPLIES` off: as before — leaderboards show but
are empty — but uploaded heroes, scores, counters and mail are now **stored** and
visible on the dashboard, so nothing is lost while the capture is pending.

## Missing: state and identity

- **Persistence (`store.go`, `testing` branch).** User data, hero uploads, counters, mail and leaderboards are now kept on disk (one JSON file each under `D3_STATE`, default `state/`, atomic writes, in-memory when `D3_STATE=off`), so they survive a restart. Matchmaking sessions are still in memory by design (a game outlives the server process only if its host reconnects); login tickets and identity files are written by auth as before.
- **No stable user IDs.** Every ticket carries user id 1, and the lobby connection id is a counter (code). Stable per-account IDs are still open in the notes.
- **No real subscription check.** `nso_subscription_status` is always `1` (code).

## Matchmaking

- **Session expiry — done (`testing` branch).** `updated` is now read: a session whose host stays connected but stops touching it is expired after `D3_SESSION_TTL` (default 900 s, 0 disables), both in the reaper and at read time. This is the documented stale-quick-match failure. Full games (`numPlayers >= maxPlayers`) are also skipped in find/friend results, so a searcher is never handed a game it cannot join (`matchmaking.go`).
- **Search filters are still ignored.** `findSessions` reads only query, start and max, and returns every other joinable session, up to 50. The per-attribute filter format is client-specific and not captured, so honouring it is left for a capture (code).
- **No player list per session, no region or skill logic** (code).
- **Peer-to-peer only.** The server introduces players and relays nothing of the game itself (code).

## Missing: friends and presence

- **Console friends.** Names come from `baas-proxy`'s log (local stack only) or, new, from nextendo-account's `/internal/resolve` (`accountlookup.go`). The account lookup is tested only against a stand-in service, never with real friend ids (code).
- **Only online players resolve** in `getUserNames`; offline friends do not (code).
- **The "friends online" count on devices** is still open: presence has to reach the account service that builds the console's friend list, which is private (notes).
- **Rich presence now resolves console friend ids (`testing` branch).** `getAndSubscribeRichPresence` (68/5) resolved friends only by raw PID; it now resolves a console's Nintendo device id to a PID via `onlinePlayerFor`, the same path the friend name lookup uses, and keys the reply row by the id the game asked about (`ctr_presence.go`). Still unconfirmed in a live Diablo III game.

## Missing: hardening

- **The all-zero lobby key can now be refused (`testing` branch).** `NEXTENDO_REQUIRE_TICKET=1` drops the zero key from the handshake candidates, so a client with no ticket cannot complete the handshake unidentified. Off by default because older tickets whose lobby-key field is null legitimately need the zero key; turn it on once every client issues a modern ticket (`handshake.go`).
- **No rate limits, no bans or moderation, no anti-abuse** (code).
- **Account gates fail open** when the account service is unreachable, by design; `NEXTENDO_REQUIRE_ACCOUNT` defaults to off (code).
- **IPv4 only** for NAT (code).

## Missing: content and settings

- **Blacklist entries.** The file is always sent empty (code); see Unknowns.
- **`update-1.cpk`.** The game asks for this content update on every login and is answered "absent" (log). A real server may ship patches through it.
- **Only one season block** in `Seasons.txt` (code).

## Unknown

- **Config keys.** Whether the game reads `Config.txt` keys beyond the 26 we send. The game has its own key list; d3hack's may be shorter (guess). The parser is at `0x6429C` and `0x65314` in the binary; tracing it would settle this.
- **Blacklist values.** What the `0`/`1` in `[GBID]` and `[SNO]` lines means (d3hack documents the line format only).
- **Date range.** The game may keep dates in 32 bits (d3hack limits its rift end to 19 Jan 2038). A season end of 1 Jan 2050 was served during the console trouble, but deleting the save fixed it on the old, unchanged files, so the date was probably not the cause. The server still clamps dates past the limit as a precaution.
- **Season and events.**
  - Whether the game accepts season numbers below 14 or above 39, and whether older builds accept later seasons at all: builds 2.7.6 and 2.7.7 are only tested with season 37.
  - The exact rule behind "not a seasonal hero". Seen once: after serving seasons 37, 69, 39 and 37 in turn, a console refused every new seasonal hero until its save was deleted, on files that had worked before; the server build and files were ruled out. Most likely the save records the season and a lower one is refused; not confirmed.
  - Whether `SeasonOnly` set to `0` crashes on drop: d3hack's comment says yes, its own file and ours write `0`.
  - What `ParagonCap` does.
  - How all 21 events behave together (only four have run in live co-op).
  - Whether the game re-fetches the config mid-session, which decides if a season change reaches a connected player.
- **XP multiplier.** Its exact meaning and any upper limit (`"1.0"` is normal; `"6000.0"` is untested for effect).
- **Challenge Rifts.** The request names come from d3hack's source, not a live capture. Whether the game accepts our rewritten config (times and hash) is untested. The menu needs a high character level.
- **Quick-match refusals.** A joiner once rejected a found game instantly when one session attribute differed (a flag set 1 versus 0). The cause was never identified.
- **Formats of the empty services** above: request and reply layouts for leaderboards, hero upload, mail and rich presence.
- **Other game versions.** Only the Switch title `01001B300B9BE000`, versions 2.7.6 and 2.7.7, protocol versions 200 to 220, have been seen.
- **Load.** One process, in-memory state; nothing is known about behavior with many players.

## How to close the gaps

1. Trace the config parser and the service-4, 10, 6 and 68 handlers in the game binary with Ghidra (the method is in `NOTES.md`). This answers the config keys and the missing formats.
2. Add a small logger to d3hack that records service, task and arguments for every task, so the missing formats can be read from a real session.
3. Test on a real device: the four unverified season and rift behaviors above, and a stale-session quick match.
