import { useEffect, useMemo, useReducer, useRef } from 'react';
import type { DynamicConfig, DynamicType } from '@/api';
import {
  createDynamicEditorBaseline,
  isSameDynamicEditorBaseline,
  type DynamicEditorBaseline,
} from '@/features/dynamic/model/editor-baseline';

interface DynamicEditorState {
  baseline: DynamicEditorBaseline;
  type: DynamicType | null;
  frameName: string;
  configKey: string;
}

interface BaselineInput {
  contentId: string;
  serverType: DynamicType;
  serverFrameName: string | null;
  serverConfig: DynamicConfig;
}

type BaselineAction =
  | { type: 'SET_BASELINE'; baseline: DynamicEditorBaseline }
  | { type: 'SERVER_UPDATED_CLEAN'; serverBaseline: DynamicEditorBaseline }
  | { type: 'SERVER_UPDATED_DIRTY'; serverBaseline: DynamicEditorBaseline };

/*
Baseline sync state:

  server update arrives
        |
        v
  compare current editor values with the previous baseline
        |
        +-- no local edits, or local values already match server
        |      -> accept server baseline and copy server fields into the editor
        |
        +-- local edits differ from the incoming server baseline
               -> move the baseline forward for future dirty checks, but keep
                  the user's in-progress editor values intact

SET_BASELINE is dispatched after a successful local save, making the saved
server response the new clean point.
*/
export function useDynamicEditorBaselineSync({
  contentId,
  serverType,
  serverFrameName,
  serverConfig,
  type,
  frameName,
  configKey,
  setType,
  setFrameName,
  setConfig,
}: {
  contentId: string;
  serverType: DynamicType;
  serverFrameName: string | null;
  serverConfig: DynamicConfig;
  type: DynamicType | null;
  frameName: string;
  configKey: string;
  setType: (type: DynamicType) => void;
  setFrameName: (frameName: string) => void;
  setConfig: (config: DynamicConfig) => void;
}) {
  const [baseline, dispatch] = useReducer(
    baselineReducer,
    { contentId, serverType, serverFrameName, serverConfig },
    createBaselineFromInput
  );
  const serverBaseline = useMemo(
    () => createDynamicEditorBaseline(contentId, serverType, serverFrameName, serverConfig),
    [contentId, serverConfig, serverFrameName, serverType]
  );
  const editorStateRef = useRef<DynamicEditorState>({ baseline, type, frameName, configKey });
  const syncedServerRef = useRef<DynamicEditorBaseline | null>(null);

  useEffect(() => {
    editorStateRef.current = { baseline, type, frameName, configKey };
  }, [baseline, configKey, frameName, type]);

  useEffect(() => {
    if (
      syncedServerRef.current &&
      isSameDynamicEditorBaseline(syncedServerRef.current, serverBaseline)
    ) {
      return;
    }

    const editorState = editorStateRef.current;
    const transition = resolveServerBaselineTransition(editorState, serverBaseline);
    dispatch({ type: transition, serverBaseline });

    if (transition === 'SERVER_UPDATED_CLEAN') {
      syncEditorToServer(editorState, serverBaseline, serverConfig, {
        setType,
        setFrameName,
        setConfig,
      });
    }

    syncedServerRef.current = serverBaseline;
  }, [serverBaseline, serverConfig, setConfig, setFrameName, setType]);

  return {
    baseline,
    setBaseline: (nextBaseline: DynamicEditorBaseline) =>
      dispatch({ type: 'SET_BASELINE', baseline: nextBaseline }),
  };
}

function createBaselineFromInput({
  contentId,
  serverType,
  serverFrameName,
  serverConfig,
}: BaselineInput): DynamicEditorBaseline {
  return createDynamicEditorBaseline(contentId, serverType, serverFrameName, serverConfig);
}

function baselineReducer(
  baseline: DynamicEditorBaseline,
  action: BaselineAction
): DynamicEditorBaseline {
  switch (action.type) {
    case 'SET_BASELINE':
      return action.baseline;
    case 'SERVER_UPDATED_CLEAN':
    case 'SERVER_UPDATED_DIRTY':
      return isSameDynamicEditorBaseline(baseline, action.serverBaseline)
        ? baseline
        : action.serverBaseline;
  }
}

function resolveServerBaselineTransition(
  editorState: DynamicEditorState,
  serverBaseline: DynamicEditorBaseline
): Extract<BaselineAction['type'], 'SERVER_UPDATED_CLEAN' | 'SERVER_UPDATED_DIRTY'> {
  const hasLocalEdits =
    editorState.baseline.contentId === serverBaseline.contentId &&
    (editorState.type !== editorState.baseline.type ||
      editorState.frameName !== editorState.baseline.frameName ||
      editorState.configKey !== editorState.baseline.configKey);
  const localMatchesServer =
    editorState.type === serverBaseline.type &&
    editorState.frameName === serverBaseline.frameName &&
    editorState.configKey === serverBaseline.configKey;

  return hasLocalEdits && !localMatchesServer ? 'SERVER_UPDATED_DIRTY' : 'SERVER_UPDATED_CLEAN';
}

function syncEditorToServer(
  editorState: DynamicEditorState,
  serverBaseline: DynamicEditorBaseline,
  serverConfig: DynamicConfig,
  setters: {
    setType: (type: DynamicType) => void;
    setFrameName: (frameName: string) => void;
    setConfig: (config: DynamicConfig) => void;
  }
): void {
  if (editorState.type !== serverBaseline.type) setters.setType(serverBaseline.type);
  if (editorState.frameName !== serverBaseline.frameName) {
    setters.setFrameName(serverBaseline.frameName);
  }
  if (editorState.configKey !== serverBaseline.configKey) setters.setConfig(serverConfig);
}
