package app

// sceneFrom notes, for the motion scene, that the gear opens the pane key,
// as a click on it does.
func sceneFrom(a *app, key string) { a.growFromRect(key, a.mk.gear, 14) }
