import type { DynamicConfig, DynamicType } from '@/api';
import { defaultDynamicFrameName } from '@/features/dynamic/model/display-name';
import { dynamicConfigKey } from '@/features/dynamic/model/json-parse';

export interface DynamicEditorBaseline {
  contentId: string;
  type: DynamicType;
  frameName: string;
  configKey: string;
}

export function createDynamicEditorBaseline(
  contentId: string,
  type: DynamicType,
  frameName: string | null,
  config: DynamicConfig
): DynamicEditorBaseline {
  return {
    contentId,
    type,
    frameName: frameName ?? defaultDynamicFrameName(type, config),
    configKey: dynamicConfigKey(config),
  };
}

export function isSameDynamicEditorBaseline(
  a: DynamicEditorBaseline,
  b: DynamicEditorBaseline
): boolean {
  return (
    a.contentId === b.contentId &&
    a.type === b.type &&
    a.frameName === b.frameName &&
    a.configKey === b.configKey
  );
}
