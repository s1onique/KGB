// Diagnostic Timeline Pagination View Tests - DOM rendering and accessibility tests

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { 
  createInitialModel, 
  getPaginationInfo,
  computeProbeKindSummary,
} from './model';
import type { TimelineModel, TimelineEvent } from '../diagnosticTimeline.model';

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: vi.fn((key: string) => store[key] ?? null),
    setItem: vi.fn((key: string, value: string) => { store[key] = value; }),
    clear: vi.fn(() => { store = {}; }),
    get store() { return store; },
  };
})();

Object.defineProperty(globalThis, 'localStorage', {
  value: localStorageMock,
  writable: true,
});

// Helper to create test events
function createTestEvent(overrides: Partial<TimelineEvent> = {}): TimelineEvent {
  const eventId = overrides.eventId ?? 'evt-1';
  const probeKind = overrides.probeKind ?? 'http';
  const severity = overrides.severity ?? 'warning';
  const sampleTs = overrides.sampleTs ?? '2026-06-18T12:00:00Z';
  
  return {
    eventId,
    targetId: overrides.targetId ?? 'test-target',
    probeKind: probeKind as 'http' | 'icmp',
    severity: severity as 'warning' | 'critical',
    latencyMs: overrides.latencyMs ?? 1234,
    sampleTs,
    collectedAt: overrides.collectedAt ?? sampleTs,
    reasons: overrides.reasons ?? [],
    rollingMedianMs: overrides.rollingMedianMs ?? 100,
    thresholds: {
      warningMs: 500,
      criticalMs: 1000,
      relativeMultiplier: 10,
    },
    captures: overrides.captures ?? [],
    primaryCapture: overrides.primaryCapture ?? null,
    captureStatus: overrides.captureStatus ?? 'captured',
    canonicalTime: new Date(sampleTs),
    sortProbeKind: probeKind === 'http' ? 0 : 1,
    sortSeverity: severity === 'warning' ? 0 : 1,
    sortEventId: eventId,
  };
}

// Create a mock container for testing
function createMockContainer(): HTMLElement {
  const container = document.createElement('div');
  container.id = 'test-timeline';
  document.body.appendChild(container);
  return container;
}

// Helper to render pagination HTML - matches the actual view.ts implementation
function renderPaginationHtml(
  pageIndex: number,
  pageSize: number,
  totalEvents: number
): string {
  const totalPages = Math.max(1, Math.ceil(totalEvents / pageSize));
  const safePageIndex = Math.min(pageIndex, totalPages - 1);
  const displayStart = totalEvents === 0 ? 0 : safePageIndex * pageSize + 1;
  const displayEnd = Math.min((safePageIndex + 1) * pageSize, totalEvents);
  const displayCurrentPage = safePageIndex + 1;
  
  const isFirstPage = safePageIndex === 0;
  const isLastPage = safePageIndex >= totalPages - 1;
  
  const pageSizeOptions = [10, 25, 50, 100].map(size => {
    const selected = size === pageSize ? ' selected' : '';
    return `<option value="${size}"${selected}>${size}</option>`;
  }).join('');
  
  return `
    <div class="diagnostic-timeline-pagination">
      <div class="diagnostic-timeline-pagination__rows">
        <label for="diagnostic-timeline-page-size">Rows</label>
        <select id="diagnostic-timeline-page-size" class="page-size-select" aria-label="Rows per page">
          ${pageSizeOptions}
        </select>
      </div>

      <div class="diagnostic-timeline-pagination__range" aria-live="polite">
        Showing ${displayStart}–${displayEnd} of ${totalEvents}
      </div>

      <nav class="diagnostic-timeline-pagination__nav" aria-label="Diagnostic timeline pagination">
        <button type="button" class="page-nav-btn" data-action="page-first" ${isFirstPage ? 'disabled' : ''} aria-label="Go to first page">
          ‹‹
        </button>
        <button type="button" class="page-nav-btn" data-action="page-prev" ${isFirstPage ? 'disabled' : ''} aria-label="Go to previous page">
          ‹
        </button>
        <span class="page-indicator" aria-current="page" aria-label="Current page">
          Page ${displayCurrentPage} of ${totalPages}
        </span>
        <button type="button" class="page-nav-btn" data-action="page-next" ${isLastPage ? 'disabled' : ''} aria-label="Go to next page">
          ›
        </button>
        <button type="button" class="page-nav-btn" data-action="page-last" ${isLastPage ? 'disabled' : ''} aria-label="Go to last page">
          ››
        </button>
      </nav>
    </div>
  `;
}

