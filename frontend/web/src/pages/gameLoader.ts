import type { ApiClient } from '../api/types';
import { clampRaiseAmount } from '../lib/pokerLogic.ts';
import type { GameState } from '../types';
import { applyObserverReplayToState } from './gameObserver.ts';

export interface GameLoadResult {
  state: GameState;
  raiseAmount: number;
}

export async function loadGameState(
  api: ApiClient,
  tableId: string,
  isMockMode: boolean,
): Promise<GameLoadResult> {
  const baseState = await api.getTableState(tableId);
  const state = isMockMode ? baseState : applyObserverReplayToState(baseState, await api.getLatestReplay(tableId));

  return {
    state,
    raiseAmount: clampRaiseAmount({
      requested: baseState.minRaise,
      minRaise: baseState.minRaise,
      stack: baseState.seats[2]?.stack ?? 0,
    }),
  };
}
