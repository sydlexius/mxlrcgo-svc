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
Usage: mxlrcgo-svc [fetch|serve|scan|library|keys|config]

Commands:
  fetch     fetch lyrics once without HTTP server or DB queue
  serve     run HTTP server, worker, and library scheduler
  scan      scan configured libraries and enqueue missing lyrics
  library   manage library roots
  keys      manage API keys
  config    inspect or update configuration

Legacy flag-only invocation is still supported:
  mxlrcgo-svc [--outdir OUTDIR] [--cooldown COOLDOWN] [--depth DEPTH] [--update] [--upgrade] [--bfs] [--serve] [--listen LISTEN] [--token TOKEN] [--config CONFIG] [SONG ...]
```

## Example:
### One song
```sh
mxlrcgo-svc adele,hello
mxlrcgo-svc fetch adele,hello
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
>
> **_The `-d/--depth` argument limits the depth of subdirectories to scan; use `-d 0` or `--depth 0` to only scan the specified directory._**

### Lidarr webhook server
```sh
MUSIXMATCH_TOKEN=YOUR_TOKEN MXLRC_WEBHOOK_API_KEY=mxlrc_your_webhook_key mxlrcgo-svc --serve --listen 127.0.0.1:3876
MUSIXMATCH_TOKEN=YOUR_TOKEN MXLRC_WEBHOOK_API_KEY=mxlrc_your_webhook_key mxlrcgo-svc serve --listen 127.0.0.1:3876
```

The server listens on `MXLRC_SERVER_ADDR` when `--listen` is not provided. Configure one or more webhook keys with `MXLRC_WEBHOOK_API_KEY`, use `mxlrcgo-svc keys create`, or put the server address and webhook keys in a config file and start with `mxlrcgo-svc serve --config path/to/config.toml`.

### Library and key management
```sh
mxlrcgo-svc library add /music --name Music
mxlrcgo-svc library list
mxlrcgo-svc scan
mxlrcgo-svc keys create --name lidarr --scope webhook
mxlrcgo-svc keys list
mxlrcgo-svc config get db.path
```

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
