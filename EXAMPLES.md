# nextendo-demonware — example usage

`./run.sh` starts the Demonware backend (auth HTTPS :8460, NAT, lobby ports,
dashboard :8093). It serves **Diablo III** and **Crash Team Racing: Nitro-Fueled**,
which use Activision/Blizzard's Demonware infrastructure, not Nintendo's.

Demonware is a binary TCP protocol (bdByteBuffer tasks), so there is no simple
`curl` example — a real console (or the test suite) drives it:

```sh
./run.sh &
curl -s localhost:8093/api/stats     # live counts: sessions, lobbies, leaderboards
go test ./...                        # exercises the service/task handlers
```

Implemented: auth, matchmaking/lobby, D3 leaderboards, hero storage, mail,
counters, user data, persistence (see README). Unhandled tasks are recorded as a
capture surface.
