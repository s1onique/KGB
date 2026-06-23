// Diagnostic Timeline View - Rendering logic (no business logic, no state mutations)

import type { TimelineModel } from './model';
import { PAGE_SIZE_OPTIONS, getFilteredEvents, getPagedEvents, getPaginationInfo } from './model';
import type { TimelineEvent, ProbeKindSummary } from '../diagnosticTimeline.model';
import { formatSpikeTime, formatLatencyMs } from '../format';
import { formatLocalDateTime, parseApiInstant } from '../time';

// ---------------------------------------------------------------------------
// XSS Protection
// ---------------------------------------------------------------------------

/** HTML escape helper for XSS protection */
function escapeText(s: string | null | undefined): string {
  if (!s) return '';
  const div = document.createElement('div');
  div.textContent = s;
  return div.innerHTML;
}

/** HTML attribute escape helper - escapes text for safe use in HTML attributes */
function escapeAttr(s: string | null | undefined): string {
  if (!s) return '';
  const str = String(s);
  return str.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

// ---------------------------------------------------------------------------
// Defense-in-Depth Display Helpers
// ---------------------------------------------------------------------------

/**
 * Safe uppercase label helper - ensures .toUpperCase() never throws.
 * Defense-in-depth for render code that may receive unexpected values.
 */
function upperLabel(value: unknown, fallback = 'UNKNOWN'): string {
  if (typeof value === 'string' && value.trim() !== '') {
    return value.toUpperCase();
  }
  return fallback;
}

// ---------------------------------------------------------------------------
// CSS Class Helpers
// ---------------------------------------------------------------------------

/** Get CSS class for probe kind */
function getProbeKindClass(probeKind: string): string {
  return probeKind === 'http' ? 'probe-http' : 'probe-icmp';
}

/** Get CSS class for severity badge */
function getSeverityClass(severity: string): string {
  return severity === 'critical' ? 'severity-critical' : 'severity-warn';
}

/** Get CSS class for capture status badge */
function getCaptureStatusClass(status: string): string {
  switch (status) {
    case 'captured': return 'capture-captured';
    case 'suppressed': return 'capture-suppressed';
    case 'failed': return 'capture-failed';
    case 'not_attempted': return 'capture-muted';
    default: return 'capture-muted';
  }
}

/** Get label for capture status */
function getCaptureStatusLabel(status: string): string {
  switch (status) {
    case 'captured': return 'captured';
    case 'suppressed': return 'suppressed';
    case 'failed': return 'failed';
    case 'not_attempted': return 'not attempted';
    default: return status;
  }
}

// ---------------------------------------------------------------------------
// Timestamp Formatting
// ---------------------------------------------------------------------------

/** Format timestamp for display using local timezone */
function formatTimelineTime(timestamp: string | undefined): string {
  const date = parseApiInstant(timestamp ?? null);
  if (!date) return '—';
  return formatLocalDateTime(timestamp);
}

// ---------------------------------------------------------------------------
// Summary Card Rendering
// ---------------------------------------------------------------------------

/** Render a single probe kind summary card */
function renderSummaryCard(summary: ProbeKindSummary): string {
  const { probeKind, totalEvents, capturedCount, suppressedCount, failedCount, criticalCount, warningCount } = summary;
  
  const label = probeKind === 'http' ? 'HTTP' : 'ICMP';
  const className = getProbeKindClass(probeKind);
  
  return `
    <div class="timeline-summary-card ${className}">
      <div class="timeline-summary-header">
        <span class="timeline-summary-label">${label}</span>
        <span class="timeline-summary-total">${totalEvents} events</span>
      </div>
      <div class="timeline-summary-stats">
        <div class="timeline-summary-stat">
          <span class="stat-captured">${capturedCount}</span>
          <span class="stat-label">captured</span>
        </div>
        <div class="timeline-summary-stat">
          <span class="stat-suppressed">${suppressedCount}</span>
          <span class="stat-label">suppressed</span>
        </div>
        <div class="timeline-summary-stat">
          <span class="stat-failed">${failedCount}</span>
          <span class="stat-label">failed</span>
        </div>
        <div class="timeline-summary-stat">
          <span class="stat-critical">${criticalCount}</span>
          <span class="stat-label">critical</span>
        </div>
        <div class="timeline-summary-stat">
          <span class="stat-warning">${warningCount}</span>
          <span class="stat-label">warning</span>
        </div>
      </div>
    </div>
  `;
}

/** Render both summary cards */
function renderSummaryCards(httpSummary: ProbeKindSummary, icmpSummary: ProbeKindSummary): string {
  return `
    <div class="timeline-summary-container">
      ${renderSummaryCard(httpSummary)}
      ${renderSummaryCard(icmpSummary)}
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Filter Controls Rendering
// ---------------------------------------------------------------------------

/** Render filter controls */
function renderFilterControls(filters: TimelineModel['filters'], totalCount: number, filteredCount: number): string {
  const hasActiveFilters = filters.probeKind !== 'all' || filters.captureStatus !== 'all' || filters.severity !== 'all';
  
  return `
    <div class="timeline-filters">
      <div class="filter-group">
        <label class="filter-label">Probe:</label>
        <select class="filter-select" data-filter="probeKind">
          <option value="all" ${filters.probeKind === 'all' ? 'selected' : ''}>All</option>
          <option value="http" ${filters.probeKind === 'http' ? 'selected' : ''}>HTTP</option>
          <option value="icmp" ${filters.probeKind === 'icmp' ? 'selected' : ''}>ICMP</option>
        </select>
      </div>
      <div class="filter-group">
        <label class="filter-label">Status:</label>
        <select class="filter-select" data-filter="captureStatus">
          <option value="all" ${filters.captureStatus === 'all' ? 'selected' : ''}>All</option>
          <option value="captured" ${filters.captureStatus === 'captured' ? 'selected' : ''}>Captured</option>
          <option value="suppressed" ${filters.captureStatus === 'suppressed' ? 'selected' : ''}>Suppressed</option>
          <option value="failed" ${filters.captureStatus === 'failed' ? 'selected' : ''}>Failed</option>
        </select>
      </div>
      <div class="filter-group">
        <label class="filter-label">Severity:</label>
        <select class="filter-select" data-filter="severity">
          <option value="all" ${filters.severity === 'all' ? 'selected' : ''}>All</option>
          <option value="warning" ${filters.severity === 'warning' ? 'selected' : ''}>Warning</option>
          <option value="critical" ${filters.severity === 'critical' ? 'selected' : ''}>Critical</option>
        </select>
      </div>
      <div class="filter-count">
        Showing ${filteredCount} of ${totalCount}
      </div>
      ${hasActiveFilters ? '<button class="filter-reset-btn" data-action="reset-filters">Clear filters</button>' : ''}
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Pagination Rendering
// ---------------------------------------------------------------------------

/** Render compact page number buttons with ellipsis for many pages */
function renderPageNumbers(currentPage: number, totalPages: number): string {
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, i) => {
      const pageNum = i;
      const isCurrent = pageNum === currentPage;
      return `<button class="page-num-btn ${isCurrent ? 'active' : ''}" data-page-number="${pageNum}" ${isCurrent ? 'aria-current="page"' : ''}>${pageNum + 1}</button>`;
    }).join('');
  }
  
  const pages: (number | 'ellipsis')[] = [];
  
  if (currentPage < 4) {
    for (let i = 0; i < 4; i++) pages.push(i);
    pages.push('ellipsis');
    pages.push(totalPages - 1);
  } else if (currentPage > totalPages - 5) {
    pages.push(0);
    pages.push('ellipsis');
    for (let i = totalPages - 4; i < totalPages; i++) pages.push(i);
  } else {
    pages.push(0);
    pages.push('ellipsis');
    pages.push(currentPage - 1);
    pages.push(currentPage);
    pages.push(currentPage + 1);
    pages.push('ellipsis');
    pages.push(totalPages - 1);
  }
  
  return pages.map(p => {
    if (p === 'ellipsis') {
      return '<span class="page-ellipsis">…</span>';
    }
    const isCurrent = p === currentPage;
    return `<button class="page-num-btn ${isCurrent ? 'active' : ''}" data-page-number="${p}" ${isCurrent ? 'aria-current="page"' : ''}>${p + 1}</button>`;
  }).join('');
}

