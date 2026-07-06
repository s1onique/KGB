#!/usr/bin/env python3
# test_verify_memory_ownership_inventory.py — Contract tests for memory ownership inventory verifier
#
# ACT-HULK29R-ZIG016-MEMOWN04-MEMORY-OWNERSHIP-INVENTORY
#
# Tests verify the verifier correctly detects:
# 1. Missing inventory file
# 2. Wrong header
# 3. Duplicate IDs
# 4. Malformed ID
# 5. Nonexistent path
# 6. Invalid kind
# 7. Invalid allocator_boundary
# 8. owned_type row missing deinit when cleanup=deinit
# 9. producer row missing errdefer
# 10. consumer row missing deinit/defer
# 11. std.testing.allocator test row when test body lacks std.testing.allocator
# 12. request_path=yes with verified=no
# 13. Produces repo-relative diagnostics
# 14. Handles current real repository inventory
#
# Run with: python3 tests/test_verify_memory_ownership_inventory.py
# Run with verbose: python3 tests/test_verify_memory_ownership_inventory.py -v

import os
import sys
import tempfile
import unittest
from pathlib import Path

# Add scripts directory to path for imports
sys.path.insert(0, str(Path(__file__).parent.parent / "scripts"))

from verify_memory_ownership_inventory import (
    check_csv_schema,
    check_source_backed_ownership,
    find_symbol,
    find_zig_deinit,
    find_errdefer,
    find_deinit_or_defer,
    has_testing_allocator,
    REPO_ROOT,
)


class TestFindSymbol(unittest.TestCase):
    """Test find_symbol function."""
    
    def test_finds_struct(self):
        """Should find struct declaration."""
        content = 'pub const OwnedWgCommandResult = struct {'
        self.assertTrue(find_symbol(content, 'OwnedWgCommandResult'))
    
    def test_finds_fn(self):
        """Should find function declaration."""
        content = 'pub fn runWgShowDump(allocator: std.mem.Allocator) !OwnedWgCommandResult {'
        self.assertTrue(find_symbol(content, 'runWgShowDump'))
    
    def test_finds_test_name(self):
        """Should find test name."""
        content = 'test "OwnedWgCommandResult deinit frees stdout stderr" {'
        self.assertTrue(find_symbol(content, 'OwnedWgCommandResult deinit frees stdout stderr'))
    
    def test_does_not_find_missing_symbol(self):
        """Should not find missing symbol."""
        content = 'pub const Foo = struct {'
        self.assertFalse(find_symbol(content, 'Bar'))


class TestFindZigDeinit(unittest.TestCase):
    """Test find_zig_deinit function."""
    
    def test_finds_deinit_in_struct(self):
        """Should find deinit method in struct."""
        content = '''pub const OwnedWgCommandResult = struct {
    stdout_storage: []u8,
    stderr_storage: []u8,
    
    pub fn deinit(self: *OwnedWgCommandResult, allocator: std.mem.Allocator) void {
        allocator.free(self.stdout_storage);
        allocator.free(self.stderr_storage);
    }
};
'''
        self.assertTrue(find_zig_deinit(content, 'OwnedWgCommandResult'))
    
    def test_missing_deinit(self):
        """Should not find deinit when missing."""
        content = '''pub const OwnedType = struct {
    stdout_storage: []u8,
};
'''
        self.assertFalse(find_zig_deinit(content, 'OwnedType'))


class TestFindErrdefer(unittest.TestCase):
    """Test find_errdefer function."""
    
    def test_finds_errdefer(self):
        """Should find errdefer in function."""
        content = '''fn runWgShowDump(allocator: std.mem.Allocator) !OwnedWgCommandResult {
    var stdout_buf = try allocator.alloc(u8, 8192);
    errdefer allocator.free(stdout_buf);
    
    var stderr_buf = try allocator.alloc(u8, 1024);
    errdefer allocator.free(stderr_buf);
    
    return OwnedWgCommandResult{...};
}
'''
        self.assertTrue(find_errdefer(content, 'runWgShowDump'))
    
    def test_missing_errdefer(self):
        """Should not find errdefer when missing."""
        content = '''fn createOwned(allocator: std.mem.Allocator) !OwnedType {
    var buf = try allocator.alloc(u8, 1024);
    // Missing errdefer
    return OwnedType{...};
}
'''
        self.assertFalse(find_errdefer(content, 'createOwned'))


