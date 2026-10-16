package output

import "os/signal"

// Indirections so tests can observe arming and re-delivery without sending
// real signals to the test process.
var (
	notifyInterrupt = signal.Notify
	stopInterrupt   = signal.Stop
	raiseInterrupt  = raiseSignal
)
