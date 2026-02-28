import type { ApiClient } from '../api/types';
import type { LatestReplay } from '../api/types';
import { clampRaiseAmount } from '../lib/pokerLogic.ts';
import type { GameState } from '../types';
import { applyObserverReplayToState } from './gameObserver.ts';

export interface GameLoadResult {
  state: GameState;
  raiseAmount: number;
  latestReplay?: LatestReplay;
}

export async function loadGameState(
  api: ApiClient,
  tableId: string,
  isMockMode: boolean,
): Promise<GameLoadResult> {
  const baseState = await api.getTableState(tableId);
  const latestReplay = isMockMode ? undefined : await api.getLatestReplay(tableId);
  const state = isMockMode || !latestReplay ? baseState : applyObserverReplayToState(baseState, latestReplay);
  const actingStack = state.seats[state.currentTurnSeat]?.stack ?? 0;

  return {
    state,
    raiseAmount: clampRaiseAmount({
      requested: state.minRaise,
      minRaise: state.minRaise,
      stack: actingStack,
    }),
    latestReplay,
  };
}