/** Render pagination controls */
function renderPaginationControls(
  pagination: TimelineModel['pagination'],
  totalPages: number,
  safePageIndex: number,
  filteredCount: number
): string {
  const displayStart = filteredCount === 0 ? 0 : safePageIndex * pagination.pageSize + 1;
  const displayEnd = Math.min((safePageIndex + 1) * pagination.pageSize, filteredCount);
  const displayCurrentPage = safePageIndex + 1;
  
  const isFirstPage = safePageIndex === 0;
  const isLastPage = safePageIndex >= totalPages - 1;
  
  const pageSizeOptions = PAGE_SIZE_OPTIONS.map(size => {
    const selected = size === pagination.pageSize ? ' selected' : '';
    return `<option value="${size}"${selected}>${size}</option>`;
  }).join('');
  
  return `
    <nav class="timeline-pagination" aria-label="Timeline pagination">
      <div class="pagination-left">
        <label class="pagination-label">
          Rows:
          <select class="page-size-select" aria-label="Rows per page">
            ${pageSizeOptions}
          </select>
        </label>
      </div>
      <div class="pagination-center">
        <span class="showing-text" aria-live="polite">
          Showing ${displayStart}–${displayEnd} of ${filteredCount}
        </span>
      </div>
      <div class="pagination-right">
        <button class="page-nav-btn" data-action="page-first" ${isFirstPage ? 'disabled' : ''} aria-label="First page">
          « First
        </button>
        <button class="page-nav-btn" data-action="page-prev" ${isFirstPage ? 'disabled' : ''} aria-label="Previous page">
          ‹ Prev
        </button>
        <span class="page-indicator" aria-label="Current page">
          Page ${displayCurrentPage} of ${totalPages}
        </span>
        ${totalPages > 1 ? `
          <div class="page-numbers">
            ${renderPageNumbers(safePageIndex, totalPages)}
          </div>
        ` : ''}
        <button class="page-nav-btn" data-action="page-next" ${isLastPage ? 'disabled' : ''} aria-label="Next page">
          Next ›
        </button>
        <button class="page-nav-btn" data-action="page-last" ${isLastPage ? 'disabled' : ''} aria-label="Last page">
          Last »
        </button>
      </div>
    </nav>
  `;
}

