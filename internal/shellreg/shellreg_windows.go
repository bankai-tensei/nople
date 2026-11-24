package shellreg

import (
	"errors"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// The keys under HKEY_CURRENT_USER that Register writes. They hold nothing
// but Nople's, so Unregister deletes them whole.
const (
	classes  = `Software\Classes\`
	appKey   = classes + `Applications\nople.exe`
	capsKey  = `Software\Nople\Capabilities`
	ownerKey = `Software\Nople`
	regApps  = `Software\RegisteredApplications`
	appName  = "Nople"
	verbName = "Nople"
	// progPrefix begins the ProgID of each extension, as Nople.go.
	progPrefix = "Nople."
	// legacyProgID is the one ProgID of all the extensions that Nople
	// registered before each got its own; Register takes it out.
	legacyProgID = "Nople.File"
)

// dropped are the extensions that an earlier version offered to open and
// this one does not; Register and Unregister take Nople out of them.
var dropped = []string{".bat", ".cmd"}

// menus are the context menus that get "Open with Nople", and what of the
// clicked thing the command gets: %V is the folder, also when the menu is
// that of the inside of a folder, and %1 the file.
var menus = []struct{ key, arg string }{
	{classes + `Directory\shell\` + verbName, "%V"},
	{classes + `Directory\Background\shell\` + verbName, "%V"},
	{classes + `Drive\shell\` + verbName, "%V"},
	{classes + `*\shell\` + verbName, "%1"},
}

// progIDOf is the ProgID of the extension ext, as Nople.go for ".go".
func progIDOf(ext string) string { return progPrefix + strings.TrimPrefix(ext, ".") }

// Register puts Nople at exe into the shell for the current user, in place
// of what an earlier Register put there. Each extension gets a ProgID of
// its own, whose icon is IconFile(iconDir, ext), so that a file Nople opens
// keeps an icon of its type and does not take Nople's.
func Register(exe string, l Labels, iconDir string) error {
	icon := `"` + exe + `",0`
	open := func(arg string) string { return `"` + exe + `" "` + arg + `"` }
	cleanLegacy()

	for _, m := range menus {
		if err := setValues(m.key, map[string]string{"": l.Open, "Icon": icon}); err != nil {
			return err
		}
		if err := setValues(m.key+`\command`, map[string]string{"": open(m.arg)}); err != nil {
			return err
		}
	}

	// nople.exe itself, for "Open with" → "Choose another app". A file
	// opened so shows the page of no kind, not Nople's icon.
	if err := setValues(appKey, map[string]string{"FriendlyAppName": appName}); err != nil {
		return err
	}
	if err := setValues(appKey+`\DefaultIcon`, map[string]string{"": IconFile(iconDir, "")}); err != nil {
		return err
	}
	if err := setValues(appKey+`\shell\open\command`, map[string]string{"": open("%1")}); err != nil {
		return err
	}

	// What Default apps shows of Nople.
	if err := setValues(capsKey, map[string]string{"ApplicationName": appName, "ApplicationDescription": l.Description, "ApplicationIcon": icon}); err != nil {
		return err
	}
	types := map[string]string{}
	assocs := map[string]string{}
	for _, ext := range Extensions {
		id := progIDOf(ext)
		name := l.TypeName(ext)
		if err := setValues(classes+id, map[string]string{"": name, "FriendlyTypeName": name}); err != nil {
			return err
		}
		if err := setValues(classes+id+`\DefaultIcon`, map[string]string{"": IconFile(iconDir, ext)}); err != nil {
			return err
		}
		if err := setValues(classes+id+`\shell\open\command`, map[string]string{"": open("%1")}); err != nil {
			return err
		}
		if err := setValues(classes+ext+`\OpenWithProgids`, map[string]string{id: ""}); err != nil {
			return err
		}
		types[ext] = ""
		assocs[ext] = id
	}
	if err := setValues(appKey+`\SupportedTypes`, types); err != nil {
		return err
	}
	if err := setValues(capsKey+`\FileAssociations`, assocs); err != nil {
		return err
	}
	if err := setValues(regApps, map[string]string{appName: capsKey}); err != nil {
		return err
	}
	assocChanged()
	return nil
}

// IconFile is the .ico of the extension ext in the folder dir, as go.ico
// for ".go"; "" is file.ico, the icon of a file of no kind.
func IconFile(dir, ext string) string {
	name := strings.TrimPrefix(ext, ".")
	if name == "" {
		name = "file"
	}
	return filepath.Join(dir, name+".ico")
}

// cleanLegacy takes out what earlier versions registered and this one does
// not: the one ProgID of all the extensions, and Nople from the extensions
// it dropped.
func cleanLegacy() {
	// What is left of an earlier version mostly does no harm, so a key that
	// will not go is left.
	_ = deleteTree(classes + legacyProgID) //nolint:errcheck // see above
	for _, ext := range Extensions {
		_ = deleteValue(classes+ext+`\OpenWithProgids`, legacyProgID) //nolint:errcheck // see above
	}
	for _, ext := range dropped {
		id := progIDOf(ext)
		_ = deleteTree(classes + id)                      //nolint:errcheck // see above
		_ = deleteValue(appKey+`\SupportedTypes`, ext)    //nolint:errcheck // see above
		_ = deleteValue(capsKey+`\FileAssociations`, ext) //nolint:errcheck // see above
		for _, v := range []string{id, legacyProgID} {
			_ = deleteValue(classes+ext+`\OpenWithProgids`, v) //nolint:errcheck // see above
		}
		deleteIfEmpty(classes + ext + `\OpenWithProgids`)
		deleteIfEmpty(classes + ext)
	}
}

// Unregister takes out of the shell what Register put there: every ProgID
// that begins with Nople., also of an extension a later version dropped.
// The keys of the extensions stay, as other programs share them; only
// Nople's values leave them.
func Unregister() error {
	var errs []error
	for _, m := range menus {
		errs = append(errs, deleteTree(m.key))
	}
	errs = append(errs, deleteTree(appKey), deleteTree(ownerKey))
	ids, err := nopleProgIDs()
	errs = append(errs, err)
	for _, id := range ids {
		errs = append(errs, deleteTree(classes+id))
	}
	for _, ext := range append(append([]string{}, Extensions...), dropped...) {
		for _, id := range []string{progIDOf(ext), legacyProgID} {
			errs = append(errs, deleteValue(classes+ext+`\OpenWithProgids`, id))
		}
		// Register may have made the keys; empty, they go.
		deleteIfEmpty(classes + ext + `\OpenWithProgids`)
		deleteIfEmpty(classes + ext)
	}
	errs = append(errs, deleteValue(regApps, appName))
	assocChanged()
	return errors.Join(errs...)
}

// nopleProgIDs are the keys of the classes of the current user that begin
// with Nople.
func nopleProgIDs() ([]string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, strings.TrimSuffix(classes, `\`), registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, err
	}
	defer k.Close() //nolint:errcheck // only read
	names, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, n := range names {
		if len(n) > len(progPrefix) && strings.EqualFold(n[:len(progPrefix)], progPrefix) {
			ids = append(ids, n)
		}
	}
	return ids, nil
}

// setValues sets the string values of the key path, making the key if it is
// not there. The name "" is the default value of the key.
func setValues(path string, values map[string]string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close() //nolint:errcheck // the values are set by then; closing the handle writes nothing
	for name, v := range values {
		if err := k.SetStringValue(name, v); err != nil {
			return err
		}
	}
	return nil
}

// deleteTree deletes the key path and the keys under it; a key that is not
// there is no error.
func deleteTree(path string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	subs, err := k.ReadSubKeyNames(-1)
	_ = k.Close() //nolint:errcheck // only read
	if err != nil {
		return err
	}
	for _, s := range subs {
		if err := deleteTree(path + `\` + s); err != nil {
			return err
		}
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// deleteValue deletes the value name of the key path; a key or value that
// is not there is no error.
func deleteValue(path, name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close() //nolint:errcheck // the value is deleted by then; closing the handle writes nothing
	if err := k.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// deleteIfEmpty deletes the key path if it holds no values and no keys.
func deleteIfEmpty(path string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	info, err := k.Stat()
	_ = k.Close() //nolint:errcheck // only read
	if err == nil && info.SubKeyCount == 0 && info.ValueCount == 0 {
		// An empty key that stays does no harm.
		_ = registry.DeleteKey(registry.CURRENT_USER, path) //nolint:errcheck // see above
	}
}

// assocChanged tells Explorer that the associations changed, so that it
// shows them without a restart.
func assocChanged() {
	const shcneAssocChanged, shcnfIDList = 0x08000000, 0
	// SHChangeNotify returns nothing; without it Explorer shows the change
	// after a restart.
	_, _, _ = syscall.NewLazyDLL("shell32.dll").NewProc("SHChangeNotify").Call(shcneAssocChanged, shcnfIDList, 0, 0) //nolint:errcheck // see above
}
