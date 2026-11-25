package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// songTypes are the files Cloudy opens, as mygo.json's fileAssociations
// list them.
var songTypes = []struct{ ext, name string }{{"mp3", "MP3 音频"}, {"flac", "FLAC 音频"}}

// appProgID is the start of the names of Cloudy's file types: what its
// installer registers, so that the two are one, of the identifier's letters,
// digits and dots.
func appProgID() string {
	return strings.Map(func(r rune) rune {
		if r < 0x80 && (r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return r
		}
		return -1
	}, appIdentifier)
}

// canRegisterOpenWith reports whether Cloudy can list itself to open songs.
const canRegisterOpenWith = true

// registerOpenWith lists Cloudy, as it runs from where it is, among the apps
// that open MP3 and FLAC files, for the listener: what the installer does,
// for a copy unpacked by hand. Choosing it as the default stays the
// listener's, in the system's settings.
func registerOpenWith() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	set := func(path, name, value string) error {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
		if err != nil {
			return err
		}
		defer k.Close()
		return k.SetStringValue(name, value)
	}
	for _, t := range songTypes {
		prog := appProgID() + "." + t.ext
		class := `Software\Classes\` + prog
		for _, v := range [][3]string{
			{class, "", t.name},
			{class + `\DefaultIcon`, "", exe + ",0"},
			{class + `\shell\open\command`, "", `"` + exe + `" "%1"`},
			{`Software\Classes\.` + t.ext + `\OpenWithProgids`, prog, ""},
		} {
			if err := set(v[0], v[1], v[2]); err != nil {
				return err
			}
		}
	}
	// Tell Explorer the associations changed.
	windows.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify").Call(0x08000000, 0, 0, 0)
	return nil
}

// openWithRegistered reports whether Cloudy is among the apps that open
// MP3 or FLAC files: listed by itself, by its installer, or by Windows as
// one chosen with 「选择其他应用」.
func openWithRegistered() bool {
	exists := func(path string) bool {
		k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
		if err != nil {
			return false
		}
		k.Close()
		return true
	}
	for _, t := range songTypes {
		prog := appProgID() + "." + t.ext
		if exists(`Software\Classes\` + prog) {
			return true
		}
		if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\.`+t.ext+`\OpenWithProgids`, registry.QUERY_VALUE); err == nil {
			_, _, err := k.GetValue(prog, nil)
			k.Close()
			if err == nil {
				return true
			}
		}
	}
	if exe, err := os.Executable(); err == nil && exists(`Software\Classes\Applications\`+filepath.Base(exe)) {
		return true
	}
	return false
}

// unregisterOpenWith takes Cloudy out of the apps that open MP3 and FLAC
// files, for the listener, undoing registerOpenWith and what its installer
// and Windows' own 「选择其他应用」 added: its file types, its place in each
// extension's list, and Windows' note of the app. Being the default stays
// the system's to change: Windows lets no app change that.
func unregisterOpenWith() error {
	var first error
	keep := func(err error) {
		if err != nil && !errors.Is(err, registry.ErrNotExist) && first == nil {
			first = err
		}
	}
	exe, _ := os.Executable()
	name := filepath.Base(exe)
	for _, t := range songTypes {
		prog := appProgID() + "." + t.ext
		keep(deleteKeyTree(registry.CURRENT_USER, `Software\Classes\`+prog))
		for _, list := range []string{
			`Software\Classes\.` + t.ext + `\OpenWithProgids`,
			`Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.` + t.ext + `\OpenWithProgids`,
		} {
			if k, err := registry.OpenKey(registry.CURRENT_USER, list, registry.SET_VALUE); err == nil {
				keep(k.DeleteValue(prog))
				k.Close()
			}
		}
		if name != "" {
			keep(dropFromOpenWithList(`Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.`+t.ext+`\OpenWithList`, name))
		}
	}
	if name != "" {
		keep(deleteKeyTree(registry.CURRENT_USER, `Software\Classes\Applications\`+name))
	}
	// Tell Explorer the associations changed.
	windows.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify").Call(0x08000000, 0, 0, 0)
	return first
}

// deleteKeyTree deletes the key at path under root and all under it.
func deleteKeyTree(root registry.Key, path string) error {
	k, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	subs, err := k.ReadSubKeyNames(-1)
	k.Close()
	if err != nil {
		return err
	}
	for _, s := range subs {
		if err := deleteKeyTree(root, path+`\`+s); err != nil {
			return err
		}
	}
	return registry.DeleteKey(root, path)
}

// dropFromOpenWithList takes the app named name (its .exe's name) out of
// the list Windows keeps of the apps a type was opened with: its letter,
// in the values and in the order, MRUList, that names them.
func dropFromOpenWithList(path, name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	letters, err := k.ReadValueNames(-1)
	if err != nil {
		return err
	}
	mru, _, _ := k.GetStringValue("MRUList")
	for _, l := range letters {
		if len(l) != 1 {
			continue
		}
		if v, _, err := k.GetStringValue(l); err == nil && strings.EqualFold(v, name) {
			if err := k.DeleteValue(l); err != nil {
				return err
			}
			mru = strings.ReplaceAll(mru, l, "")
		}
	}
	if mru != "" {
		return k.SetStringValue("MRUList", mru)
	}
	return nil
}
