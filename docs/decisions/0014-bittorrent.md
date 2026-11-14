# 0014 — BitTorrent downloads (magnet links and .torrent files)

- Status: accepted
- Date: 2026-11-14

## Context
Users expect a download manager to take magnet links and `.torrent` files. The engine has a
protocol plug-in point (`engine.Protocol`, commit 02d26f6): a protocol only moves bytes into
`Job.PartPath`; the engine owns the queue, pause, the `.peregrine.part` name and its atomic
rename, and Mark-of-the-Web. BitTorrent is different from our other sources in three ways
that need a deliberate choice: it **listens** for incoming peers, it **uploads** to strangers
while downloading, and the file names come from **untrusted metadata** that can hold paths.
Release builds use `CGO_ENABLED=0` (CLI, native host, Windows desktop), so everything must
build without cgo.

## Decision
Package `engine/proto/torrent` (kind `"torrent"`) built on **`github.com/anacrolix/torrent`**
(v1.62, MPL-2.0, the only mature pure-Go BitTorrent client: DHT, PEX, magnet metadata
exchange (BEP 9), MSE encryption, v1 and v2 torrents).

**Matching.** `magnet:?…xt=urn:btih:…` links, and http(s) URLs whose path ends in `.torrent`.
The `.torrent` file is fetched with the engine's HTTP client (proxy, timeouts) and the
browser's headers, capped at 20 MB, and parsed before anything else happens.

**One client per protocol instance**, started lazily on the first torrent download (so the
app opens no port until the user starts a torrent). It listens on TCP **46881** (and UDP for
the DHT) on all interfaces; if that port is taken it falls back to a random free port.
`Config.ListenPort` overrides it (`-1` = any free port). The host app calls `Close()` on
shutdown, after the Manager.

**Each Run** adds the torrent with deferred storage, waits for metadata (magnets) while
honouring ctx, calls `SetName(info.Name)` while the job is Fresh, then opens a file storage
that maps the data onto `Job.PartPath`:
- single-file torrent: the file *is* `PartPath`;
- multi-file torrent: `SetDirectory()`, files go inside the `PartPath` folder.

It reports `SetSize`, and every second `SetDownloaded(bytes completed)` and
`SetActive(active peers)`. When Run returns, the torrent is dropped from the client (files
closed, data kept), so the engine can rename it.

**Resume.** `SetResumable(true)`. The saved state holds the info hash, the raw info
dictionary (the metadata), the trackers and web seeds, so a resumed magnet needs neither the
DHT nor the original `.torrent` URL. Piece completion is kept in memory only; on each Run the
library hash-checks the data already on disk and only fetches missing pieces. (No piece
database file next to the download, nothing to clean up, and a crash can never mark
unwritten data as complete. The cost is one read of the existing data per resume.)

**Behaviour and privacy defaults.**
- **No seeding after completion.** Run returns as soon as every piece is verified, and the
  torrent is dropped. Peregrine is a downloader, not a seedbox.
- **Upload while downloading** is inherent to BitTorrent (peers trade pieces). It is capped
  at **2× the download limit**, or **1 MiB/s** when downloads are unlimited.
- **Download speed limit.** The library rate-limits per client, not per torrent, so the
  client limit is the sum of the running torrents' `Job.SpeedLimit()` (unlimited if any is
  unlimited), re-read every second. The global limit is applied through `SpeedLimit()` too,
  but HTTP and torrent downloads don't share one bucket: the global limit is approximate when
  both run at once.
- **DHT and PEX on**, so magnets work without trackers. Tests turn the DHT off
  (`Config.DisableDHT`) and use only loopback peers.
- **Encryption preferred** (MSE header obfuscation preferred, plaintext still accepted so we
  can talk to most of the swarm).
- **No UPnP / NAT-PMP**: we never open ports on the user's router.
- **No uTP**: one transport (TCP), the same behaviour with and without cgo (the library uses
  libutp with cgo and a pure-Go uTP without).
- **No WebTorrent** (WebRTC, websocket trackers) and no Local Peer Discovery.
- **Quiet logs.** The library's loggers are discarded: it logs tracker URLs (which carry
  passkeys) and peer addresses. Our errors never quote the magnet link or `.torrent` URL
  (the parser's message for a bad magnet is replaced; the engine redacts query strings
  from http URLs).