class TestFindDeinitOrDefer(unittest.TestCase):
    """Test find_deinit_or_defer function."""
    
    def test_finds_defer(self):
        """Should find defer in consumer."""
        content = '''pub fn cliWireguardStatusWithRunner(
    allocator: std.mem.Allocator,
    test_path_override: ?[*:0]const u8,
    runner: WgCommandRunner,
) wg.StatusError!wg.WireGuardStatusResult {
    var cmd_result = runner.run(allocator, wg_path, CliBackend.DEFAULT_TIMEOUT_SECS) catch |err| {
        return mapCollectorError(err);
    };
    defer cmd_result.deinit(allocator);
    
    if (cmd_result.exit_code == 127) return error.backend_missing;
    ...
}
'''
        self.assertTrue(find_deinit_or_defer(content, 'cliWireguardStatusWithRunner'))
    
    def test_finds_dot_deinit(self):
        """Should find .deinit() pattern."""
        content = '''fn consumer(allocator: std.mem.Allocator) !void {
    var result = try producer(allocator);
    result.deinit(allocator);
}
'''
        self.assertTrue(find_deinit_or_defer(content, 'consumer'))
    
    def test_finds_allocator_free(self):
        """Should find allocator.free() pattern for raw slice cleanup."""
        content = '''pub fn freeTunnelSummarySnapshots(allocator: std.mem.Allocator, snapshots: []Snapshot) void {
    for (snapshots) |snap| allocator.free(snap.data);
    allocator.free(snapshots);
}
'''
        self.assertTrue(find_deinit_or_defer(content, 'freeTunnelSummarySnapshots'))


class TestHasTestingAllocator(unittest.TestCase):
    """Test has_testing_allocator function."""
    
    def test_finds_allocator(self):
        """Should find std.testing.allocator in test body."""
        content = '''test "OwnedWgCommandResult deinit frees stdout stderr" {
    const allocator = std.testing.allocator;
    
    var result = wg_cli.OwnedWgCommandResult{
        .stdout_storage = try allocator.alloc(u8, 18 * 1024),
        .stderr_storage = try allocator.alloc(u8, 1024),
        ...
    };
    result.deinit(allocator);
}
'''
        self.assertTrue(has_testing_allocator(content, 'OwnedWgCommandResult deinit frees stdout stderr'))
    
    def test_missing_allocator(self):
        """Should not find std.testing.allocator when missing."""
        content = '''test "memory leak regression" {
    try std.testing.expect(true);
}
'''
        self.assertFalse(has_testing_allocator(content, 'memory leak regression'))


