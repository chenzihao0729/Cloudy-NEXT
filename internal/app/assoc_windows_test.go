package app

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// Taken out of the apps a type opened with, Cloudy leaves the others where
// they were, and their order; a tree of keys goes with all under it. On a
// key of the test's own, under the user's, deleted after.
func TestOpenWithRemovalLeavesTheOthers(t *testing.T) {
	base := fmt.Sprintf(`Software\CloudyTest-%d`, os.Getpid())
	t.Cleanup(func() { deleteKeyTree(registry.CURRENT_USER, base) })
	list := base + `\OpenWithList`
	k, _, err := registry.CreateKey(registry.CURRENT_USER, list, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		t.Skip("no registry to write to:", err)
	}
	for name, v := range map[string]string{"a": "foobar2000.exe", "b": "Cloudy.exe", "c": "vlc.exe", "MRUList": "bca"} {
		k.SetStringValue(name, v)
	}
	k.Close()
	if err := dropFromOpenWithList(list, "cloudy.exe"); err != nil {
		t.Fatal(err)
	}
	k, _ = registry.OpenKey(registry.CURRENT_USER, list, registry.QUERY_VALUE)
	if _, _, err := k.GetStringValue("b"); err == nil {
		t.Error("Cloudy still listed")
	}
	if mru, _, _ := k.GetStringValue("MRUList"); mru != "ca" {
		t.Errorf("order %q, want ca", mru)
	}
	if v, _, _ := k.GetStringValue("a"); v != "foobar2000.exe" {
		t.Errorf("another app's entry became %q", v)
	}
	k.Close()

	deep := base + `\io.example.mp3\shell\open\command`
	k, _, _ = registry.CreateKey(registry.CURRENT_USER, deep, registry.SET_VALUE)
	k.SetStringValue("", `"x.exe" "%1"`)
	k.Close()
	if err := deleteKeyTree(registry.CURRENT_USER, base+`\io.example.mp3`); err != nil {
		t.Fatal(err)
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, base+`\io.example.mp3`, registry.QUERY_VALUE); err == nil {
		k.Close()
		t.Error("the type's key is still there")
	}
}
