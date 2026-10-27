// 拖拽重排公共逻辑 — Groups / GroupDetail / 内容网格 都用这个 hook。
//
// 用法:
//   const { sensors, currentOrder, orderedItems, onDragEnd } = useDndOrder(
//     items,
//     (id) => id,            // 取拖拽 id 的函数(默认就是 String(item))
//     (newOrder) => mutateAsync({ order: newOrder }), // reject 表示保存失败
//   );
//
// 本地只负责拖拽后的即时顺序和失败回滚；最终顺序由调用方 mutation 后 invalidate 的
// React Query 数据回填。保存串行执行：请求进行中再拖拽时只记下最新顺序，等当前请求
// 结束后再提交，避免乱序到达的旧顺序最后写入；保存期间也不用回填数据覆盖本地顺序。
// 队列靠 onPersist 返回的 Promise 推进，组件卸载后排队的顺序照样提交；每次拖拽固定
// 当时的 onPersist，排队的顺序不会提交到别的资源。

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { arrayMove } from '@dnd-kit/sortable';

type Persist = (newOrder: string[]) => Promise<unknown>;

export function useDndOrder<T>(
  items: T[] | undefined,
  getId: (item: T) => string,
  onPersist: Persist
) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }));

  const [currentOrder, setCurrentOrderState] = useState<string[]>([]);
  const currentOrderRef = useRef<string[]>([]);
  const savingRef = useRef(false);
  const pendingRef = useRef<{ order: string[]; save: Persist } | null>(null);
  const confirmedRef = useRef<string[]>([]);

  const setCurrentOrder = useCallback((resolved: string[]) => {
    currentOrderRef.current = resolved;
    setCurrentOrderState(resolved);
  }, []);

  useEffect(() => {
    const order = items?.map(getId) ?? [];
    confirmedRef.current = order;
    if (!savingRef.current) setCurrentOrder(order);
  }, [getId, items, setCurrentOrder]);

  const onPersistRef = useRef(onPersist);
  useEffect(() => {
    onPersistRef.current = onPersist;
  }, [onPersist]);

  const persist = useCallback(
    function persistOrder(order: string[], save: Persist) {
      if (savingRef.current) {
        pendingRef.current = { order, save };
        return;
      }
      savingRef.current = true;
      const settle = (saved: boolean) => {
        savingRef.current = false;
        if (saved) confirmedRef.current = order;
        const next = pendingRef.current;
        pendingRef.current = null;
        if (next) persistOrder(next.order, next.save);
        else setCurrentOrder(saved ? order : confirmedRef.current);
      };
      save(order).then(
        () => settle(true),
        () => settle(false)
      );
    },
    [setCurrentOrder]
  );

  const orderedItems = useMemo(() => {
    const itemMap = new Map((items ?? []).map((item) => [getId(item), item]));
    const ordered = currentOrder
      .map((id) => itemMap.get(id))
      .filter((item): item is T => item !== undefined);
    const seen = new Set(ordered.map(getId));
    for (const item of items ?? []) {
      if (!seen.has(getId(item))) ordered.push(item);
    }
    return ordered;
  }, [currentOrder, getId, items]);

  const onDragEnd = useCallback(
    (e: DragEndEvent) => {
      const order = currentOrderRef.current;
      const { active, over } = e;
      if (!over || active.id === over.id) return;
      const oldPos = order.indexOf(active.id as string);
      const newPos = order.indexOf(over.id as string);
      if (oldPos < 0 || newPos < 0) return;
      const newOrder = arrayMove(order, oldPos, newPos);
      setCurrentOrder(newOrder);
      persist(newOrder, onPersistRef.current);
    },
    [persist, setCurrentOrder]
  );

  return { sensors, currentOrder, orderedItems, setCurrentOrder, onDragEnd };
}