class TestCheckCsvSchema(unittest.TestCase):
    """Test check_csv_schema function."""
    
    def test_missing_file(self):
        """Missing file should fail."""
        with tempfile.TemporaryDirectory() as tmpdir:
            csv_path = Path(tmpdir) / "missing.csv"
            errors = check_csv_schema(csv_path)
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("not found" in e for e in errors))
    
    def test_wrong_header(self):
        """Wrong header should fail."""
        with tempfile.TemporaryDirectory() as tmpdir:
            csv_path = Path(tmpdir) / "test.csv"
            csv_path.write_text("id,name,description\nMEMOWN-0001,test,Test\n")
            
            errors = check_csv_schema(csv_path)
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("header mismatch" in e for e in errors))
    
    def test_duplicate_ids(self):
        """Duplicate IDs should fail."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,Symbol,producer,allocates,n/a,n/a,none,n/a,no,yes,Test
MEMOWN-0001,test.zig,zig,Symbol2,producer,allocates,n/a,n/a,none,n/a,no,yes,Test
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            csv_path = Path(tmpdir) / "test.csv"
            csv_path.write_text(csv_content)
            
            errors = check_csv_schema(csv_path)
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("duplicate ID" in e for e in errors))
    
    def test_malformed_id(self):
        """Malformed ID should fail."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-1,test.zig,zig,Symbol,producer,allocates,n/a,n/a,none,n/a,no,yes,Test
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            csv_path = Path(tmpdir) / "test.csv"
            csv_path.write_text(csv_content)
            
            errors = check_csv_schema(csv_path)
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("malformed ID" in e for e in errors))
    
    def test_nonexistent_path(self):
        """Nonexistent path should fail."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,nonexistent.zig,zig,Symbol,producer,allocates,n/a,n/a,none,n/a,no,yes,Test
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            csv_path = Path(tmpdir) / "test.csv"
            csv_path.write_text(csv_content)
            
            errors = check_csv_schema(csv_path)
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("path does not exist" in e for e in errors))
    
    def test_invalid_kind(self):
        """Invalid kind should fail."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,Symbol,invalid_kind,allocates,n/a,n/a,none,n/a,no,yes,Test
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            csv_path = Path(tmpdir) / "test.csv"
            csv_path.write_text(csv_content)
            
            errors = check_csv_schema(csv_path)
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("invalid kind" in e for e in errors))
    
    def test_invalid_allocator_boundary(self):
        """Invalid allocator_boundary should fail."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,Symbol,producer,invalid_boundary,n/a,n/a,none,n/a,no,yes,Test
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            csv_path = Path(tmpdir) / "test.csv"
            csv_path.write_text(csv_content)
            
            errors = check_csv_schema(csv_path)
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("invalid allocator_boundary" in e for e in errors))
    
    def test_request_path_yes_verified_no(self):
        """request_path=yes with verified=no should fail."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,Symbol,producer,allocates,n/a,n/a,none,n/a,yes,no,Test
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            csv_path = Path(tmpdir) / "test.csv"
            csv_path.write_text(csv_content)
            
            errors = check_csv_schema(csv_path)
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("request_path=yes but verified" in e for e in errors))
    
    def test_repo_relative_diagnostics(self):
        """Diagnostics should be repo-relative."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,Symbol,producer,invalid_boundary,n/a,n/a,none,n/a,no,yes,Test
"""
        import verify_memory_ownership_inventory
        
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            csv_path = tmpdir / "test.csv"
            csv_path.write_text(csv_content)
            
            # Monkey-patch REPO_ROOT to tmpdir
            old_root = verify_memory_ownership_inventory.REPO_ROOT
            verify_memory_ownership_inventory.REPO_ROOT = tmpdir
            
            errors = check_csv_schema(csv_path)
            
            verify_memory_ownership_inventory.REPO_ROOT = old_root
            
            self.assertTrue(len(errors) > 0)
            # Should not start with absolute path
            for error in errors:
                self.assertFalse(error.startswith(str(tmpdir)))
    
    def test_valid_minimal_inventory(self):
        """Valid minimal inventory should pass."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,Symbol,producer,allocates,n/a,n/a,none,n/a,no,yes,Test
"""
        import verify_memory_ownership_inventory
        
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            csv_path = tmpdir / "test.csv"
            csv_path.write_text(csv_content)
            
            # Create the zig file that the CSV references
            zig_path = tmpdir / "test.zig"
            zig_path.write_text("// Test file\n")
            
            # Monkey-patch REPO_ROOT to tmpdir
            old_root = verify_memory_ownership_inventory.REPO_ROOT
            verify_memory_ownership_inventory.REPO_ROOT = tmpdir
            
            errors = check_csv_schema(csv_path)
            
            verify_memory_ownership_inventory.REPO_ROOT = old_root
            self.assertEqual(len(errors), 0)


class TestCheckSourceBackedOwnership(unittest.TestCase):
    """Test check_source_backed_ownership function."""
    
    def test_owned_type_without_deinit(self):
        """owned_type row with cleanup=deinit but missing deinit should fail."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,OwnedType,owned_type,consumes_owned,OwnedType,self,deinit,test,no,yes,Test
"""
        zig_content = """pub const OwnedType = struct {
    // Missing deinit method
};
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            csv_path = tmpdir / "docs/tooling/memory-ownership-inventory.csv"
            csv_path.parent.mkdir(parents=True, exist_ok=True)
            csv_path.write_text(csv_content)
            
            zig_path = tmpdir / "test.zig"
            zig_path.write_text(zig_content)
            
            # Patch REPO_ROOT
            import verify_memory_ownership_inventory
            old_root = verify_memory_ownership_inventory.REPO_ROOT
            verify_memory_ownership_inventory.REPO_ROOT = tmpdir
            
            errors = check_source_backed_ownership(csv_path)
            
            verify_memory_ownership_inventory.REPO_ROOT = old_root
            
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("lacks `fn deinit`" in e for e in errors))
    
    def test_producer_without_errdefer(self):
        """producer row with allocator_boundary=returns_owned but missing errdefer should fail."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,createOwned,producer,returns_owned,OwnedType,caller,errdefer,test,yes,yes,Test
"""
        zig_content = """const OwnedType = struct {};

