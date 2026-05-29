// Package build provides the version information for the application.
package build

import (
	"regexp"
)

var version string
var buildstamp string
var githash string
var officialBuild string

func Version() (string, string, string) {
	return version, githash, buildstamp
}

func VersionString() string {
	var versionString string
	switch {
	case version != "":
		if githash != "" && !IsDevelop() {
			versionString = version + " (" + githash + ")"
		} else {
			versionString = version
		}
	case githash != "":
		versionString = githash
	default:
		versionString = "unknown"
	}
	if IsOfficial() {
		versionString += " - Official Build"
	} else {
		versionString += " - Unofficial Build"
	}
	if buildstamp != "" {
		versionString += " - " + buildstamp
	}
	return versionString
}

func IsOfficial() bool {
	return officialBuild == "true"
}

// Edition identifies this build's variant. Original upstream Stash has no
// equivalent; the NG fork stamps "ng" so clients can detect fork-only
// capabilities via the serverCapabilities GraphQL query.
const Edition = "ng"

// NGAPIVersion is bumped whenever NG's client-facing API surface changes,
// independently of the SQLite appSchema migration counter (which upstream
// also increments and therefore cannot be used to distinguish the fork).
const NGAPIVersion = 1

// NGFeatures lists the fork-only capabilities clients may rely on. Keep in
// sync with the NG additions actually wired into the schema/resolvers.
func NGFeatures() []string {
	return []string{"deletedSince", "moveFolder", "folderCounts", "webhooks", "similarScenes"}
}

func IsDevelop() bool {
	if githash == "" {
		return false
	}

	// if the version is suffixed with -x-xxxx, then we are running a development build
	develop := false
	re := regexp.MustCompile(`-\d+-g\w+$`)
	if re.MatchString(version) {
		develop = true
	}
	return develop
}
