// Diagnostic Timeline Update - Pure state transition functions

import type { TimelineModel, TimelinePageState, TimelineFilterState } from './model';
import { computeProbeKindSummary, applyFilters, clampPageIndex, getFirstVisibleRow, getPageIndexForRow } from './model';
import type { TimelineEvent } from '../diagnosticTimeline.model';
import type { TimelineMsg } from './msg';

// ---------------------------------------------------------------------------
// Update Result
// ---------------------------------------------------------------------------

/** Result of an update function - returns new model */
export interface UpdateResult {
  model: TimelineModel;
}

// ---------------------------------------------------------------------------
// Pure Update Function
// ---------------------------------------------------------------------------

/** Pure update: given current model and message, return new model */
export function update(model: TimelineModel, msg: TimelineMsg): UpdateResult {
  switch (msg.type) {
    case 'LoadStarted':
      return {
        model: {
          ...model,
          isLoading: true,
          error: null,
        },
      };

    case 'LoadFailed':
      return {
        model: {
          ...model,
          isLoading: false,
          error: msg.error,
        },
      };

    case 'TimelineLoaded':
      return handleTimelineLoaded(model, msg.events);

    case 'RefreshRequested':
      // No model change - just triggers effect
      return { model };

    case 'RefreshFailed':
      return {
        model: {
          ...model,
          isLoading: false,
          error: msg.error,
        },
      };

    case 'PageChanged':
      return handlePageChanged(model, msg.page);

    case 'PageSizeChanged':
      return handlePageSizeChanged(model, msg.pageSize);

    case 'FilterChanged':
      return handleFilterChanged(model, msg.filters);

    case 'FiltersReset':
      return handleFiltersReset(model);

    case 'RowToggled':
      return handleRowToggled(model, msg.eventId);

    default:
      return { model };
  }
}

// ---------------------------------------------------------------------------
// Message Handlers (Pure)
// ---------------------------------------------------------------------------

/** Handle timeline loaded with new events */
function handleTimelineLoaded(model: TimelineModel, events: TimelineEvent[]): UpdateResult {
  // Filter out expanded rows that no longer exist
  const existingEventIds = new Set(events.map(e => e.eventId));
  const survivingExpandedIds = new Set(
    [...model.expandedEventIds].filter(id => existingEventIds.has(id))
  );

  return {
    model: {
      ...model,
      mergedEvents: events,
      httpSummary: computeProbeKindSummary('http', events),
      icmpSummary: computeProbeKindSummary('icmp', events),
      isLoading: false,
      error: null,
      expandedEventIds: survivingExpandedIds,
      // Clamp pagination to valid range after data changes
      pagination: clampPaginationForFilteredCount(model, events),
    },
  };
}

/** Handle page change */
function handlePageChanged(model: TimelineModel, page: number): UpdateResult {
  const filteredCount = applyFilters(model.mergedEvents, model.filters).length;
  const totalPages = Math.max(1, Math.ceil(filteredCount / model.pagination.pageSize));
  const clampedPage = clampPageIndex(page, totalPages);
  
  return {
    model: {
      ...model,
      pagination: {
        ...model.pagination,
        pageIndex: clampedPage,
      },
    },
  };
}

/** Handle page size change - preserves first visible row when possible */
function handlePageSizeChanged(model: TimelineModel, pageSize: number): UpdateResult {
  const filteredCount = applyFilters(model.mergedEvents, model.filters).length;
  
  if (filteredCount === 0) {
    return {
      model: {
        ...model,
        pagination: {
          pageIndex: 0,
          pageSize,
        },
      },
    };
  }

  // Calculate the first visible row before changing page size
  const firstVisibleRow = getFirstVisibleRow(model.pagination.pageIndex, model.pagination.pageSize);
  
  // Update page size
  const newPagination: TimelinePageState = {
    pageIndex: model.pagination.pageIndex,
    pageSize,
  };
  
  // Calculate new page index that preserves the first visible row
  const newPageIndex = getPageIndexForRow(firstVisibleRow, pageSize);
  const newTotalPages = Math.max(1, Math.ceil(filteredCount / pageSize));
  newPagination.pageIndex = clampPageIndex(newPageIndex, newTotalPages);
  
  return {
    model: {
      ...model,
      pagination: newPagination,
    },
  };
}

/** Handle filter change - resets to page 1 */
function handleFilterChanged(
  model: TimelineModel,
  filters: Partial<TimelineModel['filters']>
): UpdateResult {
  return {
    model: {
      ...model,
      filters: {
        ...model.filters,
        ...filters,
      },
      pagination: {
        ...model.pagination,
        pageIndex: 0, // Reset to page 1 when filters change
      },
    },
  };
}

/** Handle filters reset */
function handleFiltersReset(model: TimelineModel): UpdateResult {
  return {
    model: {
      ...model,
      filters: {
        probeKind: 'all',
        captureStatus: 'all',
        severity: 'all',
      },
      pagination: {
        ...model.pagination,
        pageIndex: 0, // Reset to page 1 when filters reset
      },
    },
  };
}

/** Handle row toggle */
function handleRowToggled(model: TimelineModel, eventId: string): UpdateResult {
  const newExpandedIds = new Set(model.expandedEventIds);
  
  if (newExpandedIds.has(eventId)) {
    newExpandedIds.delete(eventId);
  } else {
    newExpandedIds.add(eventId);
  }
  
  return {
    model: {
      ...model,
      expandedEventIds: newExpandedIds,
    },
  };
}

// ---------------------------------------------------------------------------
// Helper Functions
// ---------------------------------------------------------------------------

/** Clamp pagination to valid range after data changes */
function clampPaginationForFilteredCount(model: TimelineModel, events: TimelineEvent[]): TimelinePageState {
  const filteredCount = applyFilters(events, model.filters).length;
  const totalPages = Math.max(1, Math.ceil(filteredCount / model.pagination.pageSize));
  
  return {
    ...model.pagination,
    pageIndex: clampPageIndex(model.pagination.pageIndex, totalPages),
  };
}

// ---------------------------------------------------------------------------
// Pure Invariants (for testing)
// ---------------------------------------------------------------------------

/** Verify invariants that should always hold after an update */
export function verifyInvariants(model: TimelineModel): string[] {
  const errors: string[] = [];
  
  // Page index should never be negative
  if (model.pagination.pageIndex < 0) {
    errors.push(`Page index should never be negative: ${model.pagination.pageIndex}`);
  }
  
  // Page size should be valid
  if (![10, 20, 50, 100].includes(model.pagination.pageSize)) {
    errors.push(`Invalid page size: ${model.pagination.pageSize}`);
  }
  
  // Expanded IDs should all exist in merged events
  const eventIds = new Set(model.mergedEvents.map(e => e.eventId));
  for (const id of model.expandedEventIds) {
    if (!eventIds.has(id)) {
      errors.push(`Expanded event ID not found in events: ${id}`);
    }
  }
  
  return errors;
}
