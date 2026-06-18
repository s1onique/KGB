// Spike diagnostics rendering module
import { api, type SpikeEventWithCaptures, type DiagCapture, type TcpSocketDiagData } from './api';
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
      return 'status-muted';
    default:
      return 'status-unknown';
  }
}

// Format safe error text (compact, no sensitive details)
function formatError(error: string | null | undefined): string {
  if (!error) return '';
  // Truncate long errors and escape HTML
  const text = escapeText(error);
  if (text.length > 80) {
    return text.substring(0, 80) + '…';
  }
  return text;
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

// Render a single capture row
function renderCaptureRow(capture: DiagCapture): string {
  const statusClass = getCaptureStatusClass(capture);
  const statusText = formatCaptureStatus(capture);
  const sourceName = escapeText(capture.source);
  const duration = capture.duration_ms !== undefined ? ` (${capture.duration_ms}ms)` : '';
  const errorText = formatError(capture.error);
  
  let networkDiagText = '';
  if (capture.network_diag && !capture.suppressed_by_cooldown) {
    const diag = capture.network_diag;
    const diagStatus = escapeText(diag.status) || 'unknown';
    networkDiagText = `<div class="diag-row">
      <span class="diag-label">Network diag:</span>
      <span class="${diag.status === 'ok' ? 'status-ok' : 'status-warn'}">${diagStatus}</span>
    </div>`;
    
    // Add underlay TCP summary if available
    if (diag.underlay_tcp && diag.underlay_tcp.length > 0) {
      const tcpSummary = formatTcpSummary(diag.underlay_tcp);
      if (tcpSummary) {
        networkDiagText += `<div class="diag-row tcp-summary">
          <span class="diag-label">Underlay TCP:</span>
          <span class="tcp-text">${tcpSummary}</span>
        </div>`;
      }
    }
  }
  
  return `
    <div class="capture-row">
      <div class="capture-header">
        <span class="capture-source">${sourceName}${duration}</span>
        <span class="capture-status ${statusClass}">Capture: ${statusText}</span>
      </div>
      ${errorText ? `<div class="capture-error">${errorText}</div>` : ''}
      ${networkDiagText}
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
    
    const spikesHTML = response.spikes.map((spike: SpikeEventWithCaptures) => {
      const time = formatSpikeTime(spike.sample_ts);
      const kind = escapeText(spike.kind);
      const severity = escapeText(spike.severity);
      const latency = formatLatencyMs(spike.latency_ms);
      const reasons = spike.reasons?.map((r: string) => escapeText(r)).join(', ') || '';
      
      let capturesHTML = '<div class="captures-empty">No captures</div>';
      if (spike.captures && spike.captures.length > 0) {
        capturesHTML = spike.captures.map(renderCaptureRow).join('');
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
        </div>
      `;
    }).join('');
    
    container.innerHTML = `
      <div class="spike-diag-header">
        <span class="spike-diag-title">Spike diagnostics</span>
      </div>
      ${spikesHTML}
    `;
  } catch (e) {
    console.error('Failed to load spike diagnostics for', targetId, e);
    container.innerHTML = '<div class="spike-diag-error">Spike diagnostics unavailable</div>';
  }
}
