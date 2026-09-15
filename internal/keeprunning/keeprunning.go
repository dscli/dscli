// Package keeprunning prevents the system from entering idle screen lock or
// sleep while a long-running dscli command (chat / webchat) is active.
//
// The wake lock is best-effort: if the platform-specific mechanism is
// unavailable (headless server, unsupported desktop environment), KeepRunning
// returns a no-op DoneFunc and the caller keeps working unchanged. The user
// can still lock the screen manually at any time.
//
// What the wake lock did, and why it fell back to a no-op, is reported through
// clog at debug level. Debug records are filtered out by the default logging
// level and never reach the console, but they are still attached to the active
// trace span, so the wake lock's behavior stays inspectable after the fact.
// User-visible messages belong to internal/outfmt.
package keeprunning

import (
	"context"
	"sync"

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
// ctx carries the session's log and trace context: the wake lock records a
// "keeprunning.KeepRunning" span on it, and both acquisition and release
// report what they did through clog on that span. The span is finished only
// when the returned DoneFunc runs, so release-time records still have a span
// to land on; the returned function is guarded so the span is finished
// exactly once no matter how often it is called.
//
// Acquiring the lock is best-effort: if no supported mechanism is available,
// KeepRunning returns a no-op DoneFunc. The returned function is never nil.
func KeepRunning(ctx context.Context) DoneFunc {
	span, ctx := clog.StartSpanFromContext(ctx, "keeprunning.KeepRunning")
	done := keepRunning(ctx)

	var once sync.Once
	return func() {
		once.Do(func() {
			done()
			span.Finish()
		})
	}
}
