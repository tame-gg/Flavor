fn main() {
    println!("cargo:rerun-if-changed=lattice.binpb");
    connectrpc_build::Config::new()
        .descriptor_set("lattice.binpb")
        .files(&[
            "lattice/v1/common.proto",
            "lattice/v1/daemon.proto",
            "lattice/v1/device.proto",
            "lattice/v1/diagnostics.proto",
            "lattice/v1/events.proto",
            "lattice/v1/network.proto",
        ])
        .include_file("_connectrpc.rs")
        .compile()
        .expect("generate lattice.v1 bindings");
}
