package app

import "os"

// xParts is which parts of the motion scene to go through (NM_PARTS), all
// of them when empty.
var xParts = os.Getenv("NM_PARTS")
