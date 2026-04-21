// Package locale owns user-facing string translation for the client UI.
// Catalogs are embedded TOML files parsed once at first use; lookups route
// through go-i18n with English as the default fallback.
package locale

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// catalogs bundles every shipped active.*.toml message file so the client
// binary has no external runtime dependency on the translations directory.
//
//go:embed active.*.toml
var catalogs embed.FS

// catalogFiles is the canonical list of catalog filenames shipped with the
// binary. Iteration order is deterministic — lookups fall back on the
// first-registered language (English) when a requested tag is unknown.
var catalogFiles = []string{
	"active.en.toml",
	"active.ru.toml",
}

// supported is the set of BCP-47 short tags carried by catalogFiles.
// Exposed via List so callers building UI (e.g. a language picker) do not
// have to reparse the embed tree.
var supported = []string{"en", "ru"}

// defaultLang is the locale used both as the i18n bundle's default and as
// the fallback when detection fails. Kept as a constant so changing the
// baseline locale is a one-liner.
const defaultLang = "en"

// once guards one-time bundle assembly; loadErr reports whether that
// assembly succeeded. Lookups fall back to returning the messageID unchanged
// if the bundle is unusable — the catalog key surfaces in the UI as a loud
// signal that translation is broken rather than silently dropping text.
var (
	once    sync.Once
	bundle  *i18n.Bundle
	loadErr error
)

// loadBundle reads every catalog out of the embedded filesystem and
// registers it with a fresh i18n.Bundle. Called at most once per process
// via sync.Once — never in init() so the package obeys the project rule
// forbidding side-effecting package initialisers.
func loadBundle() {
	b := i18n.NewBundle(language.English)
	b.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, name := range catalogFiles {
		data, err := catalogs.ReadFile(name)
		if err != nil {
			loadErr = fmt.Errorf("read catalog %s: %w", name, err)
			return
		}
		if _, err := b.ParseMessageFileBytes(data, name); err != nil {
			loadErr = fmt.Errorf("parse catalog %s: %w", name, err)
			return
		}
	}
	bundle = b
}

// Tr localises messageID into the requested language, substituting template
// data from key/value pairs. Example:
//
//	Tr("ru", "crossing.blighted", "Region", "The Elmwolds")
//
// Unknown languages fall back to English. When the message is missing in
// every registered catalog, Tr returns messageID unchanged — a visible
// marker that the catalog is incomplete, chosen over a silent blank.
// A malformed kv list (odd length or non-string keys) is treated as no
// template data; the raw catalog string is returned.
func Tr(lang, messageID string, kv ...any) string {
	once.Do(loadBundle)
	if loadErr != nil || bundle == nil {
		return messageID
	}
	loc := i18n.NewLocalizer(bundle, lang, defaultLang)
	data := pairsToMap(kv)
	out, err := loc.Localize(&i18n.LocalizeConfig{
		MessageID:    messageID,
		TemplateData: data,
	})
	if err != nil {
		var missing *i18n.MessageNotFoundErr
		if errors.As(err, &missing) {
			return messageID
		}
		return messageID
	}
	return out
}

// Detect reads POSIX locale environment variables and returns a BCP-47
// short tag ("en", "ru"). Anything unrecognised falls back to "en". Always
// returns a tag present in List so the result is safe to send on the wire.
// Checked in order: LC_ALL, LC_MESSAGES, LANG — same precedence POSIX
// programs use so exporting LANG=ru_RU.UTF-8 Just Works.
func Detect() string {
	for _, v := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if tag := matchEnv(os.Getenv(v)); tag != "" {
			return tag
		}
	}
	return defaultLang
}

// matchEnv parses a POSIX locale string (e.g. "ru_RU.UTF-8") and returns
// a supported short tag if the primary subtag maps to one. Empty input or
// a non-matching prefix returns the empty string so callers can chain.
func matchEnv(raw string) string {
	if raw == "" {
		return ""
	}
	// Drop codeset and modifier: "ru_RU.UTF-8@mod" → "ru_RU".
	if i := strings.IndexAny(raw, ".@"); i >= 0 {
		raw = raw[:i]
	}
	// Use the primary subtag only: "ru_RU" → "ru".
	if i := strings.IndexAny(raw, "_-"); i >= 0 {
		raw = raw[:i]
	}
	raw = strings.ToLower(raw)
	for _, tag := range supported {
		if raw == tag {
			return tag
		}
	}
	return ""
}

// List returns the supported language tags. The returned slice is a copy —
// callers cannot mutate package state by appending to it.
func List() []string {
	out := make([]string, len(supported))
	copy(out, supported)
	return out
}

// Default returns the fallback locale tag used by Tr when a specific
// language is missing a key. Exposed so call sites can be explicit when
// they want English rather than environment-derived detection.
func Default() string { return defaultLang }

// pairsToMap converts a flat key/value argument list into the string-keyed
// map expected by text/template. Odd-length or non-string keys yield nil
// so Tr fails safely instead of panicking on malformed call sites.
func pairsToMap(kv []any) map[string]string {
	if len(kv) == 0 {
		return nil
	}
	if len(kv)%2 != 0 {
		return nil
	}
	out := make(map[string]string, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok {
			return nil
		}
		out[key] = fmt.Sprint(kv[i+1])
	}
	return out
}
