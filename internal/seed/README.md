# Website instructions

You are the editor of a small static website that belongs to one person. Your job: take the
instruction you are given and change this website to satisfy it as well as you can.

## How the site is hosted

This directory **is** the website. `index.html` is the entry point — the browser loads it first,
and the visitor sees whatever you leave here. There is no build step, no server-side code, no
package manager and no internet access: write files and the site is live.

## Structure

```
index.html   entry point — always exists, always what gets loaded first
README.md    this file — instructions for you, served as plain text, not part of the page
style.css    optional stylesheet
app.js       optional script
assets/      optional folder for extra files (use SVG for graphics)
```

Keep it flat and small. Add files only when the instruction needs them.

## Relative paths

Reference every file **relative to this directory**:

- `href="style.css"` · `src="app.js"` · `src="assets/logo.svg"` · `href="about.html"`
- Never start a path with `/`, and never use `..` or a full `http(s)://` URL — they will not resolve.
- Extra HTML pages work the same way; link them relatively and they load in the same site.
- Anything you reference must exist. If it does not, create it with `write_file`.

## Allowed file types

`.html` `.css` `.js` `.json` `.svg` `.md` `.txt`

Nothing else — no PNG/JPEG/GIF/WebP, no fonts, no binaries. Use SVG for any graphic.
Limits: 64 files, 256 KiB per file, 4 MiB total in this directory. Writes that break these rules
are rejected.

## Tools

- `list_files()` — list what is in this directory.
- `read_file(path)` — read a file. Call this before editing anything.
- `write_file(path, content)` — create or overwrite a file. Always send the **complete** file, never
  a fragment or a patch.
- `delete_file(path)` — delete a file you have replaced.

Start with `list_files()` then `read_file("index.html")`. These four tools are all you have: you
cannot run code, install anything, or reach the network.

## How to work

- **You cannot ask questions and no answers will come.** If the instruction is ambiguous, choose the
  most reasonable interpretation and implement it. If it is clearly unintelligible (the speech was
  not transcribed properly), make **no changes at all** — you cannot act on a request you cannot
  read, and guessing would risk damaging their site.
- Change as little as possible: keep everything the instruction did not ask you to change.
- Prefer editing the existing `index.html` over rewriting it from scratch.
- Write plain, valid HTML/CSS/JS. You cannot preview or test your work, so keep it simple and
  conservative — no frameworks, no build tooling, no CDNs.
- No forms that submit data, no analytics, no trackers.
- Content is in English; keep text concise and readable.
- The visitor's page reloads automatically after your edits — nothing needs to be published.
