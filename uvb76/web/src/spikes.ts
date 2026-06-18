// Spike diagnostics rendering module
import { api, type SpikeEventWithCaptures, type DiagCapture, type TcpSocketDiagData, type NetworkDiagData } from './api';
import { formatSpikeTime, formatLatencyMs } from './format';

// HTML escape helper for XSS protection
function escapeText(s: string | null | undefined): string {
  if (!s) return '';
  const div = document.createElement('div');
  div.textContent = s;
  return div.innerHTML;
}

// Format capture status for display
function formatCaptureStatus(capture: DiagCapture): string {
  if (capture.suppressed_by_cooldown) {
    return 'suppressed by cooldown';
  }
  switch (capture.status) {
    case 'ok':
      return 'ok';
    case 'error':
      return 'error';
    case 'timeout':
      return 'timeout';
    case 'disabled':
      return 'disabled';
    case 'no_peer_mapping':
      return 'no peer mapping';
    case 'unavailable':
      return 'unavailable';
    default:
      return escapeText(capture.status);
  }
}

// Get CSS class for capture status
function getCaptureStatusClass(capture: DiagCapture): string {
  if (capture.suppressed_by_cooldown) {
    return 'status-suppressed';
  }
  switch (capture.status) {
    case 'ok':
      return 'status-ok';
    case 'error':
    case 'timeout':
      return 'status-error';
    case 'disabled':
    case 'no_peer_mapping':
    case 'unavailable':
      return 'status-muted';
    default:
      return 'status-unknown';
  }
}

// Format safe error text (compact, no sensitive details)
function formatError(error: string | null | undefined): string {
  if (!error) return '';
  const text = escapeText(error);
  if (text.length > 80) {
    return text.substring(0, 80) + '…';
  }
  return text;
}

// Format TCP socket details for expanded view
function formatTcpDetails(sockets: TcpSocketDiagData[]): string {
  if (!sockets || sockets.length === 0) return '<span class="diag-muted">No TCP sockets captured</span>';
  
  return sockets.map(sock => {
    const name = escapeText(sock.name) || 'socket';
    const state = escapeText(sock.state) || 'UNKNOWN';
    
    const fields: string[] = [];
    fields.push(`<span class="tcp-field"><span class="tcp-label">socket:</span> ${name}</span>`);
    fields.push(`<span class="tcp-field"><span class="tcp-label">state:</span> ${state}</span>`);
    
    if (sock.rtt_ms !== undefined) {
      fields.push(`<span class="tcp-field"><span class="tcp-label">RTT:</span> ${sock.rtt_ms.toFixed(1)} ms</span>`);
    }
    if (sock.rto_ms !== undefined) {
      fields.push(`<span class="tcp-field"><span class="tcp-label">RTO:</span> ${sock.rto_ms} ms</span>`);
    }
    if (sock.retransmits !== undefined) {
      fields.push(`<span class="tcp-field"><span class="tcp-label">retransmits:</span> ${sock.retransmits}</span>`);
    }
    if (sock.unacked !== undefined) {
      fields.push(`<span class="tcp-field"><span class="tcp-label">unacked:</span> ${sock.unacked}</span>`);
    }
    if (sock.cwnd !== undefined) {
      fields.push(`<span class="tcp-field"><span class="tcp-label">cwnd:</span> ${sock.cwnd}</span>`);
    }
    if (sock.send_queue_bytes !== undefined) {
      fields.push(`<span class="tcp-field"><span class="tcp-label">send_queue:</span> ${sock.send_queue_bytes} bytes</span>`);
    }
    if (sock.recv_queue_bytes !== undefined) {
      fields.push(`<span class="tcp-field"><span class="tcp-label">recv_queue:</span> ${sock.recv_queue_bytes} bytes</span>`);
    }
    
    return `<div class="tcp-socket-row">${fields.join('')}</div>`;
  }).join('');
}

// Format TCP socket summary (compact xray-style display)
function formatTcpSummary(sockets: TcpSocketDiagData[]): string {
  if (!sockets || sockets.length === 0) return '';
  
  const parts: string[] = [];
  for (const sock of sockets.slice(0, 3)) { // Limit to 3 sockets
    const name = escapeText(sock.name) || 'socket';
    const state = escapeText(sock.state) || 'UNKNOWN';
    
    let summary = `${name} ${state}`;
    
    if (sock.rtt_ms !== undefined) {
      summary += ` · RTT ${sock.rtt_ms.toFixed(1)} ms`;
    }
    if (sock.rto_ms !== undefined) {
      summary += ` · RTO ${sock.rto_ms} ms`;
    }
    if (sock.retransmits !== undefined && sock.retransmits > 0) {
      summary += ` · retransmits ${sock.retransmits}`;
    }
    
    parts.push(summary);
  }
  
  return parts.join(' | ');
}

