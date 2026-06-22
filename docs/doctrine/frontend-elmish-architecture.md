# Frontend Elm-ish Architecture Doctrine

## Purpose

Prevent stateful UVB-76 frontend features from drifting into hook soup or ad-hoc DOM/controller logic. This doctrine establishes Elm-ish architecture as the default pattern for non-trivial UI state management.

## When to Apply

Any frontend feature with server data **plus at least one** of:

- Pagination
- Filters
- Expansion state (collapsible rows/panels)
- Auto-refresh
- Modals
- Selection state
- Optimistic actions
- Persisted view state

Features with only static display or simple toggle states may use simpler patterns.

## Core Pattern: Model / Msg / Update / Effects / View

### 1. Model (`model.ts`)

The single source of truth. All UI state lives here.

```typescript
// Example: Timeline model
export interface TimelinePageState {
  pageIndex: number;
  pageSize: number;
}

export interface TimelineFilterState {
  probeKind: ProbeKind | 'all';
  captureStatus: CaptureStatusDisplay | 'all';
  severity: Severity | 'all';
}

export interface TimelineModel {
  // Server data
  mergedEvents: TimelineEvent[];
  isLoading: boolean;
  error: string | null;
  
  // UI state
  filters: TimelineFilterState;
  pagination: TimelinePageState;
  expandedEventIds: Set<string>;
  
  // Computed
  filteredEvents: TimelineEvent[];
  totalFiltered: number;
}
```

### 2. Messages (`msg.ts`)

All possible user actions and system events as a discriminated union.

```typescript
// Example: Timeline messages
export type TimelineMsg =
  | { type: 'TimelineLoaded'; events: TimelineEvent[] }
  | { type: 'RefreshRequested' }
  | { type: 'PageChanged'; page: number }
  | { type: 'PageSizeChanged'; pageSize: number }
  | { type: 'FilterChanged'; filters: Partial<TimelineFilterState> }
  | { type: 'RowToggled'; eventId: string }
  | { type: 'RefreshFailed'; error: string }
  | { type: 'LoadStarted' }
  | { type: 'LoadFailed'; error: string };
```

### 3. Update (`update.ts`)

**Pure functions only.** Given current model and a message, return the new model.

```typescript
export function update(model: TimelineModel, msg: TimelineMsg): TimelineModel {
  switch (msg.type) {
    case 'PageChanged':
      return {
        ...model,
        pagination: { ...model.pagination, pageIndex: msg.page }
      };
      
    case 'FilterChanged':
      return {
        ...updateFilteredEvents({
          ...model,
          filters: { ...model.filters, ...msg.filters },
          pagination: { ...model.pagination, pageIndex: 0 } // Reset to page 1
        }, msg.filters);
      };
      
    // ... other cases
  }
}
```

### 4. Effects (`effects.ts`)

Side effects are explicit and return commands, not mutating state.

```typescript
export interface TimelineEffect {
  type: 'FetchTimeline';
  targetId: string;
}

export async function executeEffect(
  effect: TimelineEffect,
  dispatch: (msg: TimelineMsg) => void
): Promise<void> {
  switch (effect.type) {
    case 'FetchTimeline':
      dispatch({ type: 'LoadStarted' });
      try {
        const events = await fetchTimeline(effect.targetId);
        dispatch({ type: 'TimelineLoaded', events });
      } catch (e) {
        dispatch({ type: 'LoadFailed', error: String(e) });
      }
  }
}
```

### 5. View (`view.ts`)

Renders the model to HTML. No business logic, no state mutations.

```typescript
export function renderTimeline(
  container: HTMLElement,
  model: TimelineModel
): void {
  const html = `
    <div class="timeline">
      ${renderFilters(model.filters)}
      ${renderPagination(model.pagination, model.totalFiltered)}
      ${renderTable(getPagedEvents(model))}
    </div>
  `;
  container.innerHTML = html;
}
```

### 6. Controller (`controller.ts`)

Wires DOM events, timers, and effects to the update loop.

```typescript
export class TimelineController {
  private model: TimelineModel;
  private dispatch: (msg: TimelineMsg) => void;
  
  constructor(targetId: string, container: HTMLElement) {
    this.model = initialModel();
    this.dispatch = (msg) => {
      const effects = update(this.model, msg);
      this.model = effects.model;
      renderTimeline(container, this.model);
      effects.effects.forEach(effect => executeEffect(effect, this.dispatch));
    };
    
    this.setupEventListeners(container);
    this.dispatch({ type: 'RefreshRequested' });
  }
}
```

## Invariants (Must Hold)

1. **Single source of truth**: All state in model, nowhere else
2. **Immutable updates**: Never mutate model, always return new model
3. **Explicit effects**: Side effects never hidden in update
4. **Separation**: View never has business logic; update never has DOM code
5. **Deterministic rendering**: Same model always produces same HTML

## Testing Requirements

### Pure Update Tests

Test the `update` function with no DOM, no async, no mocking:

```typescript
describe('update', () => {
  it('filter change resets page to 1', () => {
    const model = createModel({ pagination: { pageIndex: 5, pageSize: 20 } });
    const result = update(model, { type: 'FilterChanged', filters: { probeKind: 'http' } });
    expect(result.pagination.pageIndex).toBe(0);
  });
  
  it('page clamps when row count shrinks', () => {
    const model = createModel({ 
      pagination: { pageIndex: 10, pageSize: 20 },
      filteredEvents: Array(50).fill(null)
    });
    const result = update(model, { type: 'TimelineLoaded', events: [] });
    expect(result.pagination.pageIndex).toBe(0);
  });
  
  it('refresh preserves expanded rows for surviving event IDs', () => {
    const model = createModel({
      expandedEventIds: new Set(['evt-1', 'evt-2', 'evt-3']),
      mergedEvents: [
        { eventId: 'evt-1', ... },
        { eventId: 'evt-2', ... },
        // evt-3 no longer exists
      ]
    });
    const result = update(model, { type: 'TimelineLoaded', events: [...] });
    expect(result.expandedEventIds).toEqual(new Set(['evt-1', 'evt-2']));
  });
});
```

### DOM/Controller Tests

Only for wiring and event dispatch:

```typescript
describe('TimelineController', () => {
  it('dispatches PageChanged on pagination button click', () => {
    const dispatch = vi.fn();
    setupController({ dispatch });
    clickNextPage();
    expect(dispatch).toHaveBeenCalledWith({ type: 'PageChanged', page: 1 });
  });
});
```

## Non-Goals

- No Elm runtime required
- No full frontend rewrite
- No framework migration (we use vanilla TypeScript + Vitest)
- No generic state-machine framework until at least two real features use the pattern

## Pattern Evolution

This is intentionally lightweight. As the codebase matures:

- Consider formalizing the `Effect` type if multiple features need async
- Consider adding a `Subscriptions` concept for timers if auto-refresh patterns repeat
- Do not over-abstract before the pattern is proven in at least two features

## Related Doctrine

- `llm-friendliness.md`: Keep files small and explicit
- `ai-native-code-discipline-axioms.md`: Prefer typed, testable patterns
