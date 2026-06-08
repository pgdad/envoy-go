package kafkabroker

// apiKeyEntry is one row of the static Kafka 3.9.1 api-key table (SPEC Appendix
// B + the D-PLAN-1 maxVersion). `root` is the snake-case message name (the stat
// segment is request.<root>_request / response.<root>_response). `maxVersion` is
// the highest api_version Kafka 3.9.1 defines; a request whose api_version
// exceeds it routes to *.unknown (D-PLAN-1). api_keys 71/72 (telemetry) EXCLUDED.
type apiKeyEntry struct {
	key        int16
	root       string
	maxVersion int16
}

// apiKeyTable is in api_key order with 71/72 omitted (86 entries). Transcribed
// from SPEC Appendix B; pinned by TestApiKeyRoster_MatchesUpstream.
//
// The maxVersion column holds the Kafka 3.9.1 per-key `validVersions` ceilings
// (D-PLAN-1; sourced from the Kafka 3.9.1 message-JSON `validVersions` upper
// bound per ApiKey). These exact values are NOT differentially load-bearing: the
// only version-sensitive fixture arm (0053 unknown-version) uses
// api_version=0x7FFF, far above any real ceiling. The hard contract is only the
// spot-check minimums asserted in TestApiKeyMaxVersion (produce>=9, fetch>=13,
// metadata>=9, api_versions>=3) and that every value is a small non-negative
// int16 well under 32767. The newest share-group keys (73+) ceil at 0; the KRaft keys (52+) stay low (<=4, e.g. broker_registration(62)=4).
var apiKeyTable = []apiKeyEntry{
	{0, "produce", 11},
	{1, "fetch", 17},
	{2, "list_offsets", 9},
	{3, "metadata", 12},
	{4, "leader_and_isr", 7},
	{5, "stop_replica", 4},
	{6, "update_metadata", 8},
	{7, "controlled_shutdown", 3},
	{8, "offset_commit", 9},
	{9, "offset_fetch", 9},
	{10, "find_coordinator", 6},
	{11, "join_group", 9},
	{12, "heartbeat", 4},
	{13, "leave_group", 5},
	{14, "sync_group", 5},
	{15, "describe_groups", 6},
	{16, "list_groups", 5},
	{17, "sasl_handshake", 1},
	{18, "api_versions", 4},
	{19, "create_topics", 7},
	{20, "delete_topics", 6},
	{21, "delete_records", 2},
	{22, "init_producer_id", 5},
	{23, "offset_for_leader_epoch", 4},
	{24, "add_partitions_to_txn", 5},
	{25, "add_offsets_to_txn", 4},
	{26, "end_txn", 4},
	{27, "write_txn_markers", 1},
	{28, "txn_offset_commit", 4},
	{29, "describe_acls", 3},
	{30, "create_acls", 3},
	{31, "delete_acls", 3},
	{32, "describe_configs", 4},
	{33, "alter_configs", 2},
	{34, "alter_replica_log_dirs", 2},
	{35, "describe_log_dirs", 4},
	{36, "sasl_authenticate", 2},
	{37, "create_partitions", 3},
	{38, "create_delegation_token", 3},
	{39, "renew_delegation_token", 2},
	{40, "expire_delegation_token", 2},
	{41, "describe_delegation_token", 3},
	{42, "delete_groups", 2},
	{43, "elect_leaders", 2},
	{44, "incremental_alter_configs", 1},
	{45, "alter_partition_reassignments", 0},
	{46, "list_partition_reassignments", 0},
	{47, "offset_delete", 0},
	{48, "describe_client_quotas", 1},
	{49, "alter_client_quotas", 1},
	{50, "describe_user_scram_credentials", 0},
	{51, "alter_user_scram_credentials", 0},
	{52, "vote", 1},
	{53, "begin_quorum_epoch", 1},
	{54, "end_quorum_epoch", 1},
	{55, "describe_quorum", 2},
	{56, "alter_partition", 3},
	{57, "update_features", 1},
	{58, "envelope", 0},
	{59, "fetch_snapshot", 1},
	{60, "describe_cluster", 1},
	{61, "describe_producers", 0},
	{62, "broker_registration", 4},
	{63, "broker_heartbeat", 1},
	{64, "unregister_broker", 0},
	{65, "describe_transactions", 0},
	{66, "list_transactions", 1},
	{67, "allocate_producer_ids", 0},
	{68, "consumer_group_heartbeat", 0},
	{69, "consumer_group_describe", 0},
	{70, "controller_registration", 0},
	// api_keys 71 (get_telemetry_subscriptions) and 72 (push_telemetry) are
	// EXCLUDED from the telemetry roster (SPEC Appendix B); the table jumps 70->73.
	{73, "assign_replicas_to_dirs", 0},
	{74, "list_client_metrics_resources", 0},
	{75, "describe_topic_partitions", 0},
	{76, "share_group_heartbeat", 0},
	{77, "share_group_describe", 0},
	{78, "share_fetch", 0},
	{79, "share_acknowledge", 0},
	{80, "add_raft_voter", 0},
	{81, "remove_raft_voter", 0},
	{82, "update_raft_voter", 0},
	{83, "initialize_share_group_state", 0},
	{84, "read_share_group_state", 0},
	{85, "write_share_group_state", 0},
	{86, "delete_share_group_state", 0},
	{87, "read_share_group_state_summary", 0},
}

