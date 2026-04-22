// Package locale. This file lists all message IDs used by the client as
// typed constants. Use these in place of string literals when calling Tr
// — the compiler will catch typos and rename-safety is preserved.
//
// Keep this list in sync with active.en.toml. TestKeysPresentInCatalog
// (in bundle_test.go) fails if a Key* value is missing from the catalog.
//
// CI enforcement: scripts/check-locale-keys.sh scans internal/ for
// locale.Tr calls that still pass a string literal and fails the build
// if any remain.
package locale

import "strconv"

// Crossing message keys — one per RegionCharacter variant.
const (
	KeyCrossingNormal   = "crossing.normal"
	KeyCrossingBlighted = "crossing.blighted"
	KeyCrossingFey      = "crossing.fey"
	KeyCrossingAncient  = "crossing.ancient"
	KeyCrossingSavage   = "crossing.savage"
	KeyCrossingHoly     = "crossing.holy"
	KeyCrossingWild     = "crossing.wild"
)

// KeyRegionHeaderFormat is the catalog key for the region header line format.
const KeyRegionHeaderFormat = "region_header.format"

// Status bar message keys.
const (
	KeyStatusYou                   = "status.you"
	KeyStatusServer                = "status.server"
	KeyStatusKeybindings           = "status.keybindings"
	KeyStatusConnecting            = "status.connecting"
	KeyStatusConnectedJoining      = "status.connected_joining"
	KeyStatusDisconnected          = "status.disconnected"
	KeyStatusDisconnectedWithError = "status.disconnected_with_error"
)

// Error message keys.
const (
	KeyErrorConnectionLost = "error.connection_lost"
	KeyErrorServerClosed   = "error.server_closed"
	KeyErrorUnknown        = "error.unknown"
	KeyErrorOutboxFull     = "error.outbox_full"
	KeyErrorDial           = "error.dial"
	KeyErrorOpenPlayStream = "error.open_play_stream"
	KeyErrorStreamRecv     = "error.stream_recv"

	// Wire-protocol error codes — must stay in sync with pb.ErrCode* constants
	// in internal/proto/errorcodes.go. TestServerErrorCodesHaveClientKeys
	// enforces this at compile time.
	KeyErrorInvalidArgument = "error.invalid_argument"
	KeyErrorInvalidProtocol = "error.invalid_protocol"
	KeyErrorRuleViolation   = "error.rule_violation"
)

// Input widget keys.
const (
	KeyInputNameLabel = "input.name_label"
	KeyInputPrompt    = "input.prompt"
)

// Hint keys.
const (
	KeyHintQuitShort  = "hint.quit_short"
	KeyHintQuitLong   = "hint.quit_long"
	KeyHintDisconnect = "hint.disconnect"
)

// Panel label keys.
const (
	KeyPanelPlayersHeader = "panel.players_header"
	KeyPanelEventsHeader  = "panel.events_header"
	KeyPanelEmptyLog      = "panel.empty_log"
	KeyPanelEmptyMap      = "panel.empty_map"
	KeyPanelEmptyList     = "panel.empty_list"

	// KeyPanelStatsHeader is the header for the right-side stats panel that
	// shows character parameters (HP, Agility, Intellect, etc.). Replaces the
	// old players list header once the stats model is wired up.
	KeyPanelStatsHeader = "panel.stats_header"

	// KeyStatsEmpty is shown inside the stats panel when no stat data is
	// available yet. Future: replace with HP/Agility/Intellect list rendering.
	KeyStatsEmpty = "stats.empty"
)

// Title screen keys.
const (
	KeyTitleText    = "title.text"
	KeyTitleTagline = "title.tagline"
)

// Event log message keys.
const (
	KeyLogJoined = "log.joined"
	KeyLogLeft   = "log.left"
	KeyLogMoved  = "log.moved"
)

// Landmark label keys — one per LandmarkKind variant (excluding NONE).
// Reserved for Phase 3 approach-detection UI; not rendered anywhere yet.
const (
	KeyLandmarkTower          = "landmark.tower"
	KeyLandmarkGiantTree      = "landmark.giant_tree"
	KeyLandmarkStandingStones = "landmark.standing_stones"
	KeyLandmarkObelisk        = "landmark.obelisk"
	KeyLandmarkChasm          = "landmark.chasm"
	KeyLandmarkShrine         = "landmark.shrine"
)

