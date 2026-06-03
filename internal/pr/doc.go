// Package pr implements ADR-071's TTL-bounded refresh of cached
// pull-request state for af workstreams.
//
// af stores PR metadata in state.toml.[pr] (number, url, state, plus
// the ADR-071-added last_refreshed_at and last_refresh_error). The
// package's Refresh function reconciles that cache with GitHub via
// gh pr view, honouring the configured refresh_ttl and a 5-second
// per-fetch timeout.
//
// Refresh detects state flips (e.g. "open" → "merged") and reports
// them so callers can emit the ADR-071 pr_state_changed ledger event
// alongside the existing pr_opened / pr_merged / pr_closed events
// derived from the flip.
package pr
