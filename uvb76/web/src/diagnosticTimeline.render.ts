// Diagnostic Timeline Render - Render summary cards, filter controls, table rows, and expanded evidence panels
import type { TimelineEvent, ProbeKindSummary, TimelineState } from './diagnosticTimeline.model';
import type { TimelineFilters } from './diagnosticTimeline.filters';
import type { TimelinePageState } from './diagnosticTimeline';
import { PAGE_SIZE_OPTIONS } from './diagnosticTimeline';
import { formatSpikeTime, formatLatencyMs } from './format';
import { formatLocalDateTime, parseApiInstant, formatRemainingCooldown } from './time';
import { getCooldownInfo, isSuppressionDegraded, getDegradedReason, getAnchorEventSummary, getAnchorVisibilityReason, isPinnedAnchor, getTcpQualitySourceLabel, isNativeTcpQuality, isSyntheticTcpQuality, isSsTcpQuality, isTcpQualityUnavailable } from './diagnosticTimeline.model';

// ---------------------------------------------------------------------------
// Pagination Types
// ---------------------------------------------------------------------------

/** Pagination context passed to the render function */
export interface PaginationContext {
  pagination: TimelinePageState;
  totalPages: number;
  safePageIndex: number;
  filteredCount: number;
}

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
  const str = String(s ?? '');
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

/** Get CSS class for degraded suppression badge */
function getDegradedBadgeClass(status: string, isDegraded: boolean): string {
  if (!isDegraded) return getCaptureStatusClass(status);
  return 'capture-degraded';
}

/** Get label for degraded suppression badge */
function getDegradedBadgeLabel(status: string, isDegraded: boolean): string {
  if (!isDegraded) return getCaptureStatusLabel(status);
  return 'suppressed (degraded)';
}

// ---------------------------------------------------------------------------
// Summary Card Rendering
// ---------------------------------------------------------------------------

