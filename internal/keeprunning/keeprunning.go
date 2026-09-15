// Package keeprunning prevents the system from entering idle screen lock or
// sleep while a long-running dscli command (chat / webchat) is active.
//
// The wake lock is best-effort: if the platform-specific mechanism is
// unavailable (headless server, unsupported desktop environment), KeepRunning
// returns a no-op DoneFunc and the caller keeps working unchanged. The user
// can still lock the screen manually at any time.
//
// The wake lock reports what it did (and why it fell back to a no-op) through
// clog - developer-facing logs attached to the trace, not user-facing output;
// user-visible messages belong to internal/outfmt.
package keeprunning

import (
	"context"

	"github.com/nanjj/clog"
)

// DoneFunc releases the wake lock acquired by KeepRunning. It is safe to call
// more than once and always safe to call even when KeepRunning fell back to a
// no-op.
type DoneFunc func()

// KeepRunning attempts to prevent idle screen lock/sleep for the duration of
// the calling process. It returns a DoneFunc that must be called (typically
// via defer) to release the lock when the long-running work finishes.
//
// ctx carries the session's log and trace context: the wake lock reports the
// mechanism it acquired, or the reason it fell back to a no-op, through clog
// on the active span. Acquiring the lock is best-effort: if no supported
// mechanism is available, KeepRunning returns a no-op DoneFunc. The returned
// function is never nil.
func KeepRunning(ctx context.Context) DoneFunc {
	span, ctx := clog.StartSpanFromContext(ctx, "keeprunning.KeepRunning")
	defer span.Finish()
	return keepRunning(ctx)
}
