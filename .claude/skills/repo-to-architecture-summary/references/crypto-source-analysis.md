# Cryptographic Source Analysis

Inventory actual code implementation for cryptographic usage and configuration.
Complements `security-build-analysis.md` (which covers build-time signals like Dockerfiles, compiler flags, base images).

## What to Inventory

Read source code and document what you find:

**Crypto Libraries and Imports**:
- What crypto libraries are imported (stdlib, third-party, vendored)?
- Library versions and source (package manager, git, internal)
- Language-specific patterns: Go `crypto/*`, Python `cryptography`, Rust `openssl-sys`, C `openssl/`, Java `javax.net.ssl`

**TLS Configuration**:
- Where is TLS configured (server setup, client connections)?
- Minimum and maximum protocol versions set?
- Cipher suite selection (default, explicit list, or unconfigured)?
- Certificate validation: enabled, disabled, or conditionally skipped?
- Certificate sources (system CA bundle, custom CAs, hardcoded certs)?

**Cryptographic Operations**:
- What algorithms are used for hashing (MD5, SHA1, SHA256, etc.)?
- Signature algorithms (RSA, ECDSA, EdDSA)?
- Key exchange and agreement mechanisms?
- Symmetric encryption (AES, ChaCha20, etc.)?

**Token and Secret Handling**:
- JWT/token libraries and versions used?
- Signature validation: enforced, optional, or skipped?
- Token expiration checks implemented?
- Secrets in code: hardcoded, environment variables, mounted files, secrets management system?

**Error Handling**:
- How are certificate validation failures handled?
- Are crypto errors logged (and do logs expose sensitive data)?
- Fallback behavior when crypto operations fail?

## By Language: Search Patterns

### Go
```
crypto/tls, crypto/x509, crypto/sha256, crypto/aes
tls.Config, TLSVersion, CipherSuites, InsecureSkipVerify
jwt, oauth2
```

### C/C++
```
openssl/ssl.h, openssl/crypto.h, openssl/evp.h
SSL_CTX, SSL_OP_*, EVP_*
mbedTLS, GnuTLS (alternative libraries)
```

### Python
```
cryptography, ssl, hashlib, hmac
SSLContext, ssl.PROTOCOL_*, verify_mode
PyJWT, python-jose
```

### Rust
```
openssl, rustls, ring, sodiumoxide
TlsConnector, TlsAcceptor
```

### Java
```
javax.net.ssl, java.security, java.crypto
SSLContext, KeyStore, Cipher
jsonwebtoken, jjwt
```

## Documentation Format

When documenting crypto usage:

| Aspect | Finding | Source |
|--------|---------|--------|
| TLS Library | (e.g., crypto/tls, openssl, javax.net.ssl) | filepath:line |
| Min TLS Version | (e.g., TLS 1.2, TLS 1.3, not set) | filepath:line |
| Max TLS Version | (e.g., TLS 1.3, unconstrained) | filepath:line |
| Cipher Suites | (e.g., default set, custom list, not specified) | filepath:line |
| Cert Validation | (e.g., enabled, disabled, conditional) | filepath:line |
| Cert Sources | (e.g., system CA, custom CA, hardcoded) | filepath:line |
| JWT Library | (e.g., go-jose, PyJWT, none) | filepath:line |
| Signature Validation | (e.g., enforced, skipped, conditional) | filepath:line |
| Hash Algorithms | (e.g., SHA256 for signing, MD5 for checksums) | filepath:lines |
| Secrets Handling | (e.g., environment variables, mounted files, hardcoded) | filepath:lines |

## When Source is Unavailable

If source code is:
- Proprietary or closed-source
- Not provided in repository (compiled binary only)
- In a language you cannot read

Document as:
- `unknown` (source unavailable)
- `not-extracted` (source available but not read due to scope constraints)
- Include reason (e.g., "C++ implementation proprietary", "binary-only distribution")
