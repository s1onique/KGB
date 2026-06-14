// bgp/export_reload_apply_integration_tests.zig — Integration tests for prefix reload with delta application
// Tests reloadAndApply() with real temp files (not manual computeDelta/applyDelta)

const std = @import("std");
const types = @import("types.zig");
const session = @import("session.zig");
const export_reload_apply = @import("export_reload_apply.zig");
const linux_stats = @import("../net/linux_stats.zig");
const prefix_watch = @import("prefix_watch.zig");
const prefix_watch_fake = @import("prefix_watch_fake.zig");

fn buildPeerOpen(peer_as: u16, router_id: [4]u8) [29]u8 {
    var peer_open: [29]u8 = undefined;
    @memset(peer_open[0..16], 0xFF);
    peer_open[16] = 0;
    peer_open[17] = 29;
    peer_open[18] = 1;
    peer_open[19] = 4;
    peer_open[20] = @as(u8, @intCast(peer_as / 256));
    peer_open[21] = @as(u8, @intCast(peer_as % 256));
    peer_open[22] = 0;
    peer_open[23] = 180;
    peer_open[24] = router_id[0];
    peer_open[25] = router_id[1];
    peer_open[26] = router_id[2];
    peer_open[27] = router_id[3];
    peer_open[28] = 0;
    return peer_open;
}

fn buildPeerKeepalive() [19]u8 {
    var peer_keepalive: [19]u8 = undefined;
    @memset(peer_keepalive[0..16], 0xFF);
    peer_keepalive[16] = 0;
    peer_keepalive[17] = 19;
    peer_keepalive[18] = 4;
    return peer_keepalive;
}

fn createEstablishedSession(allocator: std.mem.Allocator, local_as: u16, peer_as: u16) !struct { session.Session, session.FakeTransport } {
    var fake = try session.FakeTransport.init(allocator, &.{
        session.PeerResponse{ .recv_bytes = &buildPeerOpen(peer_as, .{ 10, 0, 0, 2 }) },
        session.PeerResponse{ .recv_bytes = &buildPeerKeepalive() },
    });
    errdefer fake.deinit();
    const trans = fake.toTransport();
    var sess = try session.initWithClock(session.SessionConfig{
        .peer_address = .{ 10, 0, 0, 2 },
        .peer_port = 179,
        .local_address = .{ 10, 0, 0, 1 },
        .local_as = local_as,
        .peer_as = peer_as,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{},
        .same_as = false,
    }, &trans, session.MockClock.interface());
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    try std.testing.expect(session.isEstablished(&sess));
    return .{ sess, fake };
}

test "reloadAndApply: added prefix sends announcement UPDATE" {
    session.MockClock.reset();
    var export_state = export_reload_apply.ExportState{};
    export_state.init(std.testing.allocator);
    defer export_state.deinit();

    var initial = std.ArrayList(types.Ipv4Prefix).empty;
    try initial.append(std.testing.allocator, types.Ipv4Prefix.init("10.0.0.0/8"));
    try initial.append(std.testing.allocator, types.Ipv4Prefix.init("192.168.0.0/16"));
    defer initial.deinit(std.testing.allocator);
    export_reload_apply.initExportedPrefixes(&export_state, initial.items);

    const prefix_path = "/tmp/tovarisch_test_prefixes.conf";
    try linux_stats.writeFile(prefix_path, "10.0.0.0/8\n192.168.0.0/16\n172.16.0.0/12\n");
    // Note: file in /tmp/ auto-cleaned on reboot; no explicit defer cleanup needed

    const fake_res = try createEstablishedSession(std.testing.allocator, 65001, 65002);
    var sess = fake_res[0];
    var fake = fake_res[1];
    defer fake.deinit();

    const result = export_reload_apply.reloadAndApply(&export_state, prefix_path, &sess);

    try std.testing.expect(result.reload_success);
    try std.testing.expectEqual(@as(usize, 1), result.delta_added_count);
    try std.testing.expectEqual(@as(usize, 0), result.delta_removed_count);
    try std.testing.expectEqual(@as(usize, 1), result.announcements_sent);
    try std.testing.expectEqual(@as(usize, 0), result.withdrawals_sent);
    try std.testing.expectEqual(@as(usize, 3), result.current_prefix_count);
}