// ---------------------------------------------------------------------------
// Cross-Probe Suppression
// ---------------------------------------------------------------------------

/** Render cross-probe suppression explanation */
function renderCrossProbeSuppression(event: TimelineEvent): string {
  const capture = event.primaryCapture;
  if (!capture?.cooldown_info) return '';
  
  const info = capture.cooldown_info;
  if (!info.is_cross_probe_suppression) return '';
  
  // Use upperLabel for defense-in-depth on cooldown info fields
  const anchorKind = upperLabel(info.anchor_probe_kind, 'UNKNOWN');
  const suppressedKind = upperLabel(info.suppressed_probe_kind ?? event.probeKind, 'UNKNOWN');
  
  return `
    <div class="cross-probe-explanation">
      <span class="cross-probe-text">${suppressedKind} spike suppressed by prior ${anchorKind} diagnostic capture due to source-level cooldown.</span>
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Expanded Panel Rendering
// ---------------------------------------------------------------------------

/** Render expanded evidence panel for an event */
function renderExpandedPanel(event: TimelineEvent, rowIndex: number): string {
  const detailsId = `timeline-details-${rowIndex}`;
  
  let html = `<div class="timeline-expanded-panel" id="${detailsId}">`;
  
  html += '<div class="event-metadata">';
  html += `<div class="detail-row"><span class="detail-label">Event ID:</span><span class="detail-value event-id">${escapeAttr(event.eventId)}</span></div>`;
  html += `<div class="detail-row"><span class="detail-label">Sample time:</span><span class="detail-value">${formatTimelineTime(event.sampleTs)}</span></div>`;
  html += `<div class="detail-row"><span class="detail-label">Collected at:</span><span class="detail-value">${formatTimelineTime(event.collectedAt)}</span></div>`;
  html += `<div class="detail-row"><span class="detail-label">Latency:</span><span class="detail-value latency-value">${formatLatencyMs(event.latencyMs)}</span></div>`;
  html += `<div class="detail-row"><span class="detail-label">Rolling median:</span><span class="detail-value">${event.rollingMedianMs} ms</span></div>`;
  
  if (event.reasons.length > 0) {
    html += `<div class="detail-row"><span class="detail-label">Reasons:</span><span class="detail-value">${escapeText(event.reasons.join(', '))}</span></div>`;
  }
  
  html += '</div>';
  
  html += renderCrossProbeSuppression(event);
  
  // Action buttons - use escapeAttr for data attributes
  html += '<div class="timeline-actions">';
  html += `<button class="timeline-action-btn copy-btn" data-event-id="${escapeAttr(event.eventId)}">Copy JSON</button>`;
  html += `<button class="timeline-action-btn download-btn" data-event-id="${escapeAttr(event.eventId)}">Download</button>`;
  html += '</div>';
  
  html += '</div>';
  return html;
}

// ---------------------------------------------------------------------------
// Timeline Row Rendering
// ---------------------------------------------------------------------------

/** Render a single timeline row */
function renderTimelineRow(event: TimelineEvent, rowIndex: number): string {
  const time = formatSpikeTime(event.sampleTs);
  const latency = formatLatencyMs(event.latencyMs);
  const probeKindClass = getProbeKindClass(event.probeKind);
  const severityClass = getSeverityClass(event.severity);
  const captureStatusClass = getCaptureStatusClass(event.captureStatus);
  const captureStatusLabel = getCaptureStatusLabel(event.captureStatus);
  
  let detailsText = '—';
  if (event.captureStatus === 'failed' && event.primaryCapture?.error) {
    const error = event.primaryCapture.error;
    detailsText = error.length > 60 ? error.substring(0, 57) + '…' : error;
  }
  
  const detailsId = `timeline-details-${rowIndex}`;
  
  // Use upperLabel for defense-in-depth - ensures .toUpperCase() never throws
  const probeKindLabel = upperLabel(event.probeKind, 'HTTP');
  const severityLabel = upperLabel(event.severity, 'WARNING');
  
  return `
    <tr class="timeline-row" data-row-index="${rowIndex}">
      <td class="timeline-cell time-cell">${time}</td>
      <td class="timeline-cell probe-cell"><span class="probe-badge ${probeKindClass}">${probeKindLabel}</span></td>
      <td class="timeline-cell severity-cell"><span class="severity-badge ${severityClass}">${severityLabel}</span></td>
      <td class="timeline-cell latency-cell">${latency}</td>
      <td class="timeline-cell capture-cell"><span class="capture-badge ${captureStatusClass}">${captureStatusLabel}</span></td>
      <td class="timeline-cell details-cell">${escapeText(detailsText)}</td>
      <td class="timeline-cell action-cell">
        <button class="timeline-details-btn" data-details-id="${detailsId}" data-row-index="${rowIndex}" data-event-id="${escapeAttr(event.eventId)}">Details</button>
      </td>
    </tr>
  `;
}

// ---------------------------------------------------------------------------
// Table Rendering
// ---------------------------------------------------------------------------

/** Render the complete timeline table */
function renderTimelineTable(events: TimelineEvent[]): string {
  if (events.length === 0) {
    return '<div class="timeline-empty">No diagnostic events in the selected range.</div>';
  }
  
  const rows = events.map((event, index) => renderTimelineRow(event, index)).join('');
  const expandedPanels = events.map((event, index) => renderExpandedPanel(event, index)).join('');
  
  return `
    <table class="timeline-table" aria-label="Diagnostic timeline">
      <thead>
        <tr>
          <th scope="col">Time</th>
          <th scope="col">Probe</th>
          <th scope="col">Severity</th>
          <th scope="col">Latency</th>
          <th scope="col">Capture</th>
          <th scope="col">Details</th>
          <th scope="col"></th>
        </tr>
      </thead>
      <tbody>
        ${rows}
      </tbody>
    </table>
    <div class="timeline-expanded-panels">
      ${expandedPanels}
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Loading and Error States
// ---------------------------------------------------------------------------

/** Render loading state */
function renderLoadingState(): string {
  return `
    <div class="timeline-loading">
      <div class="spinner"></div>
      <span>Loading diagnostic timeline...</span>
    </div>
  `;
}

/** Render error state */
function renderErrorState(error: string): string {
  return `
    <div class="timeline-error">
      <span class="error-icon">⚠</span>
      <span class="error-text">${escapeText(error)}</span>
    </div>
  `;
}

/** Render empty state */
function renderEmptyState(): string {
  return `
    <div class="timeline-empty-state">
      <div class="empty-icon">📭</div>
      <div class="empty-title">No Diagnostic Events</div>
      <div class="empty-text">No HTTP or ICMP spike diagnostics have been recorded for this target.</div>
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Main Render Function
// ---------------------------------------------------------------------------

/** Render the complete timeline component */
export function renderTimeline(container: HTMLElement, model: TimelineModel): void {
  const { isLoading, error, httpSummary, icmpSummary, mergedEvents, filters, pagination } = model;
  
  // Loading state
  if (isLoading) {
    container.innerHTML = `
      <div class="timeline-header">
        <span class="timeline-title">Diagnostic Timeline</span>
      </div>
      ${renderLoadingState()}
    `;
    return;
  }
  
  // Error state
  if (error) {
    container.innerHTML = `
      <div class="timeline-header">
        <span class="timeline-title">Diagnostic Timeline</span>
      </div>
      ${renderErrorState(error)}
    `;
    return;
  }
  
  const filteredEvents = getFilteredEvents(model);
  const pagedEvents = getPagedEvents(model);
  const paginationInfo = getPaginationInfo(model);
  
  const summaryCards = renderSummaryCards(httpSummary, icmpSummary);
  const filterControls = renderFilterControls(filters, mergedEvents.length, paginationInfo.filteredCount);
  const paginationControls = renderPaginationControls(
    pagination,
    paginationInfo.totalPages,
    paginationInfo.safePageIndex,
    paginationInfo.filteredCount
  );
  
  const timelineContent = filteredEvents.length === 0 && mergedEvents.length === 0
    ? renderEmptyState()
    : renderTimelineTable(pagedEvents);
  
  container.innerHTML = `
    <div class="timeline-header">
      <span class="timeline-title">Diagnostic Timeline</span>
    </div>
    ${summaryCards}
    ${filterControls}
    ${paginationControls}
    ${timelineContent}
  `;
}

/** Render the loading/error shell only */
export function renderShell(container: HTMLElement, isLoading: boolean, error: string | null): void {
  if (isLoading) {
    container.innerHTML = `
      <div class="timeline-header">
        <span class="timeline-title">Diagnostic Timeline</span>
      </div>
      ${renderLoadingState()}
    `;
    return;
  }
  
  if (error) {
    container.innerHTML = `
      <div class="timeline-header">
        <span class="timeline-title">Diagnostic Timeline</span>
      </div>
      ${renderErrorState(error)}
    `;
  }
}
