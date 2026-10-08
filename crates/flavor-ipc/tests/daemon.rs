use std::collections::{HashMap, HashSet};
use std::path::{Path, PathBuf};
use std::process::{Child, Command};
use std::sync::OnceLock;
use std::time::Duration;

use flavor_ipc::proto::daemon_event::Payload;
use flavor_ipc::proto::*;
use flavor_ipc::FlavorIpcClient;

fn fixture_binary() -> &'static Path {
    static BIN: OnceLock<PathBuf> = OnceLock::new();
    BIN.get_or_init(|| {
        let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("../..");
        let out = Path::new(env!("CARGO_TARGET_TMPDIR")).join("flavor-fakedaemon");
        let status = Command::new(root.join("scripts/go.sh"))
            .args(["build", "-o"])
            .arg(&out)
            .arg("./test/fakedaemon")
            .current_dir(&root)
            .status()
            .expect("run go build");
        assert!(status.success(), "building fake daemon failed");
        out
    })
}

struct Daemon {
    child: Child,
    socket: PathBuf,
    _dir: tempfile::TempDir,
}

impl Drop for Daemon {
    fn drop(&mut self) {
        let _ = self.child.kill();
        let _ = self.child.wait();
    }
}

async fn spawn_daemon() -> Daemon {
    let dir = tempfile::tempdir().unwrap();
    let run = dir.path().join("run");
    std::fs::create_dir(&run).unwrap();
    std::fs::set_permissions(&run, std::os::unix::fs::PermissionsExt::from_mode(0o700)).unwrap();
    let child = Command::new(fixture_binary())
        .env("HOME", dir.path())
        .env("XDG_DATA_HOME", dir.path().join("data"))
        .env("XDG_CONFIG_HOME", dir.path().join("config"))
        .env("XDG_RUNTIME_DIR", &run)
        .spawn()
        .expect("spawn fake daemon");
    let socket = run.join("flavor/flavord.sock");
    for _ in 0..200 {
        if socket.exists() {
            return Daemon { child, socket, _dir: dir };
        }
        tokio::time::sleep(Duration::from_millis(25)).await;
    }
    panic!("fake daemon did not create its socket");
}

async fn add(client: &FlavorIpcClient, name: &str, url: &str) -> Network {
    let res = client
        .networks
        .add_network(AddNetworkRequest {
            display_name: name.into(),
            provider: ProviderType::PROVIDER_TYPE_HEADSCALE.into(),
            control_url: url.into(),
            ..Default::default()
        })
        .await
        .unwrap()
        .into_owned();
    res.network.as_option().unwrap().clone()
}

#[tokio::test]
async fn snapshot_stream_and_duplicate_addresses_survive_rust_client() {
    let daemon = spawn_daemon().await;
    let client = FlavorIpcClient::new(&daemon.socket);

    let info = client.daemon.get_daemon_info(GetDaemonInfoRequest::default()).await.unwrap().into_owned();
    assert_eq!(info.protocol_major, flavor_ipc::PROTOCOL_MAJOR);
    let base = client.daemon.get_state_snapshot(GetStateSnapshotRequest::default()).await.unwrap().into_owned();
    assert_eq!(base.daemon_instance_id, info.daemon_instance_id);

    let mut stream = client
        .events
        .watch_events(WatchEventsRequest {
            daemon_instance_id: base.daemon_instance_id.clone(),
            after_sequence: base.snapshot_sequence,
            ..Default::default()
        })
        .await
        .unwrap();

    let a = add(&client, "Office", "https://a.example.com").await;
    let b = add(&client, "Home", "https://b.example.com").await;
    for n in [&a, &b] {
        client
            .networks
            .connect_network(ConnectNetworkRequest { network_id: n.id.clone(), ..Default::default() })
            .await
            .unwrap();
    }

    let mut next = base.snapshot_sequence + 1;
    let mut connected = HashSet::new();
    while connected.len() < 2 {
        let msg = tokio::time::timeout(Duration::from_secs(5), stream.message::<DaemonEvent>())
            .await
            .expect("event within deadline")
            .unwrap()
            .expect("stream open")
            .to_owned_message();
        assert_eq!(msg.sequence_id, next, "sequence gap");
        assert_eq!(msg.daemon_instance_id, base.daemon_instance_id);
        next += 1;
        if let Some(Payload::NetworkStateChanged(ev)) = &msg.payload {
            if ev.state == NetworkConnectionState::NETWORK_CONNECTION_STATE_CONNECTED {
                connected.insert(ev.network_id.clone());
            }
        }
    }

    let snap = client.daemon.get_state_snapshot(GetStateSnapshotRequest::default()).await.unwrap().into_owned();
    assert_eq!(snap.networks.len(), 2);
    let mut by_addr: HashMap<String, Vec<(String, String)>> = HashMap::new();
    let mut keys = HashSet::new();
    for d in &snap.devices {
        let key = (d.id.network_id.clone(), d.id.node_id.clone());
        assert!(keys.insert(key.clone()), "device identity collapsed: {key:?}");
        for addr in &d.addresses {
            by_addr.entry(addr.clone()).or_default().push(key.clone());
        }
    }
    let dup = &by_addr["100.64.0.1"];
    assert_eq!(dup.len(), 2, "both networks report 100.64.0.1");
    assert_ne!(dup[0].0, dup[1].0);

    let json = serde_json::to_value(&snap).unwrap();
    assert_eq!(json["devices"].as_array().unwrap().len(), snap.devices.len());
}

