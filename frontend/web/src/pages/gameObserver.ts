import type { GameState } from '../types';
import type { LatestReplay } from '../api/types';

export function applyObserverReplayToState(state: GameState, latestReplay: LatestReplay): GameState {
  const next: GameState = {
    ...state,
    actionLog: [...state.actionLog],
  };

  if (!latestReplay.handId) {
    next.actionLog = ['No hands recorded for this table yet.'];
    return next;
  }

  next.handId = latestReplay.handId;
  next.actionLog = [...latestReplay.actionLog];
  return next;
}
