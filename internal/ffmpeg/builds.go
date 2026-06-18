package ffmpeg

import "fmt"

// version names the pinned FFmpeg build. It is the cache subdirectory name
// (<cacheDir>/ffmpeg-<version>/ffmpeg), so bumping it provisions into a fresh
// directory instead of reusing a stale binary.
const version = "n8.1.2"

// releaseTag is the immutable BtbN FFmpeg-Builds snapshot the pinned artifacts
// come from. BtbN's "latest" tag is rolling (its asset hashes drift on every
// rebuild), so we pin a dated autobuild snapshot whose assets never change.
const releaseTag = "autobuild-2026-06-17-14-17"

const btbnBase = "https://github.com/BtbN/FFmpeg-Builds/releases/download/" + releaseTag + "/"

// archiveKind selects the decompressor used during provisioning.
type archiveKind int

const (
	archiveTarXz archiveKind = iota
	archiveZip
)

// platformKey identifies a build by runtime.GOOS / runtime.GOARCH.
type platformKey struct {
	os   string
	arch string
}

// artifact is a single pinned, checksum-verified FFmpeg build.
type artifact struct {
	URL              string
	SHA256           string
	BinPathInArchive string // path of the ffmpeg binary inside the archive
	kind             archiveKind
}

// builds maps a platform to its pinned BtbN GPL static build. These are the
// release-published per-asset SHA256s for releaseTag (GPL static, n8.1.2).
// darwin is intentionally absent: see artifactFor.
var builds = map[platformKey]artifact{
	{os: "linux", arch: "amd64"}: {
		URL:              btbnBase + "ffmpeg-n8.1.2-linux64-gpl-8.1.tar.xz",
		SHA256:           "224df3946118c44b4be06d34282609ef180f62d076bd716ab176dbca6615fd25",
		BinPathInArchive: "ffmpeg-n8.1.2-linux64-gpl-8.1/bin/ffmpeg",
		kind:             archiveTarXz,
	},
	{os: "linux", arch: "arm64"}: {
		URL:              btbnBase + "ffmpeg-n8.1.2-linuxarm64-gpl-8.1.tar.xz",
		SHA256:           "8ab8ceaa9d0b53ddd8fbfd52a7dc4038f0e95934547a3bb77b60902d5372afca",
		BinPathInArchive: "ffmpeg-n8.1.2-linuxarm64-gpl-8.1/bin/ffmpeg",
		kind:             archiveTarXz,
	},
	{os: "windows", arch: "amd64"}: {
		URL:              btbnBase + "ffmpeg-n8.1.2-win64-gpl-8.1.zip",
		SHA256:           "496128d1b102c1919e0e30dc2dc14f582f3f1bd64d5acdb1bad6beda55e055c1",
		BinPathInArchive: "ffmpeg-n8.1.2-win64-gpl-8.1/bin/ffmpeg.exe",
		kind:             archiveZip,
	},
}

// artifactFor returns the pinned build for goos/goarch, or a clear actionable
// error when none exists. macOS gets a tailored message because BtbN publishes
// no macOS builds.
func artifactFor(goos, goarch string) (artifact, error) {
	if a, ok := builds[platformKey{os: goos, arch: goarch}]; ok {
		return a, nil
	}
	if goos == "darwin" {
		return artifact{}, fmt.Errorf("ffmpeg: auto-download is unavailable on macOS; set media.ffmpeg_path or install ffmpeg (e.g. `brew install ffmpeg`)")
	}
	return artifact{}, fmt.Errorf("ffmpeg: no pinned build for %s/%s; set media.ffmpeg_path or install ffmpeg manually", goos, goarch)
}