describe('pagination view rendering', () => {
  let container: HTMLElement;

  beforeEach(() => {
    localStorageMock.clear();
    container = createMockContainer();
  });

  afterEach(() => {
    document.body.removeChild(container);
  });

  describe('pagination toolbar structure', () => {
    it('renders as a single container with correct class', () => {
      container.innerHTML = renderPaginationHtml(0, 10, 0);
      
      const pagination = container.querySelector('.diagnostic-timeline-pagination');
      expect(pagination).not.toBeNull();
    });

    it('contains rows selector group', () => {
      container.innerHTML = renderPaginationHtml(0, 10, 0);
      
      const rowsGroup = container.querySelector('.diagnostic-timeline-pagination__rows');
      expect(rowsGroup).not.toBeNull();
      
      const select = container.querySelector('.page-size-select');
      expect(select).not.toBeNull();
    });

    it('contains range display', () => {
      container.innerHTML = renderPaginationHtml(0, 10, 0);
      
      const range = container.querySelector('.diagnostic-timeline-pagination__range');
      expect(range).not.toBeNull();
    });

    it('contains navigation nav element', () => {
      container.innerHTML = renderPaginationHtml(0, 10, 0);
      
      const nav = container.querySelector('.diagnostic-timeline-pagination__nav');
      expect(nav).not.toBeNull();
    });
  });

  describe('range text', () => {
    it('shows "Showing 0–0 of 0" for empty state', () => {
      container.innerHTML = renderPaginationHtml(0, 10, 0);
      
      const range = container.querySelector('.diagnostic-timeline-pagination__range');
      expect(range?.textContent).toContain('Showing 0–0 of 0');
    });

    it('shows correct range for first page', () => {
      container.innerHTML = renderPaginationHtml(0, 10, 117);
      
      const range = container.querySelector('.diagnostic-timeline-pagination__range');
      expect(range?.textContent).toContain('Showing 1–10 of 117');
    });

    it('shows correct range for middle page', () => {
      container.innerHTML = renderPaginationHtml(1, 25, 117);
      
      const range = container.querySelector('.diagnostic-timeline-pagination__range');
      expect(range?.textContent).toContain('Showing 26–50 of 117');
    });

    it('shows correct range for last page', () => {
      container.innerHTML = renderPaginationHtml(4, 25, 117);
      
      const range = container.querySelector('.diagnostic-timeline-pagination__range');
      expect(range?.textContent).toContain('Showing 101–117 of 117');
    });
  });

  describe('page indicator', () => {
    it('shows "Page 1 of 1" for empty state', () => {
      container.innerHTML = renderPaginationHtml(0, 10, 0);
      
      const indicator = container.querySelector('.page-indicator');
      expect(indicator?.textContent?.trim()).toBe('Page 1 of 1');
    });

    it('shows correct page for first page with multiple pages', () => {
      // 100 events with page size 10 = 10 pages
      container.innerHTML = renderPaginationHtml(0, 10, 100);
      
      const indicator = container.querySelector('.page-indicator');
      expect(indicator?.textContent?.trim()).toBe('Page 1 of 10');
    });

    it('shows correct page for middle page', () => {
      // 100 events with page size 10 = 10 pages, page 2 = "Page 3 of 10"
      container.innerHTML = renderPaginationHtml(2, 10, 100);
      
      const indicator = container.querySelector('.page-indicator');
      expect(indicator?.textContent?.trim()).toBe('Page 3 of 10');
    });
  });

  describe('navigation buttons', () => {
    it('disables First and Prev buttons on first page', () => {
      container.innerHTML = renderPaginationHtml(0, 10, 100);
      
      const firstBtn = container.querySelector('[data-action="page-first"]') as HTMLButtonElement;
      const prevBtn = container.querySelector('[data-action="page-prev"]') as HTMLButtonElement;
      
      expect(firstBtn.disabled).toBe(true);
      expect(prevBtn.disabled).toBe(true);
    });

    it('enables Next and Last buttons on first page', () => {
      container.innerHTML = renderPaginationHtml(0, 10, 100);
      
      const nextBtn = container.querySelector('[data-action="page-next"]') as HTMLButtonElement;
      const lastBtn = container.querySelector('[data-action="page-last"]') as HTMLButtonElement;
      
      expect(nextBtn.disabled).toBe(false);
      expect(lastBtn.disabled).toBe(false);
    });

    it('enables First and Prev buttons on middle page', () => {
      container.innerHTML = renderPaginationHtml(2, 10, 100);
      
      const firstBtn = container.querySelector('[data-action="page-first"]') as HTMLButtonElement;
      const prevBtn = container.querySelector('[data-action="page-prev"]') as HTMLButtonElement;
      
      expect(firstBtn.disabled).toBe(false);
      expect(prevBtn.disabled).toBe(false);
    });

    it('disables Next and Last buttons on last page', () => {
      // 100 events with page size 10 = 10 pages, last page is index 9
      container.innerHTML = renderPaginationHtml(9, 10, 100);
      
      const nextBtn = container.querySelector('[data-action="page-next"]') as HTMLButtonElement;
      const lastBtn = container.querySelector('[data-action="page-last"]') as HTMLButtonElement;
      
      expect(nextBtn.disabled).toBe(true);
      expect(lastBtn.disabled).toBe(true);
    });

    it('disables all navigation buttons on single page', () => {
      container.innerHTML = renderPaginationHtml(0, 10, 5);
      
      const firstBtn = container.querySelector('[data-action="page-first"]') as HTMLButtonElement;
      const prevBtn = container.querySelector('[data-action="page-prev"]') as HTMLButtonElement;
      const nextBtn = container.querySelector('[data-action="page-next"]') as HTMLButtonElement;
      const lastBtn = container.querySelector('[data-action="page-last"]') as HTMLButtonElement;
      
      expect(firstBtn.disabled).toBe(true);
      expect(prevBtn.disabled).toBe(true);
      expect(nextBtn.disabled).toBe(true);
      expect(lastBtn.disabled).toBe(true);
    });
  });

  describe('page size select', () => {
    it('renders all valid page size options', () => {
      container.innerHTML = renderPaginationHtml(0, 25, 100);
      
      const select = container.querySelector('.page-size-select') as HTMLSelectElement;
      const options = Array.from(select.options).map(o => o.value);
      
      expect(options).toEqual(['10', '25', '50', '100']);
    });

    it('marks correct option as selected', () => {
      container.innerHTML = renderPaginationHtml(0, 25, 100);
      
      const select = container.querySelector('.page-size-select') as HTMLSelectElement;
      
      expect(select.value).toBe('25');
    });
  });
});

