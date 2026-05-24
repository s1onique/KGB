// interface_sampler.zig — In-memory interface sampler state
//
// ACT 2: Sampler state that matches current interface counter samples against
// previous samples by interface name and returns each current interface with an
// optional calculated rate.
//
// This module does NOT:
// - Wire into HTTP
// - Change /metrics.json
// - Read sysfs

const std = @import("std");
const rates = @import("rates.zig");
const interface_filter = @import("interface_filter.zig");

// ============================================================================
// Data Structures
// ============================================================================

/// A current interface sample enriched with an optional calculated rate.
pub const SampledInterface = struct {
    sample: rates.InterfaceCounterSample,
    rate: ?rates.InterfaceRate,
    /// Whether this interface is a tunnel (WireGuard, TUN, TAP, etc.)
    is_tunnel: bool,
};

/// Full sample data stored per interface in the sampler.
/// We store all counters needed for rate calculation.
/// The name is the hash map key - stored separately from this struct.
const StoredSample = struct {
    rx_bytes: u64,
    tx_bytes: u64,
    rx_packets: u64,
    tx_packets: u64,
    sampled_at_ms: i64,
};

// ============================================================================
// Interface Sampler
// ============================================================================

/// In-memory sampler that tracks interface samples across updates.
///
/// Owns previous samples and matches them against current samples by interface
/// name to compute rates.
///
/// Ownership model:
/// - Result slice: owned by caller (must free with allocator)
///   - Each SampledInterface.sample.name is a duplicated string, caller frees
/// - Map keys: owned by sampler, freed in deinit() and when replaced
/// - No allocation is shared between result and sampler state
pub const InterfaceSampler = struct {
    allocator: std.mem.Allocator,
    /// Previous samples, keyed by interface name.
    /// We own all key memory and must free it on deinit/replace.
    previous: std.StringHashMapUnmanaged(StoredSample),

    /// Initialize a new InterfaceSampler.
    pub fn init(allocator: std.mem.Allocator) InterfaceSampler {
        return .{
            .allocator = allocator,
            .previous = .{},
        };
    }

    /// Free all sampler-owned memory.
    /// Safe to call even if update() was never called.
    pub fn deinit(self: *InterfaceSampler) void {
        var it = self.previous.iterator();
        while (it.next()) |entry| {
            self.allocator.free(entry.key_ptr.*);
        }
        self.previous.deinit(self.allocator);
    }

    /// Update sampler with current samples and compute rates.
    ///
    /// For each current sample:
    /// 1. Look up previous sample by interface name (exact match)
    /// 2. Calculate rate using rates.calculateRate()
    /// 3. Build SampledInterface result (caller owns names)
    /// 4. Update sampler state (sampler owns map keys separately)
    ///
    /// Disappeared interfaces (in previous but not in current) are removed.
    ///
    /// Returns slice owned by caller; must be freed with allocator.
    pub fn update(
        self: *InterfaceSampler,
        current: []const rates.InterfaceCounterSample,
    ) ![]SampledInterface {
        var result = try std.ArrayList(SampledInterface).initCapacity(
            self.allocator,
            current.len,
        );
        errdefer result.deinit(self.allocator);

        // Process current samples
        for (current) |*sample| {
            // Look up previous sample by name
            const prev_opt = self.previous.get(sample.name);

            // Calculate rate using previous (may be null)
            const rate = rates.calculateRate(
                if (prev_opt) |prev| .{
                    .name = sample.name,
                    .rx_bytes = prev.rx_bytes,
                    .tx_bytes = prev.tx_bytes,
                    .rx_packets = prev.rx_packets,
                    .tx_packets = prev.tx_packets,
                    .sampled_at_ms = prev.sampled_at_ms,
                } else null,
                sample.*,
            );

            // Create owned copy of the name for the result (caller frees)
            const result_name = try self.allocator.dupe(u8, sample.name);
            errdefer self.allocator.free(result_name);
            
            const result_sample = rates.InterfaceCounterSample{
                .name = result_name,
                .rx_bytes = sample.rx_bytes,
                .tx_bytes = sample.tx_bytes,
                .rx_packets = sample.rx_packets,
                .tx_packets = sample.tx_packets,
                .sampled_at_ms = sample.sampled_at_ms,
            };

            try result.append(self.allocator, .{
                .sample = result_sample,
                .rate = rate,
                .is_tunnel = interface_filter.isTunnelInterface(sample.name),
            });

            // Update sampler state - check if key already exists
            if (self.previous.contains(sample.name)) {
                // Key exists - update the value, keep the key (don't leak)
                try self.previous.put(self.allocator, sample.name, .{
                    .rx_bytes = sample.rx_bytes,
                    .tx_bytes = sample.tx_bytes,
                    .rx_packets = sample.rx_packets,
                    .tx_packets = sample.tx_packets,
                    .sampled_at_ms = sample.sampled_at_ms,
                });
            } else {
                // New key - create ownership
                const map_key = try self.allocator.dupe(u8, sample.name);
                try self.previous.put(self.allocator, map_key, .{
                    .rx_bytes = sample.rx_bytes,
                    .tx_bytes = sample.tx_bytes,
                    .rx_packets = sample.rx_packets,
                    .tx_packets = sample.tx_packets,
                    .sampled_at_ms = sample.sampled_at_ms,
                });
            }
        }

        // Find and remove disappeared interfaces.
        // Collect keys first since we can't modify hash map while iterating.
        var to_remove_keys = std.ArrayListUnmanaged([]const u8){
            .items = &.{},
            .capacity = 0,
        };
        defer to_remove_keys.deinit(self.allocator);

        var it = self.previous.iterator();
        while (it.next()) |entry| {
            var found = false;
            for (current) |sample| {
                if (std.mem.eql(u8, sample.name, entry.key_ptr.*)) {
                    found = true;
                    break;
                }
            }
            if (!found) {
                // This interface disappeared - save key for removal and freeing
                try to_remove_keys.append(self.allocator, entry.key_ptr.*);
            }
        }

        // Remove the keys and free them
        for (to_remove_keys.items) |key| {
            _ = self.previous.remove(key);
            self.allocator.free(key);
        }

        return result.toOwnedSlice(self.allocator);
    }
};
