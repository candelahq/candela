fn main() -> Result<(), Box<dyn std::error::Error>> {
    let proto_dir = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("proto");

    let proto_files = [
        proto_dir.join("candela/v1/harness_service.proto"),
        proto_dir.join("candela/types/session.proto"),
        proto_dir.join("candela/types/chat.proto"),
    ];

    connectrpc_build::Config::new()
        .files(&proto_files)
        .includes(&[&proto_dir])
        .include_file("_connectrpc.rs")
        .compile()?;

    // Rerun if any proto file changes.
    for file in &proto_files {
        println!("cargo:rerun-if-changed={}", file.display());
    }

    Ok(())
}