var apiKeyByKey = func() map[int16]apiKeyEntry {
	m := make(map[int16]apiKeyEntry, len(apiKeyTable))
	for _, e := range apiKeyTable {
		m[e.key] = e
	}
	return m
}()

func apiKeyRoster() []string {
	roots := make([]string, len(apiKeyTable))
	for i, e := range apiKeyTable {
		roots[i] = e.root
	}
	return roots
}

func apiKeyName(key int16) (string, bool) {
	e, ok := apiKeyByKey[key]
	return e.root, ok
}

func apiKeyMaxVersion(key int16) (int16, bool) {
	e, ok := apiKeyByKey[key]
	return e.maxVersion, ok
}

// flexibleSince maps api_key → the lowest api_version that uses the flexible
// (tagged-fields) header form; a key ABSENT from the map is NEVER flexible
// (SPEC Appendix C; Kafka 3.9.1). Keys 17 (sasl_handshake) and 47 (offset_delete)
// are never flexible (omitted). api_keys 71/72 (telemetry) excluded. 84 entries
// (86 keys minus 17 and 47).
var flexibleSince = map[int16]int16{
	0: 9, 1: 12, 2: 6, 3: 9, 4: 4, 5: 2, 6: 6, 7: 3, 8: 8, 9: 6,
	10: 3, 11: 6, 12: 4, 13: 4, 14: 4, 15: 5, 16: 3, 18: 3, 19: 5,
	20: 4, 21: 2, 22: 2, 23: 4, 24: 3, 25: 3, 26: 3, 27: 1, 28: 3, 29: 2,
	30: 2, 31: 2, 32: 4, 33: 2, 34: 2, 35: 2, 36: 2, 37: 2, 38: 2, 39: 2,
	40: 2, 41: 2, 42: 2, 43: 2, 44: 1, 45: 0, 46: 0, 48: 1, 49: 1,
	50: 0, 51: 0, 52: 0, 53: 1, 54: 1, 55: 0, 56: 0, 57: 0, 58: 0, 59: 0,
	60: 0, 61: 0, 62: 0, 63: 0, 64: 0, 65: 0, 66: 0, 67: 0, 68: 0, 69: 0,
	70: 0, 73: 0, 74: 0, 75: 0, 76: 0, 77: 0, 78: 0, 79: 0,
	80: 0, 81: 0, 82: 0, 83: 0, 84: 0, 85: 0, 86: 0, 87: 0,
}

// requestUsesTaggedFieldsInHeader reports whether a request with the given
// api_key/api_version carries a flexible (tagged-fields) request header.
func requestUsesTaggedFieldsInHeader(key, ver int16) bool {
	since, ok := flexibleSince[key]
	return ok && ver >= since
}

// responseUsesTaggedFieldsInHeader reports whether the response header for the
// given api_key/api_version carries tagged fields. ApiVersions (18) is the lone
// exception: its response header SUPPRESSES tagged fields at every version
// (AMEND-K5), even though its request header is flexible at v3+.
func responseUsesTaggedFieldsInHeader(key, ver int16) bool {
	if key == 18 { // ApiVersions: response header suppresses tagged fields (AMEND-K5)
		return false
	}
	return requestUsesTaggedFieldsInHeader(key, ver)
}
