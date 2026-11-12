# Peregrine extension: privacy policy

**Short version: no data leaves your computer.**

The extension sends download details only to the Peregrine desktop app on the same
computer. It uses the browser's Native Messaging channel and a local helper program
(`peregrine-host`). That helper talks to the app over `127.0.0.1` with a random access token.

## What the extension handles

When the extension takes over a download, or you choose "Download with Peregrine", it
sends the app:

- the download URL, the page it came from (referrer), the file name, type and size if known,
- the cookies the browser would send for that URL, and the browser's user-agent string,
  so downloads that need a login keep working.

To list media in the popup, it keeps the addresses, types and sizes of audio and video
responses for each open tab. That list stays in browser session storage. It is deleted when
you navigate away, close the tab or close the browser.

Your settings (on/off, file types, minimum size, excluded sites) are stored locally in the
browser.

## What the extension never does

- It has no analytics, telemetry, tracking, ads or remote code.
- It makes no network requests of its own. It only reads the headers of requests the
  browser already makes.
- It never sends data to the developers or any third party.
- It never reads page content, except the list of links on a page when you choose
  "Download all links with Peregrine".

The helper's log file (`peregrine-host.log` in the Peregrine settings folder) records method
names and URLs. Query strings are removed from those URLs, and cookies are never logged.

## Contact

Open an issue in the project's GitHub repository.