#[tokio::test]
async fn foreign_instance_requires_resync() {
    let daemon = spawn_daemon().await;
    let client = FlavorIpcClient::new(&daemon.socket);
    let result = client
        .events
        .watch_events(WatchEventsRequest {
            daemon_instance_id: "previous-instance".into(),
            after_sequence: 0,
            ..Default::default()
        })
        .await;
    let err = match result {
        Err(e) => flavor_ipc::IpcError::from(e),
        Ok(mut s) => flavor_ipc::IpcError::from(s.message::<DaemonEvent>().await.unwrap_err()),
    };
    assert!(err.is_resync_required(), "{err}");
}

#[tokio::test]
async fn typed_daemon_errors_and_missing_daemon() {
    let daemon = spawn_daemon().await;
    let client = FlavorIpcClient::new(&daemon.socket);
    let err = flavor_ipc::IpcError::from(
        client
            .networks
            .add_network(AddNetworkRequest {
                display_name: "Bad".into(),
                provider: ProviderType::PROVIDER_TYPE_HEADSCALE.into(),
                control_url: "ftp://nope".into(),
                ..Default::default()
            })
            .await
            .unwrap_err(),
    );
    assert_eq!(err.flavor_code(), Some(FlavorErrorCode::FLAVOR_ERROR_CODE_INVALID_CONTROL_URL));
    assert!(!err.retryable());

    let missing = FlavorIpcClient::new(daemon.socket.with_file_name("absent.sock"));
    let err = flavor_ipc::IpcError::from(
        missing.daemon.get_daemon_info(GetDaemonInfoRequest::default()).await.unwrap_err(),
    );
    assert!(err.is_unavailable(), "{err:?}");
}