fn createOwned(allocator: std.mem.Allocator) !OwnedType {
    var buf = try allocator.alloc(u8, 1024);
    // Missing errdefer
    return OwnedType{...};
}
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            csv_path = tmpdir / "docs/tooling/memory-ownership-inventory.csv"
            csv_path.parent.mkdir(parents=True, exist_ok=True)
            csv_path.write_text(csv_content)
            
            zig_path = tmpdir / "test.zig"
            zig_path.write_text(zig_content)
            
            # Patch REPO_ROOT
            import verify_memory_ownership_inventory
            old_root = verify_memory_ownership_inventory.REPO_ROOT
            verify_memory_ownership_inventory.REPO_ROOT = tmpdir
            
            errors = check_source_backed_ownership(csv_path)
            
            verify_memory_ownership_inventory.REPO_ROOT = old_root
            
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("lacks errdefer" in e for e in errors))
    
    def test_consumer_without_deinit_defer(self):
        """consumer row with allocator_boundary=consumes_owned but missing deinit/defer should fail."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,consumeOwned,consumer,consumes_owned,OwnedType,self,defer,n/a,yes,yes,Test
"""
        zig_content = """const OwnedType = struct {};

fn consumeOwned(allocator: std.mem.Allocator) !void {
    var result = try createOwned(allocator);
    // Missing defer or deinit
    _ = result;
}
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            csv_path = tmpdir / "docs/tooling/memory-ownership-inventory.csv"
            csv_path.parent.mkdir(parents=True, exist_ok=True)
            csv_path.write_text(csv_content)
            
            zig_path = tmpdir / "test.zig"
            zig_path.write_text(zig_content)
            
            # Patch REPO_ROOT
            import verify_memory_ownership_inventory
            old_root = verify_memory_ownership_inventory.REPO_ROOT
            verify_memory_ownership_inventory.REPO_ROOT = tmpdir
            
            errors = check_source_backed_ownership(csv_path)
            
            verify_memory_ownership_inventory.REPO_ROOT = old_root
            
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("lacks `.deinit(`" in e for e in errors))
    
    def test_test_without_std_testing_allocator(self):
        """test row with cleanup=std.testing.allocator but missing std.testing.allocator in body should fail."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,memory leak test,test,verifies,OwnedType,test,std.testing.allocator,test,no,yes,Test
"""
        zig_content = """const OwnedType = struct {};

test "memory leak test" {
    // Missing std.testing.allocator
    try std.testing.expect(true);
}
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            csv_path = tmpdir / "docs/tooling/memory-ownership-inventory.csv"
            csv_path.parent.mkdir(parents=True, exist_ok=True)
            csv_path.write_text(csv_content)
            
            zig_path = tmpdir / "test.zig"
            zig_path.write_text(zig_content)
            
            # Patch REPO_ROOT
            import verify_memory_ownership_inventory
            old_root = verify_memory_ownership_inventory.REPO_ROOT
            verify_memory_ownership_inventory.REPO_ROOT = tmpdir
            
            errors = check_source_backed_ownership(csv_path)
            
            verify_memory_ownership_inventory.REPO_ROOT = old_root
            
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("lacks std.testing.allocator" in e for e in errors))


class TestRealRepositoryInventory(unittest.TestCase):
    """Integration test that verifies the verifier passes on the real repo inventory."""
    
    def test_real_inventory_passes(self):
        """The verifier should pass on the real repository inventory."""
        csv_path = REPO_ROOT / "docs/tooling/memory-ownership-inventory.csv"
        
        if not csv_path.exists():
            self.skipTest("Real inventory file does not exist")
        
        schema_errors = check_csv_schema(csv_path)
        source_errors = check_source_backed_ownership(csv_path)
        all_errors = schema_errors + source_errors
        
        self.assertEqual(
            len(all_errors), 0,
            f"Real inventory has errors:\n" + "\n".join(all_errors)
        )


class TestVerifierSelfTest(unittest.TestCase):
    """Integration test that verifies the verifier's internal self-test passes."""
    
    def test_verifier_self_test_passes(self):
        """The verifier's internal self-test should pass."""
        import subprocess
        result = subprocess.run(
            [sys.executable, str(Path(__file__).parent.parent / "scripts/verify_memory_ownership_inventory.py"), "--self-test"],
            capture_output=True,
            text=True,
            cwd=str(REPO_ROOT)
        )
        
        self.assertEqual(result.returncode, 0, 
            f"Self-test failed:\nstdout: {result.stdout}\nstderr: {result.stderr}")


