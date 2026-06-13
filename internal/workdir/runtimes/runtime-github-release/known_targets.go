package runtimegh

// GOOS constants mirror runtime.GOOS values used throughout this package.
const (
	goosDarwin  = "darwin"
	goosLinux   = "linux"
	goosWindows = "windows"
	goosFreeBSD = "freebsd"
)

// GOARCH constants mirror runtime.GOARCH values used throughout this package.
const (
	goarchAMD64 = "amd64"
	goarchARM64 = "arm64"
	goarch386   = "386"
	goarchARM   = "arm"
)

// knownPlatforms is the set of OS/arch combinations checked when computing a pin.
var knownPlatforms = [][2]string{
	{goosDarwin, goarchARM64},
	{goosDarwin, goarchAMD64},
	{goosLinux, goarchAMD64},
	{goosLinux, goarchARM64},
	{goosLinux, goarch386},
	{goosWindows, goarchAMD64},
	{goosWindows, goarchARM64},
}

// knownOSNames maps a GOOS value to the release-filename variants used by that OS.
// freebsd is included for install (autoDiscoverAsset) but is not in knownPlatforms
// because we do not generate supply-chain pins for it.
var knownOSNames = map[string][]string{
	goosDarwin:  {"darwin", "macOS", "macos", "osx", "Darwin"},
	goosLinux:   {"linux", "Linux"},
	goosWindows: {"windows", "Windows"},
	goosFreeBSD: {"freebsd", "FreeBSD"},
}

// knownArchNames maps a GOARCH value to the release-filename variants used by that arch.
var knownArchNames = map[string][]string{
	goarchAMD64: {"amd64", "x86_64", "x64", "64bit"},
	goarchARM64: {"arm64", "aarch64", "ARM64"},
	goarch386:   {"386", "x86", "i386", "32bit"},
	goarchARM:   {"armv6", "armv7", "arm", "ARM"},
}