test "reloadAndApply: removed prefix sends withdrawal UPDATE" {
    session.MockClock.reset();
    var export_state = export_reload_apply.ExportState{};
    export_state.init(std.testing.allocator);
    defer export_state.deinit();

    var initial = std.ArrayList(types.Ipv4Prefix).empty;
    try initial.append(std.testing.allocator, types.Ipv4Prefix.init("10.0.0.0/8"));
    try initial.append(std.testing.allocator, types.Ipv4Prefix.init("192.168.0.0/16"));
    try initial.append(std.testing.allocator, types.Ipv4Prefix.init("172.16.0.0/12"));
    defer initial.deinit(std.testing.allocator);
    export_reload_apply.initExportedPrefixes(&export_state, initial.items);

    const prefix_path = "/tmp/tovarisch_test_prefixes.conf";
    try linux_stats.writeFile(prefix_path, "10.0.0.0/8\n192.168.0.0/16\n");
    // Note: file in /tmp/ auto-cleaned on reboot; no explicit defer cleanup needed

    const fake_res = try createEstablishedSession(std.testing.allocator, 65001, 65002);
    var sess = fake_res[0];
    var fake = fake_res[1];
    defer fake.deinit();

    const result = export_reload_apply.reloadAndApply(&export_state, prefix_path, &sess);

    try std.testing.expect(result.reload_success);
    try std.testing.expectEqual(@as(usize, 0), result.delta_added_count);
    try std.testing.expectEqual(@as(usize, 1), result.delta_removed_count);
    try std.testing.expectEqual(@as(usize, 1), result.withdrawals_sent);
    try std.testing.expectEqual(@as(usize, 0), result.announcements_sent);
    try std.testing.expectEqual(@as(usize, 2), result.current_prefix_count);
}

test "reloadAndApply: added + removed sends both UPDATE types" {
    session.MockClock.reset();
    var export_state = export_reload_apply.ExportState{};
    export_state.init(std.testing.allocator);
    defer export_state.deinit();

    var initial = std.ArrayList(types.Ipv4Prefix).empty;
    try initial.append(std.testing.allocator, types.Ipv4Prefix.init("10.0.0.0/8"));
    try initial.append(std.testing.allocator, types.Ipv4Prefix.init("192.168.0.0/16"));
    defer initial.deinit(std.testing.allocator);
    export_reload_apply.initExportedPrefixes(&export_state, initial.items);

    const prefix_path = "/tmp/tovarisch_test_prefixes.conf";
    try linux_stats.writeFile(prefix_path, "10.0.0.0/8\n172.16.0.0/12\n");
    // Note: file in /tmp/ auto-cleaned on reboot; no explicit defer cleanup needed

    const fake_res = try createEstablishedSession(std.testing.allocator, 65001, 65002);
    var sess = fake_res[0];
    var fake = fake_res[1];
    defer fake.deinit();

    const result = export_reload_apply.reloadAndApply(&export_state, prefix_path, &sess);

    try std.testing.expect(result.reload_success);
    try std.testing.expectEqual(@as(usize, 1), result.delta_added_count);
    try std.testing.expectEqual(@as(usize, 1), result.delta_removed_count);
    try std.testing.expectEqual(@as(usize, 1), result.withdrawals_sent);
    try std.testing.expectEqual(@as(usize, 1), result.announcements_sent);
    try std.testing.expectEqual(@as(usize, 2), result.current_prefix_count);
}

test "reloadAndApply: identical prefix set sends no UPDATE" {
    session.MockClock.reset();
    var export_state = export_reload_apply.ExportState{};
    export_state.init(std.testing.allocator);
    defer export_state.deinit();

    var prefixes = std.ArrayList(types.Ipv4Prefix).empty;
    try prefixes.append(std.testing.allocator, types.Ipv4Prefix.init("10.0.0.0/8"));
    try prefixes.append(std.testing.allocator, types.Ipv4Prefix.init("172.16.0.0/12"));
    try prefixes.append(std.testing.allocator, types.Ipv4Prefix.init("192.168.0.0/16"));
    defer prefixes.deinit(std.testing.allocator);
    export_reload_apply.initExportedPrefixes(&export_state, prefixes.items);

    const prefix_path = "/tmp/tovarisch_test_prefixes.conf";
    try linux_stats.writeFile(prefix_path, "10.0.0.0/8\n172.16.0.0/12\n192.168.0.0/16\n");
    // Note: file in /tmp/ auto-cleaned on reboot; no explicit defer cleanup needed

    const fake_res = try createEstablishedSession(std.testing.allocator, 65001, 65002);
    var sess = fake_res[0];
    var fake = fake_res[1];
    defer fake.deinit();

    const result = export_reload_apply.reloadAndApply(&export_state, prefix_path, &sess);

    try std.testing.expect(result.reload_success);
    try std.testing.expectEqual(@as(usize, 0), result.delta_added_count);
    try std.testing.expectEqual(@as(usize, 0), result.delta_removed_count);
    try std.testing.expectEqual(@as(usize, 0), result.withdrawals_sent);
    try std.testing.expectEqual(@as(usize, 0), result.announcements_sent);
    try std.testing.expectEqual(@as(usize, 3), result.current_prefix_count);
}

