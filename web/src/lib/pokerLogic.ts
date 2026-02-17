import type { Table } from '../types/index.ts';

export type StakeFilter = 'all' | 'low' | 'mid' | 'high';
export type StatusFilter = 'all' | 'waiting' | 'playing';

export interface LobbyFilters {
  status: StatusFilter;
  stake: StakeFilter;
  query: string;
}

export interface QuickBetParams {
  preset: 'min' | 'half-pot' | 'pot' | 'max';
  minRaise: number;
  toCall: number;
  pot: number;
  stack: number;
}

export interface ClampRaiseParams {
  requested: number;
  minRaise: number;
  stack: number;
}

const LOW_LIMIT = 100;
const HIGH_LIMIT = 300;

const normalize = (value: string) => value.trim().toLowerCase();

export function clampRaiseAmount({ requested, minRaise, stack }: ClampRaiseParams): number {
  if (stack <= 0) {
    return 0;
  }

  if (stack <= minRaise) {
    return stack;
  }

  return Math.min(Math.max(requested, minRaise), stack);
}

export function getQuickBetAmount({ preset, minRaise, toCall, pot, stack }: QuickBetParams): number {
  const safePot = Math.max(0, pot);
  const safeToCall = Math.max(0, toCall);

  switch (preset) {
    case 'min':
      return clampRaiseAmount({ requested: minRaise, minRaise, stack });
    case 'half-pot':
      return clampRaiseAmount({ requested: Math.floor(safePot * 0.5), minRaise, stack });
    case 'pot':
      return clampRaiseAmount({ requested: safePot, minRaise, stack });
    case 'max':
      return clampRaiseAmount({ requested: stack + safeToCall, minRaise, stack });
    default:
      return clampRaiseAmount({ requested: minRaise, minRaise, stack });
  }
}

function matchesStake(table: Table, stake: StakeFilter): boolean {
  if (stake === 'all') {
    return true;
  }

  const bigBlind = table.bigBlind;
  if (stake === 'low') {
    return bigBlind <= LOW_LIMIT;
  }

  if (stake === 'mid') {
    return bigBlind > LOW_LIMIT && bigBlind <= HIGH_LIMIT;
  }

  return bigBlind > HIGH_LIMIT;
}

export function filterTables({ tables, filters }: { tables: Table[]; filters: LobbyFilters }): Table[] {
  const query = normalize(filters.query);

  return tables
    .filter((table) => (filters.status === 'all' ? true : table.status === filters.status))
    .filter((table) => matchesStake(table, filters.stake))
    .filter((table) => {
      if (!query) {
        return true;
      }

      return normalize(table.name).includes(query);
    })
    .sort((a, b) => {
      if (a.status !== b.status) {
        return a.status === 'playing' ? -1 : 1;
      }

      return a.bigBlind - b.bigBlind;
    });
}