/** Render a single probe kind summary card */
export function renderSummaryCard(summary: ProbeKindSummary): string {
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
export function renderSummaryCards(httpSummary: ProbeKindSummary, icmpSummary: ProbeKindSummary): string {
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
export function renderFilterControls(filters: TimelineFilters, totalCount: number, filteredCount: number): string {
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
function renderPageNumbers(
  currentPage: number,
  totalPages: number
): string {
  if (totalPages <= 7) {
    // Show all pages if 7 or fewer
    return Array.from({ length: totalPages }, (_, i) => {
      const pageNum = i;
      const isCurrent = pageNum === currentPage;
      return `<button class="page-num-btn ${isCurrent ? 'active' : ''}" data-page-number="${pageNum}" ${isCurrent ? 'aria-current="page"' : ''}>${pageNum + 1}</button>`;
    }).join('');
  }
  
  // For many pages, show a sliding window
  const pages: (number | 'ellipsis')[] = [];
  
  if (currentPage < 4) {
    // Near the start: show 1-5, ellipsis, last
    for (let i = 0; i < 4; i++) pages.push(i);
    pages.push('ellipsis');
    pages.push(totalPages - 1);
  } else if (currentPage > totalPages - 5) {
    // Near the end: show first, ellipsis, last 5
    pages.push(0);
    pages.push('ellipsis');
    for (let i = totalPages - 4; i < totalPages; i++) pages.push(i);
  } else {
    // In the middle: show first, ellipsis, current-1, current, current+1, ellipsis, last
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
export function renderPaginationControls(
  pagination: TimelinePageState,
  totalPages: number,
  safePageIndex: number,
  filteredCount: number
): string {
  // Calculate showing range (1-based for display)
  const displayStart = filteredCount === 0 ? 0 : safePageIndex * pagination.pageSize + 1;
  const displayEnd = Math.min((safePageIndex + 1) * pagination.pageSize, filteredCount);
  const displayCurrentPage = safePageIndex + 1;
  
  // Determine button disabled states
  const isFirstPage = safePageIndex === 0;
  const isLastPage = safePageIndex >= totalPages - 1;
  
  // Page size selector options
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
// Timestamp Formatting
// ---------------------------------------------------------------------------

/** Format timestamp for display using local timezone */
function formatTimelineTime(timestamp: string | undefined): string {
  const date = parseApiInstant(timestamp ?? null);
  if (!date) return '—';
  return formatLocalDateTime(timestamp);
}

// ---------------------------------------------------------------------------
// Cross-Probe Suppression Wording
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
// Cooldown Details
// ---------------------------------------------------------------------------

/** Render cooldown details section */
function renderCooldownDetails(event: TimelineEvent): string {
  const capture = event.primaryCapture;
  if (!capture?.cooldown_info) return '';
  
  const info = capture.cooldown_info;
  const anchorKind = escapeText(info.anchor_probe_kind || '');
  const suppressedKind = escapeText(info.suppressed_probe_kind || '');
  const isCrossProbe = info.is_cross_probe_suppression === true;
  const anchorCaptureId = escapeText(info.anchor_capture_id || '');
  const scope = escapeText(info.scope || '');
  const cooldownKey = escapeText(info.cooldown_key || '');
  
  let html = '<div class="cooldown-details">';
  html += '<div class="cooldown-section-title">Cooldown Information</div>';
  
  if (isCrossProbe) {
    html += `<div class="cooldown-row"><span class="cooldown-label">Anchor probe:</span><span class="cooldown-value">${anchorKind}</span></div>`;
    html += `<div class="cooldown-row"><span class="cooldown-label">Suppressed probe:</span><span class="cooldown-value">${suppressedKind}</span></div>`;
  }
  
  if (info.last_successful_capture_at) {
    html += `<div class="cooldown-row"><span class="cooldown-label">Anchor capture:</span><span class="cooldown-value">${formatTimelineTime(info.last_successful_capture_at)}</span></div>`;
  }
  
  if (anchorCaptureId) {
    html += `<div class="cooldown-row"><span class="cooldown-label">Anchor capture ID:</span><span class="cooldown-value capture-id">${anchorCaptureId}</span></div>`;
  }
  
  if (scope) {
    html += `<div class="cooldown-row"><span class="cooldown-label">Scope:</span><span class="cooldown-value">${scope}</span></div>`;
  }
  
  if (cooldownKey) {
    html += `<div class="cooldown-row"><span class="cooldown-label">Cooldown key:</span><span class="cooldown-value">${cooldownKey}</span></div>`;
  }
  
  if (info.next_capture_eligible_at) {
    html += `<div class="cooldown-row"><span class="cooldown-label">Next eligible:</span><span class="cooldown-value">${formatTimelineTime(info.next_capture_eligible_at)}</span></div>`;
  }
  
  if (info.remaining_cooldown_ms !== undefined && info.remaining_cooldown_ms > 0) {
    const formatted = formatRemainingCooldown(info.remaining_cooldown_ms);
    html += `<div class="cooldown-row"><span class="cooldown-label">Remaining cooldown:</span><span class="cooldown-value">${formatted}</span></div>`;
  }
  
  if (!info.anchor_visible && info.anchor_visibility_reason) {
    let reasonText = 'outside current view';
    if (info.anchor_visibility_reason === 'evicted_from_retention') {
      reasonText = 'anchor evicted from retention';
    } else if (info.anchor_visibility_reason === 'suppressed_cooldown') {
      reasonText = 'anchor also suppressed by cooldown';
    }
    html += `<div class="cooldown-row"><span class="cooldown-label">Anchor:</span><span class="cooldown-value anchor-hidden">${reasonText}</span></div>`;
  }
  
  html += '</div>';
  return html;
}

// ---------------------------------------------------------------------------
// Capture Details
// ---------------------------------------------------------------------------

/** Render capture details section */
function renderCaptureDetailsSection(event: TimelineEvent): string {
  const capture = event.primaryCapture;
  if (!capture) return '';
  
  const source = escapeText(capture.source);
  const duration = capture.duration_ms !== undefined ? ` (${capture.duration_ms}ms)` : '';
  const statusClass = getCaptureStatusClass(event.captureStatus);
  const statusLabel = getCaptureStatusLabel(event.captureStatus);
  
  let html = '<div class="capture-details-section">';
  html += '<div class="capture-section-title">Capture Details</div>';
  html += `<div class="capture-row-header"><span class="capture-source-label">${source}${duration}</span><span class="capture-status-badge ${statusClass}">${statusLabel}</span></div>`;
  
  html += `<div class="detail-row"><span class="detail-label">Started:</span><span class="detail-value">${formatTimelineTime(capture.capture_started_at)}</span></div>`;
  
  if (capture.capture_finished_at) {
    html += `<div class="detail-row"><span class="detail-label">Finished:</span><span class="detail-value">${formatTimelineTime(capture.capture_finished_at)}</span></div>`;
  }
  
  if (capture.duration_ms !== undefined) {
    html += `<div class="detail-row"><span class="detail-label">Duration:</span><span class="detail-value">${capture.duration_ms} ms</span></div>`;
  }
  
  if (capture.error) {
    const errorText = escapeText(capture.error);
    const truncated = errorText.length > 80 ? errorText.substring(0, 80) + '…' : errorText;
    html += `<div class="detail-row error-row"><span class="detail-label">Error:</span><span class="detail-value error-text">${truncated}</span></div>`;
  }
  
  html += '</div>';
  return html;
}

// ---------------------------------------------------------------------------
// TCP Quality Section
// ---------------------------------------------------------------------------

/** Render TCP quality evidence section for an event.
 * Shows actual probe socket TCP_INFO when available, or fallback/unavailable states.
 */
function renderTcpQualitySection(event: TimelineEvent): string {
  const tcpQuality = event.nativeTcpQuality;
  if (!tcpQuality) return '';

  const sourceLabel = getTcpQualitySourceLabel(event);
  const isNative = isNativeTcpQuality(event);
  const isSynthetic = isSyntheticTcpQuality(event);
  const isSs = isSsTcpQuality(event);
  const isUnavailable = isTcpQualityUnavailable(event);

  // Determine CSS class based on evidence reliability
  let badgeClass = 'tcp-quality-badge';
  if (isNative) {
    badgeClass += ' tcp-quality-native';
  } else if (isSynthetic || isSs) {
    badgeClass += ' tcp-quality-synthetic';
  } else {
    badgeClass += ' tcp-quality-unavailable';
  }

  let html = '<div class="tcp-quality-section">';
  html += '<div class="tcp-quality-header">';
  html += `<div class="tcp-quality-title">TCP Path Quality</div>`;
  html += `<span class="${badgeClass}">${escapeText(sourceLabel)}</span>`;
  html += '</div>';

  // If unavailable, show error message
  if (isUnavailable) {
    if (tcpQuality.error) {
      html += `<div class="tcp-quality-error">${escapeText(tcpQuality.error)}</div>`;
    } else if (tcpQuality.error_kind) {
      html += `<div class="tcp-quality-error">${escapeText(tcpQuality.error_kind)}</div>`;
    }
    html += '</div>';
    return html;
  }

  // Show connection details if available
  if (tcpQuality.state) {
    html += `<div class="detail-row"><span class="detail-label">State:</span><span class="detail-value">${escapeText(tcpQuality.state)}</span></div>`;
  }

  // Show RTT metrics
  if (tcpQuality.rtt_us !== undefined) {
    const rttMs = (tcpQuality.rtt_us / 1000).toFixed(1);
    html += `<div class="detail-row"><span class="detail-label">RTT:</span><span class="detail-value">${rttMs} ms</span></div>`;
  }

  if (tcpQuality.rttvar_us !== undefined) {
    const rttvarMs = (tcpQuality.rttvar_us / 1000).toFixed(1);
    html += `<div class="detail-row"><span class="detail-label">RTT variance:</span><span class="detail-value">${rttvarMs} ms</span></div>`;
  }

  // Show retransmit metrics
  if (tcpQuality.retransmits_current !== undefined) {
    html += `<div class="detail-row"><span class="detail-label">Retransmits (current):</span><span class="detail-value">${tcpQuality.retransmits_current}</span></div>`;
  }

  if (tcpQuality.retransmits_total !== undefined) {
    html += `<div class="detail-row"><span class="detail-label">Retransmits (total):</span><span class="detail-value">${tcpQuality.retransmits_total}</span></div>`;
  }

  // Show congestion window
  if (tcpQuality.snd_cwnd !== undefined) {
    html += `<div class="detail-row"><span class="detail-label">Congestion window:</span><span class="detail-value">${tcpQuality.snd_cwnd}</span></div>`;
  }

  // Show congestion algorithm
  if (tcpQuality.congestion_algorithm) {
    html += `<div class="detail-row"><span class="detail-label">Congestion algorithm:</span><span class="detail-value">${escapeText(tcpQuality.congestion_algorithm)}</span></div>`;
  }

  // Show delivery rate
  if (tcpQuality.delivery_rate_bps !== undefined) {
    const rateMbps = (tcpQuality.delivery_rate_bps / 1000000).toFixed(2);
    html += `<div class="detail-row"><span class="detail-label">Delivery rate:</span><span class="detail-value">${rateMbps} Mbps</span></div>`;
  }

  // Show queue depths
  if (tcpQuality.send_queue_bytes !== undefined) {
    html += `<div class="detail-row"><span class="detail-label">Send queue:</span><span class="detail-value">${tcpQuality.send_queue_bytes} bytes</span></div>`;
  }

  if (tcpQuality.recv_queue_bytes !== undefined) {
    html += `<div class="detail-row"><span class="detail-label">Receive queue:</span><span class="detail-value">${tcpQuality.recv_queue_bytes} bytes</span></div>`;
  }

  // Show loss indicators
  if (tcpQuality.lost !== undefined) {
    html += `<div class="detail-row"><span class="detail-label">Lost packets:</span><span class="detail-value">${tcpQuality.lost}</span></div>`;
  }

  if (tcpQuality.unacked !== undefined) {
    html += `<div class="detail-row"><span class="detail-label">Unacked:</span><span class="detail-value">${tcpQuality.unacked}</span></div>`;
  }

  // Show when collected
  if (tcpQuality.collected_at) {
    html += `<div class="detail-row"><span class="detail-label">Collected:</span><span class="detail-value">${formatTimelineTime(tcpQuality.collected_at)}</span></div>`;
  }

  html += '</div>';
  return html;
}

// ---------------------------------------------------------------------------
// Network Diagnostics
// ---------------------------------------------------------------------------

/** Render network diagnostics section */
function renderNetworkDiagSection(event: TimelineEvent): string {
  const capture = event.primaryCapture;
  if (!capture?.network_diag) return '';
  
  // Don't show network diag if suppressed by cooldown
  if (capture.suppressed_by_cooldown) {
    return '<div class="network-diag-section"><span class="diag-muted">Network diagnostics suppressed by cooldown</span></div>';
  }
  
  const diag = capture.network_diag;
  const statusClass = diag.status === 'ok' ? 'status-ok' : 'status-warn';
  
  let html = '<div class="network-diag-section">';
  html += '<div class="network-section-title">Network Diagnostics</div>';
  html += `<div class="detail-row"><span class="detail-label">Status:</span><span class="detail-value"><span class="${statusClass}">${escapeText(diag.status)}</span></span></div>`;
  
  if (diag.underlay_tcp && diag.underlay_tcp.length > 0) {
    html += '<div class="tcp-sockets">';
    for (const sock of diag.underlay_tcp) {
      const name = escapeText(sock.name) || 'socket';
      const state = escapeText(sock.state) || 'UNKNOWN';
      html += `<div class="tcp-socket-row">`;
      html += `<span class="tcp-socket-name">${name}</span>`;
      html += `<span class="tcp-socket-state">${state}</span>`;
      if (sock.rtt_ms !== undefined) {
        html += `<span class="tcp-socket-rtt">RTT: ${sock.rtt_ms.toFixed(1)} ms</span>`;
      }
      if (sock.retransmits !== undefined) {
        html += `<span class="tcp-socket-retrans">retrans: ${sock.retransmits}</span>`;
      }
      html += '</div>';
    }
    html += '</div>';
  } else if (diag.underlay_tcp !== undefined) {
    html += '<span class="diag-muted">No TCP sockets captured</span>';
  }
  
  html += '</div>';
  return html;
}

// ---------------------------------------------------------------------------
// Anchor Summary Rendering
// ---------------------------------------------------------------------------

/** Render embedded anchor event summary section */
function renderAnchorSummarySection(event: TimelineEvent): string {
  const summary = getAnchorEventSummary(event);
  if (!summary) return '';
  
  let html = '<div class="anchor-summary-section">';
  html += '<div class="anchor-summary-title">Anchor Provenance (Embedded Summary)</div>';
  
  // Suppression by prior capture message
  const probeKind = upperLabel(summary.probe_kind, 'unknown');
  html += `<div class="anchor-summary-row"><span class="anchor-summary-text">Suppressed by prior ${probeKind} capture</span></div>`;
  
  // Anchor event ID
  if (summary.event_id) {
    html += `<div class="anchor-summary-row"><span class="anchor-summary-label">Anchor event ID:</span><span class="anchor-summary-value">${escapeText(summary.event_id)}</span></div>`;
  }
  
  // Anchor capture ID
  if (summary.capture_id) {
    html += `<div class="anchor-summary-row"><span class="anchor-summary-label">Anchor capture ID:</span><span class="anchor-summary-value">${escapeText(summary.capture_id)}</span></div>`;
  }
  
  // Anchor sample time
  if (summary.sample_ts) {
    html += `<div class="anchor-summary-row"><span class="anchor-summary-label">Anchor sample time:</span><span class="anchor-summary-value">${formatTimelineTime(summary.sample_ts)}</span></div>`;
  }
  
  // Anchor captured at
  if (summary.captured_at) {
    html += `<div class="anchor-summary-row"><span class="anchor-summary-label">Anchor captured at:</span><span class="anchor-summary-value">${formatTimelineTime(summary.captured_at)}</span></div>`;
  }
  
  // Anchor latency
  if (summary.latency_ms !== undefined) {
    html += `<div class="anchor-summary-row"><span class="anchor-summary-label">Anchor latency:</span><span class="anchor-summary-value">${summary.latency_ms} ms</span></div>`;
  }
  
  // Anchor severity
  if (summary.severity) {
    const severityLabel = upperLabel(summary.severity, 'WARNING');
    html += `<div class="anchor-summary-row"><span class="anchor-summary-label">Anchor severity:</span><span class="anchor-summary-value">${severityLabel}</span></div>`;
  }
  
  // Anchor source
  if (summary.source) {
    html += `<div class="anchor-summary-row"><span class="anchor-summary-label">Anchor source:</span><span class="anchor-summary-value">${escapeText(summary.source)}</span></div>`;
  }
  
  // Anchor capture status
  if (summary.capture_status) {
    html += `<div class="anchor-summary-row"><span class="anchor-summary-label">Anchor capture status:</span><span class="anchor-summary-value">${escapeText(summary.capture_status)}</span></div>`;
  }
  
  // Visibility reason
  const visibilityReason = getAnchorVisibilityReason(event);
  if (visibilityReason) {
    html += `<div class="anchor-summary-row"><span class="anchor-summary-label">Visibility reason:</span><span class="anchor-summary-value">${escapeText(visibilityReason)}</span></div>`;
  }
  
  html += '</div>';
  return html;
}

/** Render degraded suppression warning */
function renderDegradedWarning(event: TimelineEvent): string {
  if (!isSuppressionDegraded(event)) return '';
  
  const reason = getDegradedReason(event) || 'unknown';
  const visibilityReason = getAnchorVisibilityReason(event);
  
  let html = '<div class="suppression-degraded-warning">';
  html += '<div class="warning-icon">⚠</div>';
  html += '<div class="warning-content">';
  html += '<div class="warning-title">Suppression provenance degraded</div>';
  html += `<div class="warning-text">Anchor was not visible at response time and no anchor summary was available.</div>`;
  html += `<div class="warning-reason">Reason: ${escapeText(reason)}</div>`;
  if (visibilityReason) {
    html += `<div class="warning-visibility">Visibility: ${escapeText(visibilityReason)}</div>`;
  }
  html += '</div></div>';
  
  return html;
}

// ---------------------------------------------------------------------------
// Expanded Row Rendering
// ---------------------------------------------------------------------------

/** Render expanded evidence panel for an event */
export function renderExpandedPanel(event: TimelineEvent, rowIndex: number): string {
  const detailsId = `timeline-details-${rowIndex}`;
  
  // Check if this is a degraded suppression
  const degraded = event.captureStatus === 'suppressed' && isSuppressionDegraded(event);
  
  let html = `<div class="timeline-expanded-panel ${degraded ? 'panel-degraded' : ''}" id="${detailsId}">`;
  
  // Event metadata
  html += '<div class="event-metadata">';
  html += `<div class="detail-row"><span class="detail-label">Event ID:</span><span class="detail-value event-id">${escapeText(event.eventId)}</span></div>`;
  html += `<div class="detail-row"><span class="detail-label">Sample time:</span><span class="detail-value">${formatTimelineTime(event.sampleTs)}</span></div>`;
  html += `<div class="detail-row"><span class="detail-label">Collected at:</span><span class="detail-value">${formatTimelineTime(event.collectedAt)}</span></div>`;
  html += `<div class="detail-row"><span class="detail-label">Latency:</span><span class="detail-value latency-value">${formatLatencyMs(event.latencyMs)}</span></div>`;
  html += `<div class="detail-row"><span class="detail-label">Rolling median:</span><span class="detail-value">${event.rollingMedianMs} ms</span></div>`;
  
  if (event.reasons.length > 0) {
    html += `<div class="detail-row"><span class="detail-label">Reasons:</span><span class="detail-value">${escapeText(event.reasons.join(', '))}</span></div>`;
  }
  
  html += '</div>';
  
  // Render degraded warning if applicable
  html += renderDegradedWarning(event);
  
  // Cross-probe suppression wording
  html += renderCrossProbeSuppression(event);
  
  // Anchor summary (embedded anchor provenance)
  html += renderAnchorSummarySection(event);
  
  // Capture details
  html += renderCaptureDetailsSection(event);
  
  // Cooldown details
  html += renderCooldownDetails(event);
  
  // Network diagnostics
  html += renderNetworkDiagSection(event);
  
  // TCP quality evidence
  html += renderTcpQualitySection(event);
  
  // Action buttons
  html += '<div class="timeline-actions">';
  html += `<button class="timeline-action-btn copy-btn" data-event-id="${escapeText(event.eventId)}">Copy JSON</button>`;
  html += `<button class="timeline-action-btn download-btn" data-event-id="${escapeText(event.eventId)}">Download</button>`;
  html += '</div>';
  
  html += '</div>';
  return html;
}

// ---------------------------------------------------------------------------
// Timeline Row Rendering
// ---------------------------------------------------------------------------

/** Check if event has an anchor badge to display */
function hasAnchorBadge(event: TimelineEvent): boolean {
  return isPinnedAnchor(event);
}

/** Render anchor badge for pinned anchors */
function renderAnchorBadge(): string {
  return '<span class="anchor-badge">anchor</span>';
}

/** Render a single timeline row */
export function renderTimelineRow(event: TimelineEvent, rowIndex: number): string {
  const time = formatSpikeTime(event.sampleTs);
  const latency = formatLatencyMs(event.latencyMs);
  const probeKindClass = getProbeKindClass(event.probeKind);
  const severityClass = getSeverityClass(event.severity);
  
  // Check for degraded suppression
  const suppressed = event.captureStatus === 'suppressed';
  const degraded = suppressed && isSuppressionDegraded(event);
  const captureStatusClass = getDegradedBadgeClass(event.captureStatus, degraded);
  const captureStatusLabel = getDegradedBadgeLabel(event.captureStatus, degraded);
  
  // Get error text if failed
  let detailsText = '—';
  if (event.captureStatus === 'failed' && event.primaryCapture?.error) {
    const error = event.primaryCapture.error;
    detailsText = error.length > 60 ? error.substring(0, 57) + '…' : error;
  }
  
  const detailsId = `timeline-details-${rowIndex}`;
  
  // Use upperLabel for defense-in-depth - ensures .toUpperCase() never throws
  const probeKindLabel = upperLabel(event.probeKind, 'HTTP');
  const severityLabel = upperLabel(event.severity, 'WARNING');
  
  // Render anchor badge if this is a pinned anchor
  const anchorBadgeHtml = hasAnchorBadge(event) ? renderAnchorBadge() : '';
  
  // Build row class with extra classes for pinned/degraded rows
  const rowClasses = ['timeline-row'];
  if (degraded) rowClasses.push('timeline-row-degraded');
  if (hasAnchorBadge(event)) rowClasses.push('timeline-row-pinned');
  
  return `
    <tr class="${rowClasses.join(' ')}" data-row-index="${rowIndex}">
      <td class="timeline-cell time-cell">${time}</td>
      <td class="timeline-cell probe-cell"><span class="probe-badge ${probeKindClass}">${probeKindLabel}</span>${anchorBadgeHtml}</td>
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
// Timeline Table Rendering
// ---------------------------------------------------------------------------

/** Render the complete timeline table */
export function renderTimelineTable(events: TimelineEvent[]): string {
  if (events.length === 0) {
    return '<div class="timeline-empty">No diagnostic events in the selected range.</div>';
  }
  
  const rows = events.map((event, index) => renderTimelineRow(event, index)).join('');
  
  // Expanded panels
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
export function renderLoadingState(): string {
  return `
    <div class="timeline-loading">
      <div class="spinner"></div>
      <span>Loading diagnostic timeline...</span>
    </div>
  `;
}

/** Render error state */
export function renderErrorState(error: string): string {
  return `
    <div class="timeline-error">
      <span class="error-icon">⚠</span>
      <span class="error-text">${escapeText(error)}</span>
    </div>
  `;
}

/** Render empty state */
export function renderEmptyState(): string {
  return `
    <div class="timeline-empty-state">
      <div class="empty-icon">📭</div>
      <div class="empty-title">No Diagnostic Events</div>
      <div class="empty-text">No HTTP or ICMP spike diagnostics have been recorded for this target.</div>
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Complete Timeline Render
// ---------------------------------------------------------------------------

/** Render the complete timeline component (legacy signature without pagination) */
export function renderTimeline(
  container: HTMLElement,
  state: TimelineState,
  filters: TimelineFilters,
  filteredEvents: TimelineEvent[]
): void {
  // Use default pagination context for backward compatibility
  const paginationCtx: PaginationContext = {
    pagination: { pageIndex: 0, pageSize: 20 },
    totalPages: Math.max(1, Math.ceil(filteredEvents.length / 20)),
    safePageIndex: 0,
    filteredCount: filteredEvents.length,
  };
  renderTimelineWithPagination(container, state, filters, filteredEvents, paginationCtx);
}

/** Render the complete timeline component with pagination support */
export function renderTimelineWithPagination(
  container: HTMLElement,
  state: TimelineState,
  filters: TimelineFilters,
  filteredEvents: TimelineEvent[],
  paginationCtx: PaginationContext
): void {
  const { isLoading, error, httpSummary, icmpSummary } = state;
  
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
  
  // Summary cards
  const summaryCards = renderSummaryCards(httpSummary, icmpSummary);
  
  // Filter controls (show total count, not filtered count)
  const filterControls = renderFilterControls(filters, state.mergedEvents.length, paginationCtx.filteredCount);
  
  // Pagination controls
  const paginationControls = renderPaginationControls(
    paginationCtx.pagination,
    paginationCtx.totalPages,
    paginationCtx.safePageIndex,
    paginationCtx.filteredCount
  );
  
  // Timeline table or empty state
  const timelineContent = filteredEvents.length === 0 && state.mergedEvents.length === 0
    ? renderEmptyState()
    : renderTimelineTable(filteredEvents);
  
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
