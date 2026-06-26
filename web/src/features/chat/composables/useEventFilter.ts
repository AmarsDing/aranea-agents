import { computed, ref, type Ref } from 'vue';
import {
  buildBranchTree,
  defaultEventFilterState,
  filterEnvelopes,
  type BranchNode,
  type EventFilterState,
  type InspectorEvent,
} from '../eventFilter';

export function useEventFilter(source: Ref<InspectorEvent[]>) {
  const filters = ref<EventFilterState>(defaultEventFilterState());

  const filteredEvents = computed(() => filterEnvelopes(source.value, filters.value));

  const branchTree = computed<BranchNode[]>(() => buildBranchTree(filteredEvents.value));

  function resetFilters(): void {
    filters.value = defaultEventFilterState();
  }

  return { filters, filteredEvents, branchTree, resetFilters };
}