// Render network diagnostics details (expanded view)
function renderNetworkDiagDetails(diag: NetworkDiagData, capture: DiagCapture): string {
  const lines: string[] = [];
  
  // Status
  const diagStatus = escapeText(diag.status) || 'unknown';
  const statusClass = diag.status === 'ok' ? 'status-ok' : 'status-warn';
  lines.push(`<div class="detail-section">
    <div class="detail-label">Network diagnostics:</div>
    <div class="detail-value"><span class="${statusClass}">${diagStatus}</span></div>
  </div>`);
  
  // started_at
  if (diag.started_at) {
    lines.push(`<div class="detail-section">
      <div class="detail-label">Diagnostics started:</div>
      <div class="detail-value">${escapeText(diag.started_at)}</div>
    </div>`);
  }
  
  // Underlay TCP
  if (diag.underlay_tcp && diag.underlay_tcp.length > 0) {
    lines.push(`<div class="detail-section">
      <div class="detail-label">Underlay TCP:</div>
      <div class="detail-tcp">${formatTcpDetails(diag.underlay_tcp)}</div>
    </div>`);
  } else {
    lines.push(`<div class="detail-section">
      <div class="detail-label">Underlay TCP:</div>
      <div class="detail-value"><span class="diag-muted">No TCP diagnostics captured</span></div>
    </div>`);
  }
  
  return lines.join('');
}

// Render capture details (expanded view)
function renderCaptureDetails(capture: DiagCapture, spikeEventId: string, targetId: string): string {
  const lines: string[] = [];
  
  // Source
  lines.push(`<div class="detail-section">
    <div class="detail-label">Source:</div>
    <div class="detail-value">${escapeText(capture.source)}</div>
  </div>`);
  
  // Status
  const statusText = formatCaptureStatus(capture);
  const statusClass = getCaptureStatusClass(capture);
  lines.push(`<div class="detail-section">
    <div class="detail-label">Status:</div>
    <div class="detail-value"><span class="${statusClass}">${statusText}</span></div>
  </div>`);
  
  // Duration
  if (capture.duration_ms !== undefined) {
    lines.push(`<div class="detail-section">
      <div class="detail-label">Duration:</div>
      <div class="detail-value">${capture.duration_ms} ms</div>
    </div>`);
  }
  
  // Started
  lines.push(`<div class="detail-section">
    <div class="detail-label">Started:</div>
    <div class="detail-value">${escapeText(capture.capture_started_at)}</div>
  </div>`);
  
  // Finished
  if (capture.capture_finished_at) {
    lines.push(`<div class="detail-section">
      <div class="detail-label">Finished:</div>
      <div class="detail-value">${escapeText(capture.capture_finished_at)}</div>
    </div>`);
  }
  
  // Suppressed by cooldown
  lines.push(`<div class="detail-section">
    <div class="detail-label">Suppressed by cooldown:</div>
    <div class="detail-value">${capture.suppressed_by_cooldown ? 'yes' : 'no'}</div>
  </div>`);
  
  // Error
  if (capture.error) {
    lines.push(`<div class="detail-section">
      <div class="detail-label">Error:</div>
      <div class="detail-value capture-error">${formatError(capture.error)}</div>
    </div>`);
  }
  
  // Network diagnostics
  if (capture.network_diag && !capture.suppressed_by_cooldown) {
    lines.push(renderNetworkDiagDetails(capture.network_diag, capture));
  } else if (capture.status === 'ok' && !capture.suppressed_by_cooldown) {
    // Warning: capture succeeded but no network_diag
    lines.push(`<div class="detail-section">
      <div class="detail-label">Network diagnostics:</div>
      <div class="detail-value"><span class="status-warn">missing</span></div>
    </div>`);
    lines.push(`<div class="capture-incomplete-warning">
      Capture succeeded, but network diagnostics were not included in the tovarisch response.
    </div>`);
  }
  
  return lines.join('');
}