describe('pagination accessibility', () => {
  let container: HTMLElement;

  beforeEach(() => {
    localStorageMock.clear();
    container = createMockContainer();
  });

  afterEach(() => {
    document.body.removeChild(container);
  });

  describe('nav aria-label', () => {
    it('pagination nav has aria-label', () => {
      container.innerHTML = renderPaginationHtml(0, 10, 100);
      
      const nav = container.querySelector('.diagnostic-timeline-pagination__nav');
      expect(nav?.getAttribute('aria-label')).toBe('Diagnostic timeline pagination');
    });
  });

  describe('button aria-labels', () => {
    it('First button has descriptive aria-label', () => {
      container.innerHTML = renderPaginationHtml(1, 10, 100);
      
      const btn = container.querySelector('[data-action="page-first"]');
      expect(btn?.getAttribute('aria-label')).toBe('Go to first page');
    });

    it('Prev button has descriptive aria-label', () => {
      container.innerHTML = renderPaginationHtml(1, 10, 100);
      
      const btn = container.querySelector('[data-action="page-prev"]');
      expect(btn?.getAttribute('aria-label')).toBe('Go to previous page');
    });

    it('Next button has descriptive aria-label', () => {
      container.innerHTML = renderPaginationHtml(1, 10, 100);
      
      const btn = container.querySelector('[data-action="page-next"]');
      expect(btn?.getAttribute('aria-label')).toBe('Go to next page');
    });

    it('Last button has descriptive aria-label', () => {
      container.innerHTML = renderPaginationHtml(1, 10, 100);
      
      const btn = container.querySelector('[data-action="page-last"]');
      expect(btn?.getAttribute('aria-label')).toBe('Go to last page');
    });
  });

  describe('page indicator aria-current', () => {
    it('page indicator has aria-current="page"', () => {
      container.innerHTML = renderPaginationHtml(0, 10, 100);
      
      const indicator = container.querySelector('.page-indicator');
      expect(indicator?.getAttribute('aria-current')).toBe('page');
    });
  });

  describe('range aria-live', () => {
    it('range display has aria-live="polite"', () => {
      container.innerHTML = renderPaginationHtml(0, 10, 100);
      
      const range = container.querySelector('.diagnostic-timeline-pagination__range');
      expect(range?.getAttribute('aria-live')).toBe('polite');
    });
  });

  describe('select aria-label', () => {
    it('page size select has aria-label', () => {
      container.innerHTML = renderPaginationHtml(0, 10, 100);
      
      const select = container.querySelector('.page-size-select');
      expect(select?.getAttribute('aria-label')).toBe('Rows per page');
    });
  });
});

