package ipc

import "github.com/Ostrovsky42/agent-interaction-runtime/protocol"

const snapshotJSONExpansionFactor = 8

// RecordLimit returns the bounded local IPC record budget. Public agent
// records remain governed by Limits.MaxMessageBytes; daemon/client snapshots
// may contain the entire retained Document and therefore need a larger,
// local-only envelope budget. The expansion factor covers JSON escaping and
// structural overhead while remaining finite and derived from resource limits.
func RecordLimit(limits protocol.Limits) int {
	limits = protocol.EffectiveLimits(limits)
	maxInt := int(^uint(0) >> 1)
	if limits.MaxDocumentBytes > (maxInt-limits.MaxMessageBytes)/snapshotJSONExpansionFactor {
		return maxInt
	}
	limit := limits.MaxDocumentBytes*snapshotJSONExpansionFactor + limits.MaxMessageBytes
	if limit < limits.MaxMessageBytes {
		return limits.MaxMessageBytes
	}
	return limit
}
