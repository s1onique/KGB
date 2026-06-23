// Diagnostic Timeline Row Anchoring Tests - DOM regression tests for details panel placement
// 
// These tests verify that the expanded details panel is rendered immediately after
// the selected event row, not at the bottom of the table.
// 
// Bug: Previously, details were rendered in a separate div outside the table,
// causing visual detachment from the selected row.

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { renderTimelineTable } from './diagnosticTimeline/view';
import { createTimelineStateWithEvents } from './diagnosticTimeline.fixtures';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// Mock the api module
vi.mock('./api', () => ({
  api: {
    getLatencySpikesWithCaptures: vi.fn(),
  },
}));

describe('diagnosticTimeline row anchoring', () => {
  let container: HTMLElement;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
  });

  afterEach(() => {
    container.remove();
  });

  describe('renderTimelineTable with inline details rows', () => {
    it('renders details row immediately after the event row in DOM order', () => {
      const state = createTimelineStateWithEvents([
        { eventId: 'evt-1', probeKind: 'http' },
        { eventId: 'evt-2', probeKind: 'icmp' },
        { eventId: 'evt-3', probeKind: 'http' },
      ]);
      
      const html = renderTimelineTable(state.mergedEvents.slice(0, 3), new Set());
      container.innerHTML = html;
      
      const table = container.querySelector('table.timeline-table');
      expect(table).toBeTruthy();
      
      const tbody = table!.querySelector('tbody');
      expect(tbody).toBeTruthy();
      
      const rows = tbody!.querySelectorAll('tr');
      
      // Should have 6 rows: 3 event rows + 3 details rows
      expect(rows.length).toBe(6);
      
      // Row 0: Event row for evt-1
      expect(rows[0].classList.contains('timeline-row')).toBe(true);
      expect(rows[0].getAttribute('data-row-index')).toBe('0');
      
      // Row 1: Details row for evt-1 (immediately after)
      expect(rows[1].classList.contains('timeline-details-row')).toBe(true);
      expect(rows[1].getAttribute('data-details-for')).toBe('evt-1');
      
      // Row 2: Event row for evt-2
      expect(rows[2].classList.contains('timeline-row')).toBe(true);
      expect(rows[2].getAttribute('data-row-index')).toBe('1');
      
      // Row 3: Details row for evt-2 (immediately after)
      expect(rows[3].classList.contains('timeline-details-row')).toBe(true);
      expect(rows[3].getAttribute('data-details-for')).toBe('evt-2');
      
      // Row 4: Event row for evt-3
      expect(rows[4].classList.contains('timeline-row')).toBe(true);
      expect(rows[4].getAttribute('data-row-index')).toBe('2');
      
      // Row 5: Details row for evt-3 (immediately after)
      expect(rows[5].classList.contains('timeline-details-row')).toBe(true);
      expect(rows[5].getAttribute('data-details-for')).toBe('evt-3');
    });

    it('renders details row with correct colspan for full-width display', () => {
      const state = createTimelineStateWithEvents([
        { eventId: 'evt-1', probeKind: 'http' },
      ]);
      
      const html = renderTimelineTable(state.mergedEvents.slice(0, 1), new Set());
      container.innerHTML = html;
      
      const detailsRow = container.querySelector('.timeline-details-row');
      expect(detailsRow).toBeTruthy();
      
      const detailsCell = detailsRow!.querySelector('td');
      expect(detailsCell).toBeTruthy();
      // colspan="7" should span all visible columns (Time, Probe, Severity, Latency, Capture, Details, Action)
      expect(detailsCell!.getAttribute('colspan')).toBe('7');
    });

    it('marks expanded details rows with display:none by default', () => {
      const state = createTimelineStateWithEvents([
        { eventId: 'evt-1', probeKind: 'http' },
        { eventId: 'evt-2', probeKind: 'icmp' },
      ]);
      
      const html = renderTimelineTable(state.mergedEvents.slice(0, 2), new Set());
      container.innerHTML = html;
      
      const detailsRows = container.querySelectorAll('.timeline-details-row');
      
      // Both should be hidden by default
      expect(detailsRows[0].style.display).toBe('none');
      expect(detailsRows[1].style.display).toBe('none');
    });

    it('renders expanded details rows as visible when in expandedEventIds', () => {
      const state = createTimelineStateWithEvents([
        { eventId: 'evt-1', probeKind: 'http' },
        { eventId: 'evt-2', probeKind: 'icmp' },
        { eventId: 'evt-3', probeKind: 'http' },
      ]);
      
      const expandedIds = new Set(['evt-2']);
      const html = renderTimelineTable(state.mergedEvents.slice(0, 3), expandedIds);
      container.innerHTML = html;
      
      const detailsRows = container.querySelectorAll('.timeline-details-row');
      
      // evt-1 details should be hidden
      expect(detailsRows[0].style.display).toBe('none');
      
      // evt-2 details should be visible (expanded)
      expect(detailsRows[1].style.display).toBe('');
      
      // evt-3 details should be hidden
      expect(detailsRows[2].style.display).toBe('none');
    });

    it('handles mixed HTTP/ICMP table with correct row anchoring', () => {
      const state = createTimelineStateWithEvents([
        { eventId: 'http-evt', probeKind: 'http' },
        { eventId: 'icmp-evt', probeKind: 'icmp' },
      ]);
      
      const expandedIds = new Set(['icmp-evt']);
      const html = renderTimelineTable(state.mergedEvents.slice(0, 2), expandedIds);
      container.innerHTML = html;
      
      const tbody = container.querySelector('tbody')!;
      const rows = tbody.querySelectorAll('tr');
      
      // HTTP event row
      expect(rows[0].classList.contains('timeline-row')).toBe(true);
      expect(rows[0].querySelector('.probe-badge')?.classList.contains('probe-http')).toBe(true);
      
      // HTTP details row (hidden)
      expect(rows[1].classList.contains('timeline-details-row')).toBe(true);
      expect(rows[1].getAttribute('data-details-for')).toBe('http-evt');
      expect(rows[1].style.display).toBe('none');
      
      // ICMP event row
      expect(rows[2].classList.contains('timeline-row')).toBe(true);
      expect(rows[2].querySelector('.probe-badge')?.classList.contains('probe-icmp')).toBe(true);
      
      // ICMP details row (visible)
      expect(rows[3].classList.contains('timeline-details-row')).toBe(true);
      expect(rows[3].getAttribute('data-details-for')).toBe('icmp-evt');
      expect(rows[3].style.display).toBe('');
    });

    it('has no external .timeline-expanded-panels div (details are inline)', () => {
      const state = createTimelineStateWithEvents([
        { eventId: 'evt-1', probeKind: 'http' },
      ]);
      
      const html = renderTimelineTable(state.mergedEvents.slice(0, 1), new Set());
      container.innerHTML = html;
      
      // The old external panels div should NOT exist
      const externalPanels = container.querySelector('.timeline-expanded-panels');
      expect(externalPanels).toBeNull();
      
      // Details should be inside the table tbody
      const detailsRow = container.querySelector('.timeline-details-row');
      expect(detailsRow).toBeTruthy();
    });

    it('details row contains action buttons (Copy JSON, Download)', () => {
      const state = createTimelineStateWithEvents([
        { eventId: 'evt-1', probeKind: 'http' },
      ]);
      
      const expandedIds = new Set(['evt-1']);
      const html = renderTimelineTable(state.mergedEvents.slice(0, 1), expandedIds);
      container.innerHTML = html;
      
      const detailsRow = container.querySelector('.timeline-details-row');
      expect(detailsRow).toBeTruthy();
      
      // Should contain action buttons
      const copyBtn = detailsRow!.querySelector('.copy-btn');
      expect(copyBtn).toBeTruthy();
      expect(copyBtn!.getAttribute('data-event-id')).toBe('evt-1');
      
      const downloadBtn = detailsRow!.querySelector('.download-btn');
      expect(downloadBtn).toBeTruthy();
      expect(downloadBtn!.getAttribute('data-event-id')).toBe('evt-1');
    });

    it('renders correctly with only ICMP events', () => {
      const state = createTimelineStateWithEvents([
        { eventId: 'icmp-1', probeKind: 'icmp' },
        { eventId: 'icmp-2', probeKind: 'icmp' },
      ]);
      
      const html = renderTimelineTable(state.mergedEvents.slice(0, 2), new Set());
      container.innerHTML = html;
      
      const tbody = container.querySelector('tbody')!;
      const rows = tbody.querySelectorAll('tr');
      
      // 4 rows: 2 event + 2 details
      expect(rows.length).toBe(4);
      
      // All event rows should be ICMP
      expect(rows[0].querySelector('.probe-badge')?.classList.contains('probe-icmp')).toBe(true);
      expect(rows[2].querySelector('.probe-badge')?.classList.contains('probe-icmp')).toBe(true);
    });

    it('has correct DOM structure: event row followed by details row', () => {
      const state = createTimelineStateWithEvents([
        { eventId: 'test-evt', probeKind: 'http' },
      ]);
      
      const expandedIds = new Set(['test-evt']);
      const html = renderTimelineTable(state.mergedEvents.slice(0, 1), expandedIds);
      container.innerHTML = html;
      
      const tbody = container.querySelector('tbody')!;
      const rows = Array.from(tbody.querySelectorAll('tr'));
      
      // Exactly 2 rows
      expect(rows.length).toBe(2);
      
      // First is event row
      expect(rows[0].classList.contains('timeline-row')).toBe(true);
      expect(rows[0].getAttribute('data-row-index')).toBe('0');
      
      // Second is details row, immediately after
      expect(rows[1].classList.contains('timeline-details-row')).toBe(true);
      expect(rows[1].getAttribute('data-details-for')).toBe('test-evt');
      
      // Details row is visible (expanded)
      expect(rows[1].style.display).not.toBe('none');
    });
  });

  describe('CSS contract: visibility ownership', () => {
    it('does not hide inline expanded panel content; the table row owns visibility', () => {
      // Regression test: .timeline-expanded-panel must NOT have display:none
      // because the row (.timeline-details-row) now owns visibility via inline style.
      // Bug: Previously, the panel had display:none, making content invisible
      // even when the row was expanded.
      const css = readFileSync(resolve(__dirname, 'styles.css'), 'utf8');
      
      const panelRuleMatch = css.match(/\.timeline-expanded-panel\s*\{[^}]*\}/);
      expect(panelRuleMatch).toBeTruthy();
      
      const panelRule = panelRuleMatch![0];
      // The panel rule must not contain display:none
      expect(panelRule).not.toMatch(/display\s*:\s*none/);
    });

    it('expanded details row contains visible metadata content when expanded', () => {
      const state = createTimelineStateWithEvents([
        { eventId: 'evt-visible', probeKind: 'http', latencyMs: 500, rollingMedianMs: 100 },
      ]);
      
      const expandedIds = new Set(['evt-visible']);
      const html = renderTimelineTable(state.mergedEvents.slice(0, 1), expandedIds);
      container.innerHTML = html;
      
      const detailsRow = container.querySelector('.timeline-details-row');
      expect(detailsRow).toBeTruthy();
      
      // Row should be visible (not display:none)
      expect(detailsRow!.style.display).not.toBe('none');
      
      // Panel content should be present and visible
      const panel = detailsRow!.querySelector('.timeline-expanded-panel');
      expect(panel).toBeTruthy();
      
      // Metadata content should be present
      const metadata = panel!.querySelector('.event-metadata');
      expect(metadata).toBeTruthy();
      
      // Should contain event ID
      const eventId = panel!.querySelector('.event-id');
      expect(eventId).toBeTruthy();
      expect(eventId!.textContent).toBe('evt-visible');
      
      // Should contain action buttons
      const copyBtn = panel!.querySelector('.copy-btn');
      const downloadBtn = panel!.querySelector('.download-btn');
      expect(copyBtn).toBeTruthy();
      expect(downloadBtn).toBeTruthy();
    });
  });
});
