# 0019 — Speed and remaining time

- Status: accepted
- Date: 2026-11-14

## Context
Up to 0.4.0 the engine smoothed the speed with a ~1.5 s average (0.7/0.3 every 0.5 s) and
computed the remaining time as `remaining / speed`. Torrents and busy servers deliver in
bursts, so the time swung between, say, 3 and 5 minutes several times a minute (194 s
jumps in half a second on a simulated bursty swarm), and when data stopped the speed and
time simply went blank, which looked like a hang. The owner wanted a time users can trust,
without adding a second speed or time column to the UI.

## Decision
One speed and one time on screen, answering two different questions (`engine/eta.go`):
- **Speed = now.** A short exponential average (time constant 2 s): it reacts within a
  couple of seconds, as people expect from a live speed.
- **Remaining time = when it will finish.** Based on a long average (time constant 20 s).
  If the live speed stays more than 50% above or below that average for 8 s, it is a real
  change (new peers, server slowed down) and the long average catches up with a 4 s
  constant.
- The time is shown as a **countdown**: it loses one second per second. Only when the new
  estimate stays more than 15% (and 3 s) away for 4 s does it glide toward it (8 s
  constant). No jumps.
- The first 5 s of data use the raw estimate (nothing better exists yet).
- **Waiting:** with no new bytes for 5 s (also before the first byte, e.g. a magnet link
  finding peers) the engine reports `ETA = -2` and speed 0; the UI says "Waiting…" with a
  tooltip, instead of a blank cell or a huge number. The countdown is frozen meanwhile.
- **Rounding in the UI:** seconds under a minute, 10 s steps under 10 minutes, whole
  minutes under an hour, 5-minute steps above.
- The same numbers reach the window, the tray tooltip and the CLI (`Download.Speed`,
  `Download.ETA`).

## Alternatives considered
- Show two speeds (current and average) or two times — more columns, more to read; the
  owner ruled it out.
- Remaining time from the whole-download average (some browsers) — very steady but wrong
  for a long time after a real change (e.g. a torrent that gains peers).
- A Kalman filter — more parameters, no visible gain over the countdown with hysteresis
  for this signal.

## Consequences
- `ETA` has a new value, -2 ("waiting"); old clients treat any negative value as unknown.
- Tests in `engine/eta_test.go` feed synthetic bursty, step and stalled traces: no step
  larger than 30 s per tick after warm-up, within 25% of the true time, a 4× speed change
  followed within 40 s, a steady 1 s/s countdown, and the waiting state.
