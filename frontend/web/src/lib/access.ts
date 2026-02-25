import type { User } from '../types';

export function isObserverUser(user: User | null): boolean {
  return user?.role === 'observer';
}

export function canUseAdminTableDirectory(user: User | null): boolean {
  return user?.role === 'admin';
}

export function canManageControlPlane(user: User | null): boolean {
  return user?.role === 'admin';
}

export function canViewObserverReplay(user: User | null): boolean {
  return user != null;
}
