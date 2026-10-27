export const contentKeys = {
  group: (gid: string | undefined) => ['contents', gid] as const,
  detail: (contentId: string | undefined) => ['contents', 'detail', contentId] as const,
  blob: (hash: string | null | undefined) => ['blob', hash] as const,
};
