fn main() {
    println!("cargo:rerun-if-changed=flavor.binpb");
    connectrpc_build::Config::new()
        .descriptor_set("flavor.binpb")
        .files(&[
            "flavor/v1/common.proto",
            "flavor/v1/conflicts.proto",
            "flavor/v1/daemon.proto",
            "flavor/v1/device.proto",
            "flavor/v1/diagnostics.proto",
            "flavor/v1/events.proto",
            "flavor/v1/forward.proto",
            "flavor/v1/inspector.proto",
            "flavor/v1/network.proto",
            "flavor/v1/preferences.proto",
            "flavor/v1/workspaces.proto",
        ])
        .include_file("_connectrpc.rs")
        .compile()
        .expect("generate flavor.v1 bindings");
}
