// Diagnostic Timeline Pagination Tests - Pure pagination utility tests
// Tests the exported pagination utilities from diagnosticTimeline module

import { describe, it, expect } from 'vitest';
import {
  defaultPagination,
  calculatePagination,
  clampPageIndex,
  getPageIndexForRow,
  getFirstVisibleRow,
  PAGE_SIZE_OPTIONS,
} from './diagnosticTimeline';

// ---------------------------------------------------------------------------
// Pagination Utility Tests
// ---------------------------------------------------------------------------

describe('calculatePagination', () => {
  it('returns correct values for default pagination', () => {
    const pagination = { pageIndex: 0, pageSize: 20 };
    const result = calculatePagination(100, pagination);
    
    expect(result.totalPages).toBe(5);
    expect(result.safePageIndex).toBe(0);
    expect(result.start).toBe(0);
    expect(result.end).toBe(20);
  });

  it('clamps page index to valid range', () => {
    const pagination = { pageIndex: 10, pageSize: 20 };
    const result = calculatePagination(100, pagination);
    
    expect(result.safePageIndex).toBe(4); // Last valid page (index 4 for 5 pages)
    expect(result.start).toBe(80);
    expect(result.end).toBe(100);
  });

  it('handles zero filtered count', () => {
    const pagination = { pageIndex: 0, pageSize: 20 };
    const result = calculatePagination(0, pagination);
    
    expect(result.totalPages).toBe(1);
    expect(result.safePageIndex).toBe(0);
    expect(result.start).toBe(0);
    expect(result.end).toBe(0);
  });

  it('handles partial last page', () => {
    const pagination = { pageIndex: 2, pageSize: 20 };
    const result = calculatePagination(45, pagination);
    
    expect(result.totalPages).toBe(3);
    expect(result.safePageIndex).toBe(2);
    expect(result.start).toBe(40);
    expect(result.end).toBe(45);
  });
});

describe('clampPageIndex', () => {
  it('returns 0 for negative index', () => {
    expect(clampPageIndex(-1, 5)).toBe(0);
  });

  it('returns 0 for zero total pages', () => {
    expect(clampPageIndex(0, 0)).toBe(0);
  });

  it('clamps index exceeding total pages', () => {
    expect(clampPageIndex(10, 5)).toBe(4);
  });

  it('returns valid index within range', () => {
    expect(clampPageIndex(2, 5)).toBe(2);
  });
});

describe('getPageIndexForRow', () => {
  it('returns correct page index for row', () => {
    expect(getPageIndexForRow(0, 20)).toBe(0);
    expect(getPageIndexForRow(15, 20)).toBe(0);
    expect(getPageIndexForRow(20, 20)).toBe(1);
    expect(getPageIndexForRow(45, 20)).toBe(2);
  });

  it('handles page size of 10', () => {
    expect(getPageIndexForRow(0, 10)).toBe(0);
    expect(getPageIndexForRow(9, 10)).toBe(0);
    expect(getPageIndexForRow(10, 10)).toBe(1);
    expect(getPageIndexForRow(25, 10)).toBe(2);
  });

  it('handles page size of 50', () => {
    expect(getPageIndexForRow(0, 50)).toBe(0);
    expect(getPageIndexForRow(49, 50)).toBe(0);
    expect(getPageIndexForRow(50, 50)).toBe(1);
    expect(getPageIndexForRow(125, 50)).toBe(2);
  });

  it('handles page size of 100', () => {
    expect(getPageIndexForRow(0, 100)).toBe(0);
    expect(getPageIndexForRow(99, 100)).toBe(0);
    expect(getPageIndexForRow(100, 100)).toBe(1);
    expect(getPageIndexForRow(250, 100)).toBe(2);
  });
});

