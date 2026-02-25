import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import {
  canManageControlPlane,
  canUseAdminTableDirectory,
  canViewObserverReplay,
  isObserverUser,
} from '../src/lib/access.ts';
import type { User } from '../src/types/index.ts';

describe('access role helpers', () => {
  it('allows admin users to load table directory', () => {
    const user: User = {
      id: 'u-admin',
      name: 'Admin',
      token: 'admin-token',
      role: 'admin',
    };
    assert.equal(canUseAdminTableDirectory(user), true);
    assert.equal(isObserverUser(user), false);
  });

  it('marks observer users and blocks admin directory access', () => {
    const user: User = {
      id: 'u-seat',
      name: 'Seat Observer',
      token: 'seat-token',
      role: 'observer',
      seatNo: 1,
    };
    assert.equal(canUseAdminTableDirectory(user), false);
    assert.equal(isObserverUser(user), true);
  });

  it('enforces control-plane actions as admin-only', () => {
    const observer: User = {
      id: 'u-seat',
      name: 'Seat Observer',
      token: 'seat-token',
      role: 'observer',
      seatNo: 1,
    };
    const admin: User = {
      id: 'u-admin',
      name: 'Admin',
      token: 'admin-token',
      role: 'admin',
    };

    assert.equal(canManageControlPlane(observer), false);
    assert.equal(canManageControlPlane(admin), true);
  });

  it('allows signed-in users to view observer replay routes', () => {
    const observer: User = {
      id: 'u-seat',
      name: 'Seat Observer',
      token: 'seat-token',
      role: 'observer',
      seatNo: 1,
    };

    assert.equal(canViewObserverReplay(null), false);
    assert.equal(canViewObserverReplay(observer), true);
  });
});
