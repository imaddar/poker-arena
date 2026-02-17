import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { formatArchiveTableId } from '../src/lib/presentation.ts';

describe('formatArchiveTableId', () => {
  it('formats ids into texas archive labels', () => {
    assert.equal(formatArchiveTableId('t1'), 'TX_001');
    assert.equal(formatArchiveTableId('table-22'), 'TX_022');
    assert.equal(formatArchiveTableId('tx-1234'), 'TX_1234');
  });
});
