import { Image as ImageIcon } from 'lucide-react';
import type { DynamicType } from '@/api';
import {
  DYNAMIC_TYPE_META,
  DYNAMIC_TYPE_ORDER,
  type DynamicTypeMeta,
} from '@/features/dynamic/model/type-meta';

export type AllContentType = 'image' | DynamicType;

type TypeMeta = DynamicTypeMeta;

const DYNAMIC_CONTENT_TYPE_META = Object.fromEntries(
  DYNAMIC_TYPE_ORDER.map((type) => [type, DYNAMIC_TYPE_META[type]])
) as Record<DynamicType, TypeMeta>;

export const TYPE_META: Record<AllContentType, TypeMeta> = {
  image: {
    label: '图片',
    hint: '上传图片，自动转 1bpp',
    description: '上传图片并配置抖动 / 阈值；可附音频或 TTS。',
    hasConfigurableParams: true,
    supportsAudio: true,
    Icon: ImageIcon,
  },
  ...DYNAMIC_CONTENT_TYPE_META,
};

const TYPE_ORDER = ['image', ...DYNAMIC_TYPE_ORDER] as const satisfies readonly AllContentType[];

export const TYPE_ITEMS = TYPE_ORDER.map((type) => ({
  type,
  title: TYPE_META[type].label,
  hint: TYPE_META[type].hint,
  Icon: TYPE_META[type].Icon,
}));
