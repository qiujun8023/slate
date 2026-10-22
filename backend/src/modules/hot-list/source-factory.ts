import { hotListSourceLabel, type CurrentHotListSourceIdT } from 'shared';
import { fetchJson, fetchText } from '../../common/http/fetch';
import type { HotListItem, HotListSource, HotListSourceFetchCtx } from './hot-list.types';
import { withRanks } from './hot-list.utils';

type FetchOptions = NonNullable<Parameters<typeof fetchJson>[1]>;
type SourceId = HotListSource['id'];
type HotListItemInput = Omit<HotListItem, 'rank'> | HotListItem;
type SourceRequestOptions =
  Omit<FetchOptions, 'signal'> | ((ctx: HotListSourceFetchCtx) => Omit<FetchOptions, 'signal'>);

interface DefineDirectSourceConfig<TId extends CurrentHotListSourceIdT = CurrentHotListSourceIdT> {
  id: TId;
  label?: string;
  fetch(signal: AbortSignal): Promise<HotListItem[]>;
}

interface DefineSourceConfig<T> {
  id: SourceId;
  label: string;
  load(ctx: HotListSourceFetchCtx): Promise<T>;
  map(data: T): HotListItemInput[];
  ranked?: boolean;
}

interface DefineFetchSourceConfig<T> {
  id: SourceId;
  label: string;
  url: string | ((ctx: HotListSourceFetchCtx) => string);
  options?: SourceRequestOptions;
  map(data: T): HotListItemInput[];
  ranked?: boolean;
}

export function defineSource<T>(config: DefineSourceConfig<T>): HotListSource {
  return {
    id: config.id,
    label: config.label,
    async fetch(ctx) {
      const items = config.map(await config.load(ctx));
      return config.ranked === false ? (items as HotListItem[]) : withRanks(items);
    },
  };
}

export function defineDirectSource<TId extends CurrentHotListSourceIdT>(
  config: DefineDirectSourceConfig<TId>
): HotListSource {
  return {
    id: config.id,
    label: config.label ?? hotListSourceLabel(config.id),
    fetch: (ctx) => config.fetch(ctx.signal),
  };
}

export function defineJsonSource<T>(config: DefineFetchSourceConfig<T>): HotListSource {
  return defineSource({
    ...config,
    load: (ctx) => fetchJson<T>(resolveUrl(config.url, ctx), requestOptions(config.options, ctx)),
  });
}

export function defineTextSource(config: DefineFetchSourceConfig<string>): HotListSource {
  return defineSource({
    ...config,
    load: (ctx) => fetchText(resolveUrl(config.url, ctx), requestOptions(config.options, ctx)),
  });
}

function resolveUrl(
  url: string | ((ctx: HotListSourceFetchCtx) => string),
  ctx: HotListSourceFetchCtx
): string {
  return typeof url === 'function' ? url(ctx) : url;
}

function requestOptions(
  options: SourceRequestOptions | undefined,
  ctx: HotListSourceFetchCtx
): FetchOptions {
  const base = typeof options === 'function' ? options(ctx) : options;
  return { ...base, signal: ctx.signal };
}
