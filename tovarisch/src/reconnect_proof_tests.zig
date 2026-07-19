// reconnect_proof_tests.zig — package-root entry for the bounded-memory proof.
// The implementation is split across the BGP siblings so each stays under
// the LLM-friendliness hard limit; this root entry keeps direct
// `zig test src/reconnect_proof_tests.zig` within Zig's module path.
//
// ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01-FA-3: the dedicated
// proof root now pulls in the production-init lifecycle regression
// alongside the 10,000-generation proof, the state-oracle contract
// tests, the production connector regression, and the forged-handle
// identity test. Without this entry the dedicated
// `make tovarisch-bounded-memory-reconnect-proof` step would only
// see the older proofs and miss the FA-3 wiring regression.
const std = @import("std");

test {
    std.testing.refAllDecls(@import("bgp/reconnect_proof_harness.zig"));
    std.testing.refAllDecls(@import("bgp/reconnect_proof_tests.zig"));
    std.testing.refAllDecls(@import("bgp/reconnect_proof_regression.zig"));
    std.testing.refAllDecls(@import("bgp/reconnect_proof_production_init_tests.zig"));
    std.testing.refAllDecls(@import("bgp/reconnect_proof_validate_destroy_tests.zig"));
    std.testing.refAllDecls(@import("bgp/reconnect_proof_constructor_failure_tests.zig"));
}
