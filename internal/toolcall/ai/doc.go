// Package ai implements AI-maintainer tools (ainap, aistatus, wakeup)
// with the toolcall framework.
//
//   - ainap:    put the current AI session to sleep for a duration
//   - aistatus: list all projects with their AI maintainer and status
//   - wakeup:   dispatch a message to another AI maintainer at a project
//
// The tools cooperate on the ai_status table and per-project lockfiles to
// track which AI is on/nap/off for each project: wakeup starts or notifies
// the maintainer, ainap marks the session as napping, and aistatus reports
// the resulting state to other AIs.
package ai
