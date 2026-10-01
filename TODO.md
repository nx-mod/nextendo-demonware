# TODO — nextendo-diablo-3-nx

- **Features marked (testing)** in the README: try each on a console.
- **Diablo III News**: the channel exists in bcat-nx (`stack/news/channels.json`, `nx_news_diablo3`); confirm on a
  console and write real items.
- **Diablo III has no BCAT data**: confirmed on a console (nextendo-nx Game data) — cache 0, no passphrase. So
  there is no in-game BCAT content to serve; the Diablo III *news channel* (bcat-nx) is a separate thing and works.
  In-game messages, if any, would come through its Demonware service, not BCAT.
  (nextendo-nx → Diagnostics → Game data) and what it requests; then serve it from bcat-nx. In-game messages
  that come from the game's own service go through this server instead.

## Credits

- CollectingW — the Crash Team Racing server this shares its Demonware base with.
- The whole Nextendo Network team — https://nextendo.network. Nextendo is awesome.
