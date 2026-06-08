package kafkabroker

import "testing"

func TestApiKeyRoster_MatchesUpstream(t *testing.T) {
	want := []string{
		"produce", "fetch", "list_offsets", "metadata", "leader_and_isr",
		"stop_replica", "update_metadata", "controlled_shutdown", "offset_commit", "offset_fetch",
		"find_coordinator", "join_group", "heartbeat", "leave_group", "sync_group",
		"describe_groups", "list_groups", "sasl_handshake", "api_versions", "create_topics",
		"delete_topics", "delete_records", "init_producer_id", "offset_for_leader_epoch", "add_partitions_to_txn",
		"add_offsets_to_txn", "end_txn", "write_txn_markers", "txn_offset_commit", "describe_acls",
		"create_acls", "delete_acls", "describe_configs", "alter_configs", "alter_replica_log_dirs",
		"describe_log_dirs", "sasl_authenticate", "create_partitions", "create_delegation_token", "renew_delegation_token",
		"expire_delegation_token", "describe_delegation_token", "delete_groups", "elect_leaders", "incremental_alter_configs",
		"alter_partition_reassignments", "list_partition_reassignments", "offset_delete", "describe_client_quotas", "alter_client_quotas",
		"describe_user_scram_credentials", "alter_user_scram_credentials", "vote", "begin_quorum_epoch", "end_quorum_epoch",
		"describe_quorum", "alter_partition", "update_features", "envelope", "fetch_snapshot",
		"describe_cluster", "describe_producers", "broker_registration", "broker_heartbeat", "unregister_broker",
		"describe_transactions", "list_transactions", "allocate_producer_ids", "consumer_group_heartbeat", "consumer_group_describe",
		"controller_registration", // key 70 (key 71/72 telemetry-excluded next)
		"assign_replicas_to_dirs", "list_client_metrics_resources", "describe_topic_partitions", "share_group_heartbeat", "share_group_describe",
		"share_fetch", "share_acknowledge", "add_raft_voter", "remove_raft_voter", "update_raft_voter",
		"initialize_share_group_state", "read_share_group_state", "write_share_group_state", "delete_share_group_state",
		"read_share_group_state_summary", // key 87 (last)
	}
	got := apiKeyRoster()
	if len(got) != 86 {
		t.Fatalf("roster len = %d, want 86", len(got))
	}
	if len(want) != 86 {
		t.Fatalf("want list len = %d, want 86 (test transcription error)", len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("roster[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestApiKeyName(t *testing.T) {
	if n, ok := apiKeyName(0); !ok || n != "produce" {
		t.Fatalf("apiKeyName(0) = %q,%v want produce,true", n, ok)
	}
	if n, ok := apiKeyName(18); !ok || n != "api_versions" {
		t.Fatalf("apiKeyName(18) = %q,%v want api_versions,true", n, ok)
	}
	if n, ok := apiKeyName(87); !ok || n != "read_share_group_state_summary" {
		t.Fatalf("apiKeyName(87) = %q,%v want read_share_group_state_summary,true", n, ok)
	}
	if _, ok := apiKeyName(71); ok {
		t.Fatal("apiKeyName(71) should be absent (telemetry-excluded)")
	}
	if _, ok := apiKeyName(72); ok {
		t.Fatal("apiKeyName(72) should be absent (telemetry-excluded)")
	}
	if _, ok := apiKeyName(9999); ok {
		t.Fatal("apiKeyName(9999) should be absent (unknown key)")
	}
}

func TestApiKeyMaxVersion(t *testing.T) {
	if v, ok := apiKeyMaxVersion(18); !ok || v < 3 {
		t.Fatalf("apiKeyMaxVersion(18) = %d,%v want >=3,true", v, ok)
	}
	if v, ok := apiKeyMaxVersion(0); !ok || v < 9 {
		t.Fatalf("apiKeyMaxVersion(0 produce) = %d,%v want >=9,true", v, ok)
	}
	if v, ok := apiKeyMaxVersion(1); !ok || v < 13 {
		t.Fatalf("apiKeyMaxVersion(1 fetch) = %d,%v want >=13,true", v, ok)
	}
	if v, ok := apiKeyMaxVersion(3); !ok || v < 9 {
		t.Fatalf("apiKeyMaxVersion(3 metadata) = %d,%v want >=9,true", v, ok)
	}
	if _, ok := apiKeyMaxVersion(71); ok {
		t.Fatal("apiKeyMaxVersion(71) should be absent (telemetry-excluded)")
	}
}

// TestFlexibleSinceKeysetMatchesTable locks the two parallel data structures
// together: flexibleSince's keyset must equal the apiKeyTable keyset MINUS the
// two never-flexible keys {17 sasl_handshake, 47 offset_delete}. Catches future
// drift when keys are added to one structure but not the other.
func TestFlexibleSinceKeysetMatchesTable(t *testing.T) {
	never := map[int16]bool{17: true, 47: true} // sasl_handshake, offset_delete
	// every table key except 17/47 must be in flexibleSince:
	for _, e := range apiKeyTable {
		_, flex := flexibleSince[e.key]
		if never[e.key] && flex {
			t.Errorf("key %d (%s) should be never-flexible but is in flexibleSince", e.key, e.root)
		}
		if !never[e.key] && !flex {
			t.Errorf("key %d (%s) missing from flexibleSince", e.key, e.root)
		}
	}
	// no extra keys in flexibleSince beyond the table:
	if len(flexibleSince) != len(apiKeyTable)-len(never) {
		t.Errorf("flexibleSince size = %d, want %d", len(flexibleSince), len(apiKeyTable)-len(never))
	}
}

func TestHeaderVersion(t *testing.T) {
	cases := []struct {
		key, ver          int16
		reqFlex, respFlex bool
	}{
		{0, 8, false, false},    // produce flexible at 9+ → v8 not flexible
		{0, 9, true, true},      // produce v9 flexible
		{18, 3, true, false},    // ApiVersions req v3 flexible; RESPONSE header suppressed (AMEND-K5)
		{18, 2, false, false},   // ApiVersions v2 below flex floor
		{17, 0, false, false},   // sasl_handshake never flexible
		{47, 0, false, false},   // offset_delete never flexible
		{60, 0, true, true},     // describe_cluster flexible at 0+
		{1, 12, true, true},     // fetch flexible at 12+
		{1, 11, false, false},   // fetch v11 below flex floor
		{18, 4, true, false},    // ApiVersions req v4 flexible; response still suppressed
		{9999, 0, false, false}, // unknown key never flexible
		{87, 0, true, true},     // read_share_group_state_summary flexible at 0+
	}
	for _, c := range cases {
		if got := requestUsesTaggedFieldsInHeader(c.key, c.ver); got != c.reqFlex {
			t.Errorf("request(%d,%d) = %v, want %v", c.key, c.ver, got, c.reqFlex)
		}
		if got := responseUsesTaggedFieldsInHeader(c.key, c.ver); got != c.respFlex {
			t.Errorf("response(%d,%d) = %v, want %v", c.key, c.ver, got, c.respFlex)
		}
	}
}
