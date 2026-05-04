package tools

import "errors"

// errStopWalk signals filepath.WalkDir to stop early (e.g., hit max results).
var errStopWalk = errors.New("stop walk")