class TestAllocationFreeRows(unittest.TestCase):
    """Test allocation-free request_path rows (MEMOWN06)."""
    
    def test_allocation_free_row_passes_with_review_note(self):
        """Allocation-free request_path row passes when notes contain 'Inventory reviewed'."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,buildBgpCheckInto,producer,none,n/a,self,n/a,n/a,yes,yes,Inventory reviewed: BGP collector returns value-only status
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            csv_path = tmpdir / "docs/tooling/memory-ownership-inventory.csv"
            csv_path.parent.mkdir(parents=True, exist_ok=True)
            csv_path.write_text(csv_content)
            
            zig_path = tmpdir / "test.zig"
            zig_path.write_text("// Test file\n")
            
            import verify_memory_ownership_inventory
            old_root = verify_memory_ownership_inventory.REPO_ROOT
            verify_memory_ownership_inventory.REPO_ROOT = tmpdir
            
            errors = check_source_backed_ownership(csv_path)
            
            verify_memory_ownership_inventory.REPO_ROOT = old_root
            
            # Should pass - has review note
            self.assertEqual(len(errors), 0)
    
    def test_allocation_free_row_passes_with_value_only_note(self):
        """Allocation-free request_path row passes when notes contain 'value-only'."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,snapshotFromRuntime,producer,none,n/a,self,n/a,n/a,yes,yes,Returns value-only status snapshot
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            csv_path = tmpdir / "docs/tooling/memory-ownership-inventory.csv"
            csv_path.parent.mkdir(parents=True, exist_ok=True)
            csv_path.write_text(csv_content)
            
            zig_path = tmpdir / "test.zig"
            zig_path.write_text("// Test file\n")
            
            import verify_memory_ownership_inventory
            old_root = verify_memory_ownership_inventory.REPO_ROOT
            verify_memory_ownership_inventory.REPO_ROOT = tmpdir
            
            errors = check_source_backed_ownership(csv_path)
            
            verify_memory_ownership_inventory.REPO_ROOT = old_root
            
            # Should pass - has value-only note
            self.assertEqual(len(errors), 0)
    
    def test_allocation_free_row_fails_without_explanation(self):
        """Allocation-free request_path row fails when notes are empty."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,someFunction,producer,none,n/a,self,n/a,n/a,yes,yes,
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            csv_path = tmpdir / "docs/tooling/memory-ownership-inventory.csv"
            csv_path.parent.mkdir(parents=True, exist_ok=True)
            csv_path.write_text(csv_content)
            
            zig_path = tmpdir / "test.zig"
            zig_path.write_text("// Test file\n")
            
            import verify_memory_ownership_inventory
            old_root = verify_memory_ownership_inventory.REPO_ROOT
            verify_memory_ownership_inventory.REPO_ROOT = tmpdir
            
            errors = check_source_backed_ownership(csv_path)
            
            verify_memory_ownership_inventory.REPO_ROOT = old_root
            
            # Should fail - no review note
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("Inventory reviewed" in e for e in errors))


class TestAllocatorFreeCleanup(unittest.TestCase):
    """Test allocator.free cleanup detection (MEMOWN06)."""
    
    def test_consumer_with_allocator_free_passes(self):
        """Consumer row passes when nearby cleanup uses allocator.free."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,freeInterfaceStatsSnapshots,consumer,consumes_owned,InterfaceStatsSnapshot,self,allocator.free,n/a,yes,yes,Frees interface stats
"""
        zig_content = """pub fn freeInterfaceStatsSnapshots(
    allocator: std.mem.Allocator,
    snapshots: []InterfaceStatsSnapshot,
) void {
    for (snapshots) |snap| allocator.free(snap.name);
    allocator.free(snapshots);
}
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            csv_path = tmpdir / "docs/tooling/memory-ownership-inventory.csv"
            csv_path.parent.mkdir(parents=True, exist_ok=True)
            csv_path.write_text(csv_content)
            
            zig_path = tmpdir / "test.zig"
            zig_path.write_text(zig_content)
            
            import verify_memory_ownership_inventory
            old_root = verify_memory_ownership_inventory.REPO_ROOT
            verify_memory_ownership_inventory.REPO_ROOT = tmpdir
            
            errors = check_source_backed_ownership(csv_path)
            
            verify_memory_ownership_inventory.REPO_ROOT = old_root
            
            # Should pass - has allocator.free
            self.assertEqual(len(errors), 0)