// Character label keys — one per RegionCharacter variant.
const (
	KeyCharacterNormal   = "character.normal"
	KeyCharacterBlighted = "character.blighted"
	KeyCharacterFey      = "character.fey"
	KeyCharacterAncient  = "character.ancient"
	KeyCharacterSavage   = "character.savage"
	KeyCharacterHoly     = "character.holy"
	KeyCharacterWild     = "character.wild"
)

// Geo term keys — mirror geoTermsByBiomeFamily in
// internal/game/worldgen/region_names.go one-for-one.
const (
	KeyGeoForest0 = "geo.forest.0"
	KeyGeoForest1 = "geo.forest.1"
	KeyGeoForest2 = "geo.forest.2"
	KeyGeoForest3 = "geo.forest.3"
	KeyGeoForest4 = "geo.forest.4"

	KeyGeoPlain0 = "geo.plain.0"
	KeyGeoPlain1 = "geo.plain.1"
	KeyGeoPlain2 = "geo.plain.2"
	KeyGeoPlain3 = "geo.plain.3"
	KeyGeoPlain4 = "geo.plain.4"

	KeyGeoMountain0 = "geo.mountain.0"
	KeyGeoMountain1 = "geo.mountain.1"
	KeyGeoMountain2 = "geo.mountain.2"
	KeyGeoMountain3 = "geo.mountain.3"

	KeyGeoWater0 = "geo.water.0"
	KeyGeoWater1 = "geo.water.1"
	KeyGeoWater2 = "geo.water.2"
	KeyGeoWater3 = "geo.water.3"

	KeyGeoDesert0 = "geo.desert.0"
	KeyGeoDesert1 = "geo.desert.1"
	KeyGeoDesert2 = "geo.desert.2"
	KeyGeoDesert3 = "geo.desert.3"

	KeyGeoTundra0 = "geo.tundra.0"
	KeyGeoTundra1 = "geo.tundra.1"
	KeyGeoTundra2 = "geo.tundra.2"

	KeyGeoUnknown0 = "geo.unknown.0"
)

// AllKeys returns every declared Key* constant, for testing coverage.
// The returned slice is a fresh copy — callers cannot mutate package state.
func AllKeys() []string {
	return []string{
		KeyCrossingNormal,
		KeyCrossingBlighted,
		KeyCrossingFey,
		KeyCrossingAncient,
		KeyCrossingSavage,
		KeyCrossingHoly,
		KeyCrossingWild,

		KeyRegionHeaderFormat,

		KeyStatusYou,
		KeyStatusServer,
		KeyStatusKeybindings,
		KeyStatusConnecting,
		KeyStatusConnectedJoining,
		KeyStatusDisconnected,
		KeyStatusDisconnectedWithError,

		KeyErrorConnectionLost,
		KeyErrorServerClosed,
		KeyErrorUnknown,
		KeyErrorOutboxFull,
		KeyErrorDial,
		KeyErrorOpenPlayStream,
		KeyErrorStreamRecv,
		KeyErrorInvalidArgument,
		KeyErrorInvalidProtocol,
		KeyErrorRuleViolation,

		KeyInputNameLabel,
		KeyInputPrompt,

		KeyHintQuitShort,
		KeyHintQuitLong,
		KeyHintDisconnect,

		KeyPanelPlayersHeader,
		KeyPanelEventsHeader,
		KeyPanelEmptyLog,
		KeyPanelEmptyMap,
		KeyPanelEmptyList,
		KeyPanelStatsHeader,
		KeyStatsEmpty,

		KeyTitleText,
		KeyTitleTagline,

		KeyLogJoined,
		KeyLogLeft,
		KeyLogMoved,

		KeyLandmarkTower,
		KeyLandmarkGiantTree,
		KeyLandmarkStandingStones,
		KeyLandmarkObelisk,
		KeyLandmarkChasm,
		KeyLandmarkShrine,

		KeyCharacterNormal,
		KeyCharacterBlighted,
		KeyCharacterFey,
		KeyCharacterAncient,
		KeyCharacterSavage,
		KeyCharacterHoly,
		KeyCharacterWild,

		KeyGeoForest0,
		KeyGeoForest1,
		KeyGeoForest2,
		KeyGeoForest3,
		KeyGeoForest4,

		KeyGeoPlain0,
		KeyGeoPlain1,
		KeyGeoPlain2,
		KeyGeoPlain3,
		KeyGeoPlain4,

		KeyGeoMountain0,
		KeyGeoMountain1,
		KeyGeoMountain2,
		KeyGeoMountain3,

		KeyGeoWater0,
		KeyGeoWater1,
		KeyGeoWater2,
		KeyGeoWater3,

		KeyGeoDesert0,
		KeyGeoDesert1,
		KeyGeoDesert2,
		KeyGeoDesert3,

		KeyGeoTundra0,
		KeyGeoTundra1,
		KeyGeoTundra2,

		KeyGeoUnknown0,

		KeyRegionNameCharacterPrefix,
		KeyLandmarkNameCharacterPrefix,
		KeySettlementNameCharacterPrefix,
	}
}