// Generate safe filename from string
function sanitizeFilename(str: string): string {
  return str
    .replace(/[^a-zA-Z0-9\-_]/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
    .substring(0, 64) || 'unnamed';
}

// Download JSON blob helper
function downloadJson(data: object, filename: string): void {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

// Download capture as JSON
function downloadCaptureJson(capture: DiagCapture, spikeEventId: string, targetId: string): void {
  const exportData = {
    export_kind: 'uvb76_diagnostic_capture',
    exported_at: new Date().toISOString(),
    target_id: targetId,
    spike_event_id: spikeEventId,
    capture: capture,
  };
  
  const filename = `uvb76-capture-${sanitizeFilename(targetId)}-${sanitizeFilename(spikeEventId)}-${sanitizeFilename(capture.source)}.json`;
  downloadJson(exportData, filename);
}

// Download spike bundle as JSON
function downloadSpikeBundle(spike: SpikeEventWithCaptures): void {
  const exportData = {
    export_kind: 'uvb76_spike_diagnostics_bundle',
    exported_at: new Date().toISOString(),
    target_id: spike.target_id,
    spike: spike,
    captures: spike.captures || [],
  };
  
  const filename = `uvb76-spike-${sanitizeFilename(spike.target_id)}-${sanitizeFilename(spike.event_id)}.json`;
  downloadJson(exportData, filename);
}

// Render a single capture row with view details and download
// Uses index-based identity to safely handle sources with special characters
function renderCaptureRow(capture: DiagCapture, spikeIndex: number, captureIndex: number, spikeEventId: string, targetId: string): string {
  const statusClass = getCaptureStatusClass(capture);
  const statusText = formatCaptureStatus(capture);
  const sourceName = escapeText(capture.source);
  const duration = capture.duration_ms !== undefined ? ` (${capture.duration_ms}ms)` : '';
  
  // Determine network diag status
  let networkDiagStatus = '';
  let hasNetworkDiag = false;
  
  if (capture.suppressed_by_cooldown) {
    // Suppressed capture - don't show network_diag as success
    networkDiagStatus = `<span class="diag-label">Network diag:</span> <span class="status-muted">suppressed</span>`;
  } else if (capture.network_diag) {
    hasNetworkDiag = true;
    const diag = capture.network_diag;
    const diagStatus = escapeText(diag.status) || 'unknown';
    networkDiagStatus = `<span class="diag-label">Network diag:</span> <span class="${diag.status === 'ok' ? 'status-ok' : 'status-warn'}">${diagStatus}</span>`;
  } else if (capture.status === 'ok') {
    // Warning: capture succeeded but no network_diag
    networkDiagStatus = `<span class="diag-label">Network diag:</span> <span class="status-warn">missing</span>`;
  }
  
  // TCP summary if available
  let tcpSummary = '';
  if (hasNetworkDiag && capture.network_diag!.underlay_tcp && capture.network_diag!.underlay_tcp.length > 0) {
    const tcpSummaryText = formatTcpSummary(capture.network_diag!.underlay_tcp);
    if (tcpSummaryText) {
      tcpSummary = `<div class="diag-row tcp-summary">
        <span class="diag-label">Underlay TCP:</span>
        <span class="tcp-text">${tcpSummaryText}</span>
      </div>`;
    }
  }
  
  const errorText = formatError(capture.error);
  
  // Use index-based ID for safe DOM manipulation (source may contain special chars)
  const detailsId = `capture-details-${spikeIndex}-${captureIndex}`;
  
  return `
    <div class="capture-row">
      <div class="capture-header">
        <span class="capture-source">${sourceName}${duration}</span>
        <span class="capture-status ${statusClass}">Capture: ${statusText}</span>
      </div>
      ${errorText ? `<div class="capture-error">${errorText}</div>` : ''}
      ${capture.status === 'ok' && !hasNetworkDiag && !capture.suppressed_by_cooldown ? 
        `<div class="capture-incomplete-warning">
          Capture succeeded, but network diagnostics were not included in the tovarisch response.
        </div>` : ''}
      ${networkDiagStatus ? `<div class="diag-row">${networkDiagStatus}</div>` : ''}
      ${tcpSummary}
      <div class="capture-actions">
        <button type="button" class="view-details-btn" data-details-id="${detailsId}" data-spike-index="${spikeIndex}" data-capture-index="${captureIndex}" data-target-id="${targetId}">
          View details
        </button>
        <button type="button" class="download-capture-btn" data-spike-index="${spikeIndex}" data-capture-index="${captureIndex}" data-target-id="${targetId}">
          Download capture JSON
        </button>
      </div>
      <div class="capture-details" id="${detailsId}" style="display: none;">
        ${renderCaptureDetails(capture, spikeEventId, targetId)}
      </div>
    </div>
  `;
}

// Render spike diagnostics for a target
export async function loadSpikeDiagnostics(targetId: string): Promise<void> {
  const container = document.getElementById(`spike-diag-${targetId}`);
  if (!container) return;
  
  try {
    const response = await api.getLatencySpikesWithCaptures(targetId, 10);
    
    if (!response.spikes || response.spikes.length === 0) {
      container.innerHTML = '<div class="spike-diag-empty">No recent spikes</div>';
      return;
    }
    
    // Store response for download handlers
    const responseData = response;
    
    const spikesHTML = response.spikes.map((spike: SpikeEventWithCaptures, spikeIndex: number) => {
      const time = formatSpikeTime(spike.sample_ts);
      const kind = escapeText(spike.kind);
      const severity = escapeText(spike.severity);
      const latency = formatLatencyMs(spike.latency_ms);
      const reasons = spike.reasons?.map((r: string) => escapeText(r)).join(', ') || '';
      
      let capturesHTML = '<div class="captures-empty">No captures</div>';
      if (spike.captures && spike.captures.length > 0) {
        capturesHTML = spike.captures.map((capture, captureIndex) => 
          renderCaptureRow(capture, spikeIndex, captureIndex, spike.event_id, spike.target_id)
        ).join('');
      }
      
      return `
        <div class="spike-row">
          <div class="spike-header">
            <span class="spike-time">${time}</span>
            <span class="spike-kind">${kind}</span>
            <span class="spike-severity ${severity.toLowerCase()}">${severity}</span>
            <span class="spike-latency">${latency}</span>
            ${reasons ? `<span class="spike-reasons">${reasons}</span>` : ''}
          </div>
          <div class="spike-captures">
            ${capturesHTML}
          </div>
          ${spike.captures && spike.captures.length > 0 ? `
            <div class="spike-actions">
              <button type="button" class="download-spike-btn" data-spike-index="${spikeIndex}">
                Download spike bundle
              </button>
            </div>
          ` : ''}
        </div>
      `;
    }).join('');
    
    container.innerHTML = `
      <div class="spike-diag-header">
        <span class="spike-diag-title">Spike diagnostics</span>
      </div>
      ${spikesHTML}
    `;
    
    // Attach event handlers
    // View details buttons
    container.querySelectorAll('.view-details-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const target = e.target as HTMLButtonElement;
        const detailsId = target.dataset.detailsId;
        const detailsEl = detailsId ? container.querySelector(`#${detailsId}`) : null;
        if (detailsEl) {
          const isHidden = detailsEl.style.display === 'none';
          detailsEl.style.display = isHidden ? 'block' : 'none';
          target.textContent = isHidden ? 'Hide details' : 'View details';
        }
      });
    });
    
    // Download capture buttons - use index-based lookup for safety
    container.querySelectorAll('.download-capture-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const target = e.target as HTMLButtonElement;
        const spikeIndex = parseInt(target.dataset.spikeIndex || '', 10);
        const captureIndex = parseInt(target.dataset.captureIndex || '', 10);
        const tgtId = target.dataset.targetId;
        
        if (!isNaN(spikeIndex) && !isNaN(captureIndex) && tgtId) {
          const spike = responseData.spikes[spikeIndex];
          if (spike && spike.captures && spike.captures[captureIndex] !== undefined) {
            const capture = spike.captures[captureIndex];
            downloadCaptureJson(capture, spike.event_id, tgtId);
          }
        }
      });
    });
    
    // Download spike bundle buttons
    container.querySelectorAll('.download-spike-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const target = e.target as HTMLButtonElement;
        const index = parseInt(target.dataset.spikeIndex || '0', 10);
        const spike = responseData.spikes[index];
        if (spike) {
          downloadSpikeBundle(spike);
        }
      });
    });
  } catch (e) {
    console.error('Failed to load spike diagnostics for', targetId, e);
    container.innerHTML = '<div class="spike-diag-error">Spike diagnostics unavailable</div>';
  }
}
