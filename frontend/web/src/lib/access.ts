import type { User } from '../types';

export function isObserverUser(user: User | null): boolean {
  return user?.role === 'observer';
}

export function canUseAdminTableDirectory(user: User | null): boolean {
  return user?.role === 'admin';
}
