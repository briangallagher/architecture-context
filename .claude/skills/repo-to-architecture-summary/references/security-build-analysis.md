# Security and Build Analysis

Load for legacy runs or declared partial security/build gaps.

## Build-Time FIPS Signals by Language

Look for these indicators in Dockerfiles and build scripts:

**Go**: `GOEXPERIMENT=strictfipsruntime`, `CGO_ENABLED=1`, dynamic linking against OpenSSL, `check-payload` Konflux validation

**C/C++**: UBI 9/10+ base images, `openssl-devel` build dependency, `crypto.h` or `openssl/ssl.h` includes, `-fPIE` or `-fPIC` flags for static-PIE linking, `glibc` FIPS mode

**Rust**: UBI 9/10+ base images, `openssl` or `rustls` (if FIPS-approved), static-PIE linked binaries, check-payload validation

**Python**: Virtual environment with FIPS-approved packages (cryptography >= 40), system OpenSSL integration, no pure-Python crypto implementations

**All languages**: UBI 9-minimal-pqc (post-quantum variant), Konflux check-payload gate in CI, rpms.lock.yaml for OS package pinning

## Runtime FIPS Checks

- TLS configuration: MinVersion TLS 1.2+, cipher suite selection
- No `InsecureSkipVerify`, disabled cert validation, or hardcoded credentials
- Crypto libraries: stdlib (Go), cryptography (Python), openssl-sys (Rust), java.security (Java)
- Non-FIPS risks: MD5, RC4, weak key exchange, unvalidated certs

Check hermeticity at every layer: `rpms.lock.yaml`, language lock files,
`artifacts.lock.yaml`, and Hermeto/cachi2 prefetch configuration. Record what
exists on the analyzed branch and distinguish upstream from downstream
release hardening. Never infer compliance from a missing signal; render
`unknown` or `not-extracted`.