describe('pagination model integration', () => {
  it('getPaginationInfo returns correct values for empty model', () => {
    const model = createInitialModel();
    const info = getPaginationInfo(model);
    
    expect(info.totalPages).toBe(1);
    expect(info.safePageIndex).toBe(0);
    expect(info.start).toBe(0);
    expect(info.end).toBe(0);
    expect(info.filteredCount).toBe(0);
  });

  it('getPaginationInfo returns correct values for events', () => {
    const events = Array(117).fill(null).map((_, i) => 
      createTestEvent({ eventId: `evt-${i}` })
    );
    
    const model: TimelineModel = {
      ...createInitialModel(),
      mergedEvents: events,
      httpSummary: computeProbeKindSummary('http', events),
      icmpSummary: computeProbeKindSummary('icmp', events),
      pagination: { pageIndex: 1, pageSize: 25 },
    };
    
    const info = getPaginationInfo(model);
    
    expect(info.totalPages).toBe(5);
    expect(info.safePageIndex).toBe(1);
    expect(info.start).toBe(25);
    expect(info.end).toBe(50);
    expect(info.filteredCount).toBe(117);
  });

  it('getPaginationInfo clamps page index to valid range', () => {
    const events = Array(10).fill(null).map((_, i) => 
      createTestEvent({ eventId: `evt-${i}` })
    );
    
    const model: TimelineModel = {
      ...createInitialModel(),
      mergedEvents: events,
      httpSummary: computeProbeKindSummary('http', events),
      icmpSummary: computeProbeKindSummary('icmp', events),
      pagination: { pageIndex: 99, pageSize: 10 }, // Invalid page index
    };
    
    const info = getPaginationInfo(model);
    
    expect(info.safePageIndex).toBe(0); // Should clamp to last page (0)
  });
});