- **Paths from metadata are untrusted.** Every element of each file path goes through
  `engine.SanitizeFilename`; empty, `.` and `..` elements are dropped (`SafeRelPath`), and
  the library additionally refuses any path that is not under the storage folder. A
  malicious torrent cannot write outside `Job.PartPath`. The torrent name goes through
  `SetName`, which sanitizes it as well.
- Storage uses plain file I/O (no mmap, no library `.part` files, no sqlite or bolt piece
  database).
- Torrent traffic (peers, DHT, trackers, web seeds) does **not** use the HTTP proxy setting.

**What the dependency pulls in** (all pure Go with `CGO_ENABLED=0`; about 70 new modules linked):
anacrolix helpers (`dht/v2`, `log`, `missinggo`, `chansync`, `generics`, `go-utp`, `upnp`,
`envpprof`, …), `pion/*` (WebRTC, linked through the webtorrent package even though it is
disabled), `go.etcd.io/bbolt` (linked, unused), `RoaringBitmap`, `tidwall/btree`,
`lukechampine.com/blake3`, `multiformats/go-multihash`, OpenTelemetry API packages (no
exporter: nothing is sent anywhere), `coder/websocket`, and newer `golang.org/x/{net,crypto,
text,exp}`. With cgo enabled the library would also compile `go-llsqlite` and `go-libutp`.
Correction (after 0.3.0): the desktop app was not cgo-free as this said. Wails forces cgo on
for macOS, and on Windows Go turned it on because the CI runner has MinGW gcc. The 0.3.0
Windows exe needed MinGW's `libstdc++-6.dll` and `libgcc_s_seh-1.dll` and would not start.
Every desktop build now passes `-tags "nosqlite purego_utp"`, which keeps that C code out
even with cgo on. The Windows build also sets `CGO_ENABLED=0`, and
`scripts/check-no-mingw.sh` fails the release if an exe imports MinGW DLLs. `pion/dtls` and `pion/stun` were raised to versions without known
vulnerabilities (govulncheck clean). `anacrolix/envpprof` starts a localhost pprof listener
only if the `GOPPROF` environment variable asks for it.

## Alternatives considered
- **Write our own BitTorrent client** — months of work (DHT, metadata exchange, piece
  picking, encryption) and a large attack surface we would maintain alone.
- **libtorrent (rasterbar) via cgo** — the best client, but C++/cgo breaks our
  `CGO_ENABLED=0` release builds and cross-compiling.
- **Run an external client (aria2, qBittorrent-nox)** — another binary to ship and sign,
  an RPC port to secure, and no control over defaults.
- **Seeding to a ratio** — friendlier to the swarm, but surprising for a download manager
  (uploads continue after "Completed", ports stay busy). Could become an opt-in setting.
- **Persistent piece-completion database** (the library's sqlite/bolt default) — faster
  resume, but another file and another dependency at runtime, and it can disagree with the
  data on disk after a crash.

## Consequences
- Magnets and `.torrent` URLs download like any other job: queue, pause/resume, schedule,
  speed limits, Mark-of-the-Web on every file.
- The app now opens a listening port (when the first torrent starts): Windows Firewall asks
  once; macOS may ask to accept incoming connections. Declining still lets torrents
  download (outgoing connections only).
- While a torrent downloads, the user uploads to strangers and their IP is visible to the
  swarm. The UI and docs should say so.
- The binary grows noticeably (WebRTC and friends are linked though unused); revisit if the
  library gains build tags to leave them out.
- Resuming a large torrent re-reads the data already on disk once.
- The host app must call `Protocol.Close()` on shutdown.

## Addendum (2026-11-14): local .torrent files
`.torrent` files on disk are accepted as `file:///…/name.torrent` URLs (`torrent.FileURL`),
read with the same 20 MB cap. File URLs with a host (network shares) are refused. The
desktop app registers `.torrent` (wails.json `fileAssociations`) and turns a path argument,
a macOS Finder open or a file dropped on the window into such a URL. Once the metadata is
in the saved state, the original file is no longer needed.
