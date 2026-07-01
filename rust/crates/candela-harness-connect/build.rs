fn main() -> Result<(), Box<dyn std::error::Error>> {
    let proto_dir = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../../proto");

    connectrpc_build::Config::new()
        .files(&[proto_dir.join("candela/harness/v1/harness.proto")])
        .includes(&[proto_dir])
        .include_file("_connectrpc.rs")
        .compile()?;

    Ok(())
}