// CharacterCrossingKey returns the crossing-message key for a given
// RegionCharacter suffix. The suffix is already lowercase ("blighted", etc.)
// as produced by regionCharacterKey in mapper.go.
func CharacterCrossingKey(characterName string) string {
	return "crossing." + characterName
}

// CharacterLabelKey returns the character-label key for a given character name.
func CharacterLabelKey(characterName string) string {
	return "character." + characterName
}

// GeoKey returns the geo-term key for a biome family and zero-based index.
// family is a lowercase biome name such as "forest" or "plain".
func GeoKey(family string, index int) string {
	return "geo." + family + "." + strconv.Itoa(index)
}

// KeyRegionNameCharacterPrefix is the catalog key for the
// FormatCharacterPrefix template used when composing a region display name
// from a naming.Parts record whose Format is FormatCharacterPrefix.
const KeyRegionNameCharacterPrefix = "region.name.character_prefix"

// KeyLandmarkNameCharacterPrefix is the catalog key for the
// FormatCharacterPrefix template used when composing a landmark display
// name from a naming.Parts record whose Format is FormatCharacterPrefix.
const KeyLandmarkNameCharacterPrefix = "landmark.name.character_prefix"

// KeySettlementNameCharacterPrefix is the catalog key for the
// FormatCharacterPrefix template used when composing a settlement
// display name from a naming.Parts record whose Format is
// FormatCharacterPrefix.
const KeySettlementNameCharacterPrefix = "settlement.name.character_prefix"

// RegionNamePatternKey returns the kind-pattern template key for a region
// sub-kind (biome family, e.g. "forest") and zero-based PatternIndex.
// The key shape is "region.name.<sub_kind>.kind_pattern.<index>".
func RegionNamePatternKey(subKind string, idx uint8) string {
	return "region.name." + subKind + ".kind_pattern." + strconv.Itoa(int(idx))
}

// RegionPrefixKey returns the character-prefix catalog key for a region.
// character is the RegionCharacter.Key() value (e.g. "blighted"). The key
// shape is "region.prefix.<character>.<index>".
func RegionPrefixKey(character string, idx uint8) string {
	return "region.prefix." + character + "." + strconv.Itoa(int(idx))
}

// LandmarkNamePatternKey returns the kind-pattern template key for a
// landmark sub-kind (LandmarkKind.Key(), e.g. "tower") and zero-based
// PatternIndex. Shape: "landmark.name.<sub_kind>.kind_pattern.<index>".
func LandmarkNamePatternKey(subKind string, idx uint8) string {
	return "landmark.name." + subKind + ".kind_pattern." + strconv.Itoa(int(idx))
}

// LandmarkPrefixKey returns the character-prefix catalog key for a
// landmark. Shape: "landmark.prefix.<character>.<index>".
func LandmarkPrefixKey(character string, idx uint8) string {
	return "landmark.prefix." + character + "." + strconv.Itoa(int(idx))
}

// LandmarkApproachKey returns the approach-message catalog key for a
// landmark kind (LandmarkKind.Key(), e.g. "tower"). Shape:
// "landmark.approach.<kind_key>".
func LandmarkApproachKey(kindKey string) string {
	return "landmark.approach." + kindKey
}

// SettlementNamePatternKey returns the kind-pattern template key for a
// settlement sub-kind and zero-based PatternIndex. The sub-kind is the
// "<culture>.<kind>" pair (e.g. "drevan.village"). Shape:
// "settlement.name.<culture>.<kind>.kind_pattern.<index>".
func SettlementNamePatternKey(subKind string, idx uint8) string {
	return "settlement.name." + subKind + ".kind_pattern." + strconv.Itoa(int(idx))
}

// SettlementPrefixKey returns the character-prefix catalog key for a
// settlement. Shape: "settlement.prefix.<character>.<index>".
func SettlementPrefixKey(character string, idx uint8) string {
	return "settlement.prefix." + character + "." + strconv.Itoa(int(idx))
}
