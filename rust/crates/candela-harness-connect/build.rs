fn main() -> Result<(), Box<dyn std::error::Error>> {
    let proto_dir = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("proto");
    let proto_file = proto_dir.join("candela/harness/v1/harness.proto");

    connectrpc_build::Config::new()
        .files(std::slice::from_ref(&proto_file))
        .includes(&[proto_dir])
        .include_file("_connectrpc.rs")
        .compile()?;

    println!("cargo:rerun-if-changed={}", proto_file.display());

    Ok(())
}
