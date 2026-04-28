# mxlrcgo-svc
[![build](https://github.com/sydlexius/mxlrcgo-svc/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/sydlexius/mxlrcgo-svc/actions/workflows/ci.yml)

Command line tool to fetch synced lyrics from [Musixmatch](https://www.musixmatch.com/) and save it as *.lrc file.

## Download
### Standalone binary
**TBA**

### Build from source
Required Go 1.25+
```sh
go install github.com/sydlexius/mxlrcgo-svc/cmd/mxlrcgo-svc@latest
```

---

## Usage
```text
Usage: mxlrcgo-svc [--outdir OUTDIR] [--cooldown COOLDOWN] [--token TOKEN] SONG [SONG ...]

Positional arguments:
  SONG                        song information in [ artist,title ] format (required)

Options:
  --outdir OUTDIR, -o OUTDIR  output directory [default: lyrics]
  --cooldown COOLDOWN, -c COOLDOWN
                              cooldown time in seconds [default: 15]
  --depth DEPTH, -d DEPTH     (directory mode) maximum recursion depth [default: 100]
  --update, -u                (directory mode) update existing lyrics file
  --bfs                       (directory mode) use breadth-first-search traversal
  --token TOKEN, -t TOKEN     musixmatch token (or set MUSIXMATCH_TOKEN environment variable or create a .env file)
  --help, -h                  display this help and exit
```

## Example:
### One song
```sh
mxlrcgo-svc adele,hello
```
### Multiple song and custom output directory
```sh
mxlrcgo-svc adele,hello "the killers,mr. brightside" -o some_directory
```
### With a text file and custom cooldown time
```sh
mxlrcgo-svc example_input.txt -c 20
```
### Directory Mode (recursive)
```sh
mxlrcgo-svc "Dream Theater"
```
> **_This option overrides the `-o/--outdir` argument which means the lyrics will be saved in the same directory as the given input._**

> **_The `-d/--depth` argument limit the depth of subdirectory to scan. Use `-d 0` or `--depth 0` to only scan the specified directory._**

## Development

Run the lightweight CLI smoke test:

```sh
make smoke
```

---

## How to get the Musixmatch Token
Follow steps 1 to 5 from the guide [here](https://spicetify.app/docs/faq#sometimes-popup-lyrics-andor-lyrics-plus-seem-to-not-work) to get a new Musixmatch token.

## Token Configuration

A Musixmatch API token is required. Supply it using any of the following methods (listed in order of precedence):

1. **`--token` CLI flag** — highest priority
   ```sh
   mxlrcgo-svc --token YOUR_TOKEN adele,hello
   ```

2. **`MUSIXMATCH_TOKEN` environment variable**
   ```sh
   export MUSIXMATCH_TOKEN=YOUR_TOKEN
   mxlrcgo-svc adele,hello
   ```

3. **`.env` file** — place in the working directory where you run the command
   ```sh
   MUSIXMATCH_TOKEN=YOUR_TOKEN
   ```

## Credits
* [Spicetify Lyrics Plus](https://github.com/spicetify/spicetify-cli/tree/master/CustomApps/lyrics-plus)