test "reloadAndApply: invalid reload preserves current export set" {
    var export_state = export_reload_apply.ExportState{};
    export_state.init(std.testing.allocator);
    defer export_state.deinit();

    var initial = std.ArrayList(types.Ipv4Prefix).empty;
    try initial.append(std.testing.allocator, types.Ipv4Prefix.init("10.0.0.0/8"));
    try initial.append(std.testing.allocator, types.Ipv4Prefix.init("192.168.0.0/16"));
    defer initial.deinit(std.testing.allocator);
    export_reload_apply.initExportedPrefixes(&export_state, initial.items);

    const original_ptr = export_state.current_exported_prefixes.ptr;
    const original_len = export_state.current_exported_prefixes.len;

    const result = export_reload_apply.reloadAndApply(&export_state, "/nonexistent/path/prefixes.conf", undefined);

    try std.testing.expect(!result.reload_success);
    try std.testing.expect(result.reload_error != null);
    try std.testing.expectEqual(original_len, export_state.current_exported_prefixes.len);
    try std.testing.expectEqual(original_ptr, export_state.current_exported_prefixes.ptr);
    try std.testing.expectEqual(@as(usize, 0), result.withdrawals_sent);
}

test "reloadAndApply: non-established session skips UPDATE without crash" {
    session.MockClock.reset();
    var export_state = export_reload_apply.ExportState{};
    export_state.init(std.testing.allocator);
    defer export_state.deinit();

    var initial = std.ArrayList(types.Ipv4Prefix).empty;
    try initial.append(std.testing.allocator, types.Ipv4Prefix.init("10.0.0.0/8"));
    defer initial.deinit(std.testing.allocator);
    export_reload_apply.initExportedPrefixes(&export_state, initial.items);

    // Create temp file so we test session-not-established, not file-not-found
    const prefix_path = "/tmp/tovarisch_test_prefixes.conf";
    try linux_stats.writeFile(prefix_path, "10.0.0.0/8\n172.16.0.0/12\n");
    // Note: file in /tmp/ auto-cleaned on reboot; no explicit defer cleanup needed

    var fake = try session.FakeTransport.init(std.testing.allocator, &.{
        session.PeerResponse{ .recv_bytes = &buildPeerOpen(65002, .{ 10, 0, 0, 2 }) },
    });
    defer fake.deinit();
    const trans = fake.toTransport();
    var sess = try session.initWithClock(session.SessionConfig{
        .peer_address = .{ 10, 0, 0, 2 },
        .peer_port = 179,
        .local_address = .{ 10, 0, 0, 1 },
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{},
        .same_as = false,
    }, &trans, session.MockClock.interface());
    try std.testing.expect(!session.isEstablished(&sess));

    const result = export_reload_apply.reloadAndApply(&export_state, prefix_path, &sess);

    // Reload succeeds (file parse OK), internal state updated, but no UPDATE sent
    // because session not established. Daemon state is source of truth regardless of
    // whether peer can receive the UPDATE.
    try std.testing.expect(result.reload_success);
    try std.testing.expectEqual(@as(usize, 0), result.announcements_sent);
    try std.testing.expectEqual(@as(usize, 0), result.withdrawals_sent);
    try std.testing.expectEqual(@as(usize, 2), export_state.exportedCount()); // file had 2 prefixes
}