describe('getFirstVisibleRow', () => {
  it('returns correct first visible row', () => {
    expect(getFirstVisibleRow(0, 20)).toBe(0);
    expect(getFirstVisibleRow(1, 20)).toBe(20);
    expect(getFirstVisibleRow(2, 20)).toBe(40);
  });

  it('handles page size of 10', () => {
    expect(getFirstVisibleRow(0, 10)).toBe(0);
    expect(getFirstVisibleRow(1, 10)).toBe(10);
    expect(getFirstVisibleRow(5, 10)).toBe(50);
  });

  it('handles page size of 50', () => {
    expect(getFirstVisibleRow(0, 50)).toBe(0);
    expect(getFirstVisibleRow(1, 50)).toBe(50);
    expect(getFirstVisibleRow(2, 50)).toBe(100);
  });

  it('handles page size of 100', () => {
    expect(getFirstVisibleRow(0, 100)).toBe(0);
    expect(getFirstVisibleRow(1, 100)).toBe(100);
    expect(getFirstVisibleRow(3, 100)).toBe(300);
  });
});

describe('PAGE_SIZE_OPTIONS', () => {
  it('contains expected page size options', () => {
    expect(PAGE_SIZE_OPTIONS).toEqual([10, 25, 50, 100]);
  });

  it('is a readonly tuple', () => {
    expect(PAGE_SIZE_OPTIONS.length).toBe(4);
  });
});

describe('defaultPagination', () => {
  it('has correct default values', () => {
    expect(defaultPagination.pageIndex).toBe(0);
    expect(defaultPagination.pageSize).toBe(10);
  });

  it('has page size from PAGE_SIZE_OPTIONS', () => {
    expect(PAGE_SIZE_OPTIONS).toContain(defaultPagination.pageSize);
  });
});

// ---------------------------------------------------------------------------
// Pagination Boundary Tests
// ---------------------------------------------------------------------------

describe('pagination boundary conditions', () => {
  it('single page displays all rows', () => {
    const pagination = { pageIndex: 0, pageSize: 20 };
    const result = calculatePagination(15, pagination);
    
    expect(result.totalPages).toBe(1);
    expect(result.safePageIndex).toBe(0);
    expect(result.start).toBe(0);
    expect(result.end).toBe(15);
  });

  it('exact page boundary', () => {
    const pagination = { pageIndex: 1, pageSize: 20 };
    const result = calculatePagination(40, pagination);
    
    expect(result.totalPages).toBe(2);
    expect(result.safePageIndex).toBe(1);
    expect(result.start).toBe(20);
    expect(result.end).toBe(40);
  });

  it('large dataset', () => {
    const pagination = { pageIndex: 99, pageSize: 100 };
    const result = calculatePagination(10000, pagination);
    
    expect(result.totalPages).toBe(100);
    expect(result.safePageIndex).toBe(99);
    expect(result.start).toBe(9900);
    expect(result.end).toBe(10000);
  });

  it('very small page size', () => {
    const pagination = { pageIndex: 0, pageSize: 10 };
    const result = calculatePagination(5, pagination);
    
    expect(result.totalPages).toBe(1);
    expect(result.start).toBe(0);
    expect(result.end).toBe(5);
  });

});

// ---------------------------------------------------------------------------
// Row-to-Page Navigation Tests
// ---------------------------------------------------------------------------

describe('row-to-page navigation', () => {
  it('first row is always on page 0', () => {
    expect(getPageIndexForRow(0, 20)).toBe(0);
    expect(getPageIndexForRow(0, 10)).toBe(0);
    expect(getPageIndexForRow(0, 50)).toBe(0);
  });

  it('last row determines last page', () => {
    // For 100 rows with page size 20, last row (99) is on page 4
    expect(getPageIndexForRow(99, 20)).toBe(4);
    expect(getFirstVisibleRow(4, 20)).toBe(80); // First row on page 4
  });

  it('middle rows on middle pages', () => {
    // Row 45 with page size 20: page 2 (rows 40-59)
    expect(getPageIndexForRow(45, 20)).toBe(2);
    expect(getFirstVisibleRow(2, 20)).toBe(40);
  });
});