#[tokio::test]
async fn inspector_reports_duplicate_addresses_and_unique_names() {
    let daemon = spawn_daemon().await;
    let client = FlavorIpcClient::new(&daemon.socket);
    let a = add(&client, "LunarLabs", "https://a.example.com").await;
    let b = add(&client, "Home", "https://b.example.com").await;
    for n in [&a, &b] {
        client
            .networks
            .connect_network(ConnectNetworkRequest { network_id: n.id.clone(), ..Default::default() })
            .await
            .unwrap();
    }
    let inspect = |dest: &str| {
        let client = client.clone();
        let dest = dest.to_string();
        async move {
            client
                .inspector
                .inspect_destination(InspectDestinationRequest { destination: dest, ..Default::default() })
                .await
                .map(|r| r.into_owned())
        }
    };
    let mut amb = inspect("100.64.0.1").await.unwrap();
    for _ in 0..100 {
        if amb.candidates.len() == 2 {
            break;
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
        amb = inspect("100.64.0.1").await.unwrap();
    }
    assert_eq!(amb.decision, ResolutionDecision::RESOLUTION_DECISION_AMBIGUOUS);
    let nets: HashSet<_> = amb.candidates.iter().map(|c| c.network.id.clone()).collect();
    assert_eq!(nets, HashSet::from([a.id.clone(), b.id.clone()]));

    let host = inspect("postgres").await.unwrap();
    assert_eq!(host.decision, ResolutionDecision::RESOLUTION_DECISION_UNIQUE);
    let owner = if host.candidates[0].network.id == a.id { &a } else { &b };
    let dns = format!("postgres.{}.flavor.test", owner.id.to_lowercase());
    let uniq = inspect(&dns).await.unwrap();
    assert_eq!(uniq.decision, ResolutionDecision::RESOLUTION_DECISION_UNIQUE);
    assert_eq!(uniq.reason, DecisionReason::DECISION_REASON_DEVICE_DNS_NAME);
    assert_eq!(uniq.candidates[0].network.id, owner.id);

    let collide = inspect("grafana").await.unwrap();
    assert_eq!(collide.decision, ResolutionDecision::RESOLUTION_DECISION_AMBIGUOUS);

    let err = flavor_ipc::IpcError::from(inspect("not a destination").await.unwrap_err());
    assert_eq!(err.flavor_code(), Some(FlavorErrorCode::FLAVOR_ERROR_CODE_INVALID_ARGUMENT));

    let conflicts = client.conflicts.list_conflicts(ListConflictsRequest::default()).await.unwrap().into_owned();
    let ids: HashSet<_> = conflicts.conflicts.iter().map(|c| c.id.clone()).collect();
    assert!(ids.contains("address:100.64.0.2"), "{ids:?}");
    assert!(ids.contains("name:grafana"), "{ids:?}");
    assert!(!ids.contains("name:postgres"), "{ids:?}");
    assert!(ids.contains("subnet:10.10.0.0/16"), "identical subnet routes on two networks: {ids:?}");

    let routed = inspect("10.99.0.1").await.unwrap();
    assert_eq!(routed.decision, ResolutionDecision::RESOLUTION_DECISION_UNIQUE);
    assert_eq!(routed.reason, DecisionReason::DECISION_REASON_SUBNET_ROUTE);
    assert_eq!(routed.candidates[0].matched_value, "10.0.0.0/8");
    let tie = inspect("10.10.1.1").await.unwrap();
    assert_eq!(tie.decision, ResolutionDecision::RESOLUTION_DECISION_AMBIGUOUS);
    let qualified = inspect(&format!("postgres.{}.flavor.internal", owner.label)).await.unwrap();
    assert_eq!(qualified.reason, DecisionReason::DECISION_REASON_NETWORK_QUALIFIED_NAME);
    assert_eq!(qualified.candidates[0].network.id, owner.id);
    assert!(!ids.contains("address:100.64.0.1"), "this machine reported as a conflict: {ids:?}");
    let addr = conflicts.conflicts.iter().find(|c| c.id == "address:100.64.0.2").unwrap();
    assert_eq!(addr.severity, ConflictSeverity::CONFLICT_SEVERITY_EXPECTED);
    assert!(addr.network_context_resolves);
}

#[tokio::test]
async fn workspaces_round_trip_through_rust_client() {
    let daemon = spawn_daemon().await;
    let client = FlavorIpcClient::new(&daemon.socket);
    let a = add(&client, "LunarLabs", "https://a.example.com").await;
    let b = add(&client, "Home", "https://b.example.com").await;
    let created = client
        .workspaces
        .create_workspace(CreateWorkspaceRequest {
            name: "Work".into(),
            network_ids: vec![a.id.clone(), b.id.clone()],
            ..Default::default()
        })
        .await
        .unwrap()
        .into_owned();
    let ws = created.workspace.as_option().unwrap().clone();
    let act = client
        .workspaces
        .activate_workspace(ActivateWorkspaceRequest { workspace_id: ws.id.clone(), ..Default::default() })
        .await
        .unwrap()
        .into_owned();
    assert_eq!(act.results.len(), 2);
    let snap = client.daemon.get_state_snapshot(GetStateSnapshotRequest::default()).await.unwrap().into_owned();
    assert_eq!(snap.active_workspace_id, ws.id);
    assert_eq!(snap.workspaces.len(), 1);
    let json = serde_json::to_value(&snap).unwrap();
    assert_eq!(json["activeWorkspaceId"], ws.id);

    let err = flavor_ipc::IpcError::from(
        client
            .workspaces
            .delete_workspace(DeleteWorkspaceRequest { workspace_id: "01NOSUCHWORKSPACE000000000".into(), ..Default::default() })
            .await
            .unwrap_err(),
    );
    assert_eq!(err.flavor_code(), Some(FlavorErrorCode::FLAVOR_ERROR_CODE_WORKSPACE_NOT_FOUND));
}

#[tokio::test]
async fn destination_preference_changes_the_inspector_decision() {
    let daemon = spawn_daemon().await;
    let client = FlavorIpcClient::new(&daemon.socket);
    let a = add(&client, "LunarLabs", "https://a.example.com").await;
    let b = add(&client, "Home", "https://b.example.com").await;
    for n in [&a, &b] {
        client
            .networks
            .connect_network(ConnectNetworkRequest { network_id: n.id.clone(), ..Default::default() })
            .await
            .unwrap();
    }
    let inspect = || async {
        client
            .inspector
            .inspect_destination(InspectDestinationRequest { destination: "100.64.0.2".into(), ..Default::default() })
            .await
            .unwrap()
            .into_owned()
    };
    let mut before = inspect().await;
    for _ in 0..100 {
        if before.candidates.len() == 2 {
            break;
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
        before = inspect().await;
    }
    assert_eq!(before.decision, ResolutionDecision::RESOLUTION_DECISION_AMBIGUOUS);
    client
        .preferences
        .set_destination_preference(SetDestinationPreferenceRequest {
            destination: "100.64.0.2".into(),
            network_id: b.id.clone(),
            ..Default::default()
        })
        .await
        .unwrap();
    let after = inspect().await;
    assert_eq!(after.decision, ResolutionDecision::RESOLUTION_DECISION_UNIQUE);
    assert_eq!(after.reason, DecisionReason::DECISION_REASON_DESTINATION_PREFERENCE);
    assert_eq!(after.preference.state, PreferenceState::PREFERENCE_STATE_APPLIED);
    let selected = after.candidates.iter().find(|c| c.status == CandidateStatus::CANDIDATE_STATUS_SELECTED).unwrap();
    assert_eq!(selected.network.id, b.id);
}
