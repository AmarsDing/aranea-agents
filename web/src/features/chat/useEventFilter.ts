import { computed, ref } from 'vue';
import type { Envelope, EnvelopeType } from '../chat/envelope';
import { matchFilterKey } from '../chat/dispatcher';

export type EventFilterOptions = {
  types?: EnvelopeType[];
  channels?: string[];
  filterKey?: string;
  author?: string;
  keyword?: string;
};

export function useEventFilter(events: () => Envelope[], initialFilter?: EventFilterOptions) {
  const types = ref<EnvelopeType[] | undefined>(initialFilter?.types);
  const channels = ref<string[] | undefined>(initialFilter?.channels);
  const filterKey = ref<string | undefined>(initialFilter?.filterKey);
  const author = ref<string | undefined>(initialFilter?.author);
  const keyword = ref<string | undefined>(initialFilter?.keyword);

  const filtered = computed(() => {
    let list = events();
    if (types.value && types.value.length > 0) {
      list = list.filter((e) => types.value!.includes(e.type));
    }
    if (channels.value && channels.value.length > 0) {
      list = list.filter((e) => e.channel && channels.value!.includes(e.channel));
    }
    if (filterKey.value) {
      list = list.filter((e) => !e.filter_key || matchFilterKey(filterKey.value!, e.filter_key));
    }
    if (author.value) {
      const a = author.value.toLowerCase();
      list = list.filter((e) => e.author?.toLowerCase().includes(a));
    }
    if (keyword.value) {
      const kw = keyword.value.toLowerCase();
      list = list.filter(
        (e) =>
          e.type.includes(kw) ||
          e.author?.toLowerCase().includes(kw) ||
          e.content?.text?.toLowerCase().includes(kw) ||
          e.tool_call?.name?.toLowerCase().includes(kw) ||
          e.filter_key?.toLowerCase().includes(kw) ||
          e.error?.message?.toLowerCase().includes(kw),
      );
    }
    return list;
  });

  function setFilter(opts: EventFilterOptions) {
    types.value = opts.types;
    channels.value = opts.channels;
    filterKey.value = opts.filterKey;
    author.value = opts.author;
    keyword.value = opts.keyword;
  }

  function resetFilter() {
    types.value = undefined;
    channels.value = undefined;
    filterKey.value = undefined;
    author.value = undefined;
    keyword.value = undefined;
  }

  return {
    filtered,
    types,
    channels,
    filterKey,
    author,
    keyword,
    setFilter,
    resetFilter,
  };
}