test "watchAndApply: watcher event triggers debounce and UPDATE after window elapsed" {
    session.MockClock.reset();
    var export_state = export_reload_apply.ExportState{};
    export_state.init(std.testing.allocator);
    defer export_state.deinit();

    // Initial prefix set
    var initial = std.ArrayList(types.Ipv4Prefix).empty;
    try initial.append(std.testing.allocator, types.Ipv4Prefix.init("10.0.0.0/8"));
    defer initial.deinit(std.testing.allocator);
    export_reload_apply.initExportedPrefixes(&export_state, initial.items);

    // Create temp file that will be "changed" by the watcher
    const prefix_path = "/tmp/tovarisch_test_watch_debounce.conf";
    try linux_stats.writeFile(prefix_path, "10.0.0.0/8\n172.16.0.0/12\n");
    // Note: file in /tmp/ auto-cleaned on reboot; no explicit defer cleanup needed

    const fake_res = try createEstablishedSession(std.testing.allocator, 65001, 65002);
    var sess = fake_res[0];
    var fake = fake_res[1];
    defer fake.deinit();

    // Create fake watcher and inject event
    var watcher = prefix_watch_fake.create(std.testing.allocator) catch unreachable;
    defer watcher.destroy();
    prefix_watch_fake.inject(watcher, .close_write);
    prefix_watch_fake.setEventPath(watcher, prefix_path);

    var debouncer = prefix_watch.Debouncer.initWithDebounce(100); // 100ms debounce

    // Step 1: t=1000 - event arrives, schedules debounce, no UPDATE yet (window not elapsed)
    const result1 = export_reload_apply.watchAndApply(
        &export_state,
        prefix_path,
        &sess,
        &watcher,
        &debouncer,
        1000, // now_ms
    );
    try std.testing.expectEqual(@as(usize, 0), result1.announcements_sent);
    try std.testing.expectEqual(@as(usize, 0), result1.withdrawals_sent);
    try std.testing.expect(debouncer.isPending());

    // Step 2: t=1099 - still within debounce window (100ms window from t=1000), no UPDATE
    const result2 = export_reload_apply.watchAndApply(
        &export_state,
        prefix_path,
        &sess,
        &watcher,
        &debouncer,
        1099, // now_ms - 99ms elapsed, still within 100ms window
    );
    try std.testing.expectEqual(@as(usize, 0), result2.announcements_sent);

    // Step 3: t=1100 - debounce window elapsed (100ms exactly), reload happens, UPDATE sent
    const result3 = export_reload_apply.watchAndApply(
        &export_state,
        prefix_path,
        &sess,
        &watcher,
        &debouncer,
        1100, // now_ms - 100ms elapsed, window has elapsed
    );
    try std.testing.expect(result3.reload_success);
    try std.testing.expectEqual(@as(usize, 1), result3.delta_added_count); // 172.16.0.0/12 added
    try std.testing.expectEqual(@as(usize, 1), result3.announcements_sent); // UPDATE sent

    // Debouncer should be cancelled after firing
    try std.testing.expect(!debouncer.isPending());

    // Step 4: t=1200 - no new events, debouncer cancelled, no reload
    const result4 = export_reload_apply.watchAndApply(
        &export_state,
        prefix_path,
        &sess,
        &watcher,
        &debouncer,
        1200,
    );
    // No reload because debouncer was cancelled after firing
    try std.testing.expectEqual(@as(usize, 0), result4.announcements_sent);
    try std.testing.expectEqual(@as(usize, 0), result4.withdrawals_sent);
}

test "watchAndApply: pending debounce fires exactly once after events" {
    session.MockClock.reset();
    var export_state = export_reload_apply.ExportState{};
    export_state.init(std.testing.allocator);
    defer export_state.deinit();

    // Initial prefix set
    var initial = std.ArrayList(types.Ipv4Prefix).empty;
    try initial.append(std.testing.allocator, types.Ipv4Prefix.init("10.0.0.0/8"));
    defer initial.deinit(std.testing.allocator);
    export_reload_apply.initExportedPrefixes(&export_state, initial.items);

    const prefix_path = "/tmp/tovarisch_test_pending_once.conf";
    try linux_stats.writeFile(prefix_path, "10.0.0.0/8\n192.168.0.0/16\n");
    // Note: file in /tmp/ auto-cleaned on reboot; no explicit defer cleanup needed

    const fake_res = try createEstablishedSession(std.testing.allocator, 65001, 65002);
    var sess = fake_res[0];
    var fake = fake_res[1];
    defer fake.deinit();

    var watcher = prefix_watch_fake.create(std.testing.allocator) catch unreachable;
    defer watcher.destroy();
    prefix_watch_fake.inject(watcher, .modify);

    var debouncer = prefix_watch.Debouncer.initWithDebounce(100);

    // First call: t=5000 - schedules debounce
    _ = export_reload_apply.watchAndApply(&export_state, prefix_path, &sess, &watcher, &debouncer, 5000);
    try std.testing.expect(debouncer.isPending());

    // Second call: t=5101 - fires pending debounce
    const first_fire = export_reload_apply.watchAndApply(&export_state, prefix_path, &sess, &watcher, &debouncer, 5101);
    try std.testing.expect(first_fire.reload_success);
    try std.testing.expect(!debouncer.isPending()); // cancelled after firing

    // Third call: t=5200 - no pending, no new events, should not reload
    const no_reload = export_reload_apply.watchAndApply(&export_state, prefix_path, &sess, &watcher, &debouncer, 5200);
    try std.testing.expectEqual(@as(usize, 0), no_reload.withdrawals_sent);
    try std.testing.expectEqual(@as(usize, 0), no_reload.announcements_sent);
}