class TestCoverageReferences(unittest.TestCase):
    """Test coverage reference validation (MEMOWN06)."""
    
    def test_coverage_in_source_file_passes(self):
        """Coverage string found in source file passes."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,SomeFunc,producer,allocates,n/a,n/a,none,SomeFunc test,no,yes,Test
"""
        zig_content = """fn SomeFunc() void {}

// Test for SomeFunc
test "SomeFunc test" {
    SomeFunc();
}
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            csv_path = tmpdir / "docs/tooling/memory-ownership-inventory.csv"
            csv_path.parent.mkdir(parents=True, exist_ok=True)
            csv_path.write_text(csv_content)
            
            zig_path = tmpdir / "test.zig"
            zig_path.write_text(zig_content)
            
            import verify_memory_ownership_inventory
            old_root = verify_memory_ownership_inventory.REPO_ROOT
            verify_memory_ownership_inventory.REPO_ROOT = tmpdir
            
            errors = check_source_backed_ownership(csv_path)
            
            verify_memory_ownership_inventory.REPO_ROOT = old_root
            
            # Should pass - coverage found in source
            self.assertEqual(len(errors), 0)
    
    def test_coverage_in_zig_tests_passes(self):
        """Coverage string found in Zig test file passes."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,SomeFunc,producer,allocates,n/a,n/a,none,SomeFunc coverage,no,yes,Test
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            csv_path = tmpdir / "docs/tooling/memory-ownership-inventory.csv"
            csv_path.parent.mkdir(parents=True, exist_ok=True)
            csv_path.write_text(csv_content)
            
            # Source file
            zig_path = tmpdir / "test.zig"
            zig_path.write_text("fn SomeFunc() void {}\n")
            
            # Test file
            tests_dir = tmpdir / "tovarisch/src"
            tests_dir.mkdir(parents=True)
            test_path = tests_dir / "test_coverage_tests.zig"
            test_path.write_text('test "SomeFunc coverage" { SomeFunc(); }\n')
            
            import verify_memory_ownership_inventory
            old_root = verify_memory_ownership_inventory.REPO_ROOT
            verify_memory_ownership_inventory.REPO_ROOT = tmpdir
            
            errors = check_source_backed_ownership(csv_path)
            
            verify_memory_ownership_inventory.REPO_ROOT = old_root
            
            # Should pass - coverage found in Zig test file
            self.assertEqual(len(errors), 0)
    
    def test_coverage_in_python_tests_passes(self):
        """Coverage string found in Python test file passes."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,SomeFunc,producer,allocates,n/a,n/a,none,SomeFunc py coverage,no,yes,Test
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            csv_path = tmpdir / "docs/tooling/memory-ownership-inventory.csv"
            csv_path.parent.mkdir(parents=True, exist_ok=True)
            csv_path.write_text(csv_content)
            
            # Source file
            zig_path = tmpdir / "test.zig"
            zig_path.write_text("fn SomeFunc() void {}\n")
            
            # Python test file
            tests_dir = tmpdir / "tests"
            tests_dir.mkdir(parents=True)
            test_path = tests_dir / "test_coverage.py"
            test_path.write_text("# SomeFunc py coverage\ndef test_something():\n    pass\n")
            
            import verify_memory_ownership_inventory
            old_root = verify_memory_ownership_inventory.REPO_ROOT
            verify_memory_ownership_inventory.REPO_ROOT = tmpdir
            
            errors = check_source_backed_ownership(csv_path)
            
            verify_memory_ownership_inventory.REPO_ROOT = old_root
            
            # Should pass - coverage found in Python test file
            self.assertEqual(len(errors), 0)
    
    def test_nonexistent_coverage_fails(self):
        """Coverage string not found anywhere fails."""
        csv_content = """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,SomeFunc,producer,allocates,n/a,n/a,none,CompletelyFakeCoverage,no,yes,Test
"""
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            csv_path = tmpdir / "docs/tooling/memory-ownership-inventory.csv"
            csv_path.parent.mkdir(parents=True, exist_ok=True)
            csv_path.write_text(csv_content)
            
            zig_path = tmpdir / "test.zig"
            zig_path.write_text("fn SomeFunc() void {}\n")
            
            import verify_memory_ownership_inventory
            old_root = verify_memory_ownership_inventory.REPO_ROOT
            verify_memory_ownership_inventory.REPO_ROOT = tmpdir
            
            errors = check_source_backed_ownership(csv_path)
            
            verify_memory_ownership_inventory.REPO_ROOT = old_root
            
            # Should fail - coverage not found
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("CompletelyFakeCoverage" in e for e in errors))


if __name__ == "__main__":
    unittest.main()
