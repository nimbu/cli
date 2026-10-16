//go:build windows

package output

import "os"

// Windows has no way to re-raise a console control event to ourselves, so the
// progress line is not guarded there: arming would swallow Ctrl-C.
var interruptSignals []os.Signal

func raiseSignal(os.Signal) {}
